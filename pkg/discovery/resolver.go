package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
)

type PeerResolver struct {
	cfg  *config.Config
	disc *DiscoveryService
	log  *logger.Logger
}

func NewPeerResolver(cfg *config.Config, disc *DiscoveryService) *PeerResolver {
	return &PeerResolver{
		cfg:  cfg,
		disc: disc,
		log:  logger.WithPrefix("Resolver"),
	}
}

func (r *PeerResolver) ResolvePeerEndpoint(ctx context.Context, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target peer cannot be empty")
	}

	// Check if direct host:port endpoint was specified
	if _, _, err := net.SplitHostPort(target); err == nil {
		return target, nil
	}

	// Check in-memory discovered peers first
	if r.disc != nil {
		peers := r.disc.GetDiscoveredPeers()
		for _, p := range peers {
			if strings.EqualFold(p.DeviceName, target) || strings.EqualFold(p.DeviceID, target) {
				return p.LANEndpoint, nil
			}
		}
	}

	// Active short scan on LAN if discovery service can be started
	r.log.Info("Scanning LAN for peer '%s'...", target)
	scanCtx, scanCancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer scanCancel()

	disc := NewDiscoveryService(
		r.cfg.BroadcastPort,
		r.cfg.DeviceID,
		r.cfg.DeviceName,
		r.cfg.PublicKey,
		r.cfg.VirtualIP,
		r.cfg.SFTPPort,
		r.cfg.ListenPort,
	)

	resolvedChan := make(chan string, 1)
	disc.OnPeerDiscovered(func(peer DiscoveredPeer) {
		if strings.EqualFold(peer.DeviceName, target) || strings.EqualFold(peer.DeviceID, target) {
			// Update and persist discovered endpoint
			if cp, exists := r.cfg.GetPeer(peer.DeviceID); exists {
				cp.LastSeenEndpoint = peer.LANEndpoint
				cp.LastSeenTime = time.Now()
				_ = r.cfg.AddPeer(cp)
				_ = r.cfg.Save("")
			} else {
				for id, p := range r.cfg.Peers {
					if strings.EqualFold(p.DeviceName, peer.DeviceName) {
						p.LastSeenEndpoint = peer.LANEndpoint
						p.LastSeenTime = time.Now()
						_ = r.cfg.AddPeer(p)
						_ = r.cfg.Save("")
						_ = id
						break
					}
				}
			}

			select {
			case resolvedChan <- peer.LANEndpoint:
			default:
			}
		}
	})

	if err := disc.Start(scanCtx); err == nil {
		defer disc.Stop()
		// Directly ping any configured peer endpoints for instant response
		for _, p := range r.cfg.Peers {
			if p.LastSeenEndpoint != "" {
				disc.PingEndpoint(p.LastSeenEndpoint)
			}
		}
		select {
		case ep := <-resolvedChan:
			return ep, nil
		case <-scanCtx.Done():
		}
	}

	isNotLoopback := func(ep string) bool {
		host, _, err := net.SplitHostPort(ep)
		if err != nil {
			host = ep
		}
		return host != "127.0.0.1" && host != "::1" && host != "localhost" && !strings.HasPrefix(host, "127.")
	}

	// Check configured peers in config.json as fallback
	if peer, ok := r.cfg.GetPeer(target); ok {
		if peer.LastSeenEndpoint != "" && isNotLoopback(peer.LastSeenEndpoint) {
			return peer.LastSeenEndpoint, nil
		}
	}

	for _, peer := range r.cfg.Peers {
		if strings.EqualFold(peer.DeviceName, target) {
			if peer.LastSeenEndpoint != "" && isNotLoopback(peer.LastSeenEndpoint) {
				return peer.LastSeenEndpoint, nil
			}
		}
	}

	return "", fmt.Errorf("could not resolve peer '%s' on LAN or in configured peer list", target)
}
