package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/coord"
	"isthmus/pkg/relay"
)

type ConnectionTier int

const (
	TierLAN ConnectionTier = iota + 1
	TierWANDirect
	TierRelayFallback
)

func (t ConnectionTier) String() string {
	switch t {
	case TierLAN:
		return "LAN Direct"
	case TierWANDirect:
		return "WAN Direct (STUN)"
	case TierRelayFallback:
		return "DERP Relay Fallback"
	default:
		return "Unknown"
	}
}

type RoutedConnection struct {
	Conn net.Conn
	Tier ConnectionTier
	Addr string
}

type AutoRouter struct {
	cfg         *config.Config
	resolver    *PeerResolver
	coordClient *coord.Client
	log         *logger.Logger
}

func NewAutoRouter(cfg *config.Config) *AutoRouter {
	var coordClient *coord.Client
	if cfg.CoordServer != "" {
		coordClient = coord.NewClient(cfg.CoordServer, "", cfg)
	}

	return &AutoRouter{
		cfg:         cfg,
		resolver:    NewPeerResolver(cfg, nil),
		coordClient: coordClient,
		log:         logger.WithPrefix("AutoRouter"),
	}
}

func (r *AutoRouter) DialPeer(ctx context.Context, target string) (*RoutedConnection, error) {
	// If direct host:port was provided, dial directly
	if _, _, err := net.SplitHostPort(target); err == nil {
		r.log.Info("Connecting directly to endpoint %s...", target)
		conn, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err != nil {
			return nil, fmt.Errorf("direct connection to %s failed: %w", target, err)
		}
		return &RoutedConnection{Conn: conn, Tier: TierLAN, Addr: target}, nil
	}

	// 1. Tier 1: LAN Discovery
	r.log.Info("Attempting Tier 1 (LAN discovery) for '%s'...", target)
	lanCtx, lanCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	lanEndpoint, err := r.resolver.ResolvePeerEndpoint(lanCtx, target)
	lanCancel()

	if err == nil && lanEndpoint != "" {
		r.log.Info("Found LAN endpoint: %s. Connecting...", lanEndpoint)
		dialer := &net.Dialer{Timeout: 2 * time.Second}
		conn, dialErr := dialer.DialContext(ctx, "tcp", lanEndpoint)
		if dialErr == nil {
			r.log.Info("Established Tier 1 (LAN) connection with '%s' on %s", target, lanEndpoint)
			return &RoutedConnection{Conn: conn, Tier: TierLAN, Addr: lanEndpoint}, nil
		}
		r.log.Debug("LAN connection failed: %v", dialErr)
	}

	// 2. Tier 2 & 3: WAN Coordination & Relay Fallback
	if r.coordClient == nil {
		return nil, fmt.Errorf("peer '%s' not reachable on LAN and no coordination server is configured", target)
	}

	r.log.Info("Attempting Tier 2 (WAN coordination exchange) for '%s'...", target)
	coordCtx, coordCancel := context.WithTimeout(ctx, 4*time.Second)
	defer coordCancel()

	peerInfo, err := r.coordClient.ExchangePeer(coordCtx, target)
	if err != nil {
		return nil, fmt.Errorf("coordination exchange failed for '%s': %w", target, err)
	}

	// Tier 2: Try Direct WAN endpoint
	if peerInfo.PublicAddr != "" {
		r.log.Info("Attempting Tier 2 direct WAN connection to %s...", peerInfo.PublicAddr)
		dialer := &net.Dialer{Timeout: 3 * time.Second}
		conn, dialErr := dialer.DialContext(ctx, "tcp", peerInfo.PublicAddr)
		if dialErr == nil {
			r.log.Info("Established Tier 2 (WAN Direct) connection with '%s' on %s", target, peerInfo.PublicAddr)
			return &RoutedConnection{Conn: conn, Tier: TierWANDirect, Addr: peerInfo.PublicAddr}, nil
		}
		r.log.Debug("Tier 2 direct WAN connection failed (NAT/Firewall): %v", dialErr)
	}

	// Tier 3: DERP Relay Fallback
	if peerInfo.RelayEnabled && peerInfo.RelayEndpoint != "" {
		r.log.Info("Falling back to Tier 3 (DERP Relay) via %s...", peerInfo.RelayEndpoint)
		relayClient, relayErr := relay.ConnectRelay(peerInfo.RelayEndpoint, r.cfg.DeviceID)
		if relayErr != nil {
			return nil, fmt.Errorf("failed connecting to DERP relay: %w", relayErr)
		}

		targetDeviceID := peerInfo.TargetDevice
		if targetDeviceID == "" {
			targetDeviceID = target
		}

		conn, dialErr := relayClient.DialPeer(targetDeviceID)
		if dialErr != nil {
			relayClient.Close()
			return nil, fmt.Errorf("failed opening relay session to '%s': %w", target, dialErr)
		}

		r.log.Info("Established Tier 3 (DERP Relay) connection with '%s'", target)
		return &RoutedConnection{
			Conn: conn,
			Tier: TierRelayFallback,
			Addr: fmt.Sprintf("relay://%s/%s", peerInfo.RelayEndpoint, strings.TrimSpace(targetDeviceID)),
		}, nil
	}

	return nil, fmt.Errorf("all connection tiers failed for peer '%s'", target)
}
