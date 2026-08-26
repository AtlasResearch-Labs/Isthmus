package tunnel

import (
	"fmt"
	"net"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type PeerConfig struct {
	PublicKey    string
	Endpoint     string
	AllowedIPs   []string
	KeepaliveSec int
}

type TunnelConfig struct {
	InterfaceName string
	VirtualIP     string
	PrivateKey    string
	ListenPort    int
	Peers         []PeerConfig
}

type Tunnel struct {
	mu        sync.RWMutex
	config    TunnelConfig
	isRunning bool
	log       *logger.Logger
}

func NewTunnel(cfg TunnelConfig) (*Tunnel, error) {
	if cfg.InterfaceName == "" {
		cfg.InterfaceName = "isthmus0"
	}
	if cfg.ListenPort <= 0 {
		cfg.ListenPort = 51820
	}

	return &Tunnel{
		config: cfg,
		log:    logger.WithPrefix("Tunnel"),
	}, nil
}

func (t *Tunnel) Up() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isRunning {
		return fmt.Errorf("tunnel interface '%s' is already running", t.config.InterfaceName)
	}

	t.log.Info("Bringing up WireGuard tunnel interface '%s' with virtual IP %s on port %d",
		t.config.InterfaceName, t.config.VirtualIP, t.config.ListenPort)

	t.isRunning = true
	return nil
}

func (t *Tunnel) Down() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.isRunning {
		return nil
	}

	t.log.Info("Tearing down WireGuard tunnel interface '%s'", t.config.InterfaceName)
	t.isRunning = false
	return nil
}

func (t *Tunnel) AddPeer(peer PeerConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.log.Info("Configured tunnel peer with key %s (Endpoint: %s, AllowedIPs: %v)",
		peer.PublicKey, peer.Endpoint, peer.AllowedIPs)
	t.config.Peers = append(t.config.Peers, peer)
	return nil
}

func (t *Tunnel) IsActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isRunning
}

func ParseIPCIDR(cidr string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(cidr)
}

func LastHandshakeTime(peerPublicKey string) time.Time {
	return time.Time{}
}
