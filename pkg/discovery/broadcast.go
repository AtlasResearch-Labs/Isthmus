package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"isthmus/internal/logger"
)

const (
	DefaultBroadcastPort = 7755
	BroadcastMagic       = "ISTHMUS_BEACON_V1"
	PeerExpiryDuration   = 45 * time.Second
	AnnounceInterval     = 10 * time.Second
)

type AnnouncePacket struct {
	Magic      string `json:"magic"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
	VirtualIP  string `json:"virtual_ip"`
	SFTPPort   int    `json:"sftp_port"`
	TunnelPort int    `json:"tunnel_port"`
	Timestamp  int64  `json:"timestamp"`
}

type DiscoveredPeer struct {
	DeviceID    string    `json:"device_id"`
	DeviceName  string    `json:"device_name"`
	PublicKey   string    `json:"public_key"`
	VirtualIP   string    `json:"virtual_ip"`
	SFTPPort    int       `json:"sftp_port"`
	TunnelPort  int       `json:"tunnel_port"`
	LANEndpoint string    `json:"lan_endpoint"`
	LastSeen    time.Time `json:"last_seen"`
}

type PeerDiscoveryHandler func(peer DiscoveredPeer)

type DiscoveryService struct {
	mu           sync.RWMutex
	port         int
	localPacket  AnnouncePacket
	peers        map[string]DiscoveredPeer
	onDiscovered []PeerDiscoveryHandler
	stopChan     chan struct{}
	log          *logger.Logger
}

func NewDiscoveryService(port int, devID, devName, pubKey, virtIP string, sftpPort, tunnelPort int) *DiscoveryService {
	if port <= 0 {
		port = DefaultBroadcastPort
	}

	return &DiscoveryService{
		port: port,
		localPacket: AnnouncePacket{
			Magic:      BroadcastMagic,
			DeviceID:   devID,
			DeviceName: devName,
			PublicKey:  pubKey,
			VirtualIP:  virtIP,
			SFTPPort:   sftpPort,
			TunnelPort: tunnelPort,
		},
		peers:    make(map[string]DiscoveredPeer),
		stopChan: make(chan struct{}),
		log:      logger.WithPrefix("Discovery"),
	}
}

func (s *DiscoveryService) OnPeerDiscovered(h PeerDiscoveryHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDiscovered = append(s.onDiscovered, h)
}

func (s *DiscoveryService) Start(ctx context.Context) error {
	listenAddr := fmt.Sprintf("0.0.0.0:%d", s.port)
	conn, err := net.ListenPacket("udp4", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to bind UDP listener on %s: %w", listenAddr, err)
	}

	s.log.Info("LAN discovery service active on %s", listenAddr)

	go s.listenLoop(ctx, conn)
	go s.announceLoop(ctx)
	go s.cleanupLoop(ctx)

	return nil
}

func (s *DiscoveryService) listenLoop(ctx context.Context, conn net.PacketConn) {
	defer conn.Close()
	buf := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		var packet AnnouncePacket
		if err := json.Unmarshal(buf[:n], &packet); err != nil {
			continue
		}

		if packet.Magic != BroadcastMagic {
			continue
		}

		if packet.DeviceID == s.localPacket.DeviceID {
			continue
		}

		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			host = addr.String()
		}

		peer := DiscoveredPeer{
			DeviceID:    packet.DeviceID,
			DeviceName:  packet.DeviceName,
			PublicKey:   packet.PublicKey,
			VirtualIP:   packet.VirtualIP,
			SFTPPort:    packet.SFTPPort,
			TunnelPort:  packet.TunnelPort,
			LANEndpoint: fmt.Sprintf("%s:%d", host, packet.SFTPPort),
			LastSeen:    time.Now(),
		}

		s.mu.Lock()
		_, exists := s.peers[peer.DeviceID]
		s.peers[peer.DeviceID] = peer
		handlers := append([]PeerDiscoveryHandler(nil), s.onDiscovered...)
		s.mu.Unlock()

		if !exists {
			s.log.Info("Discovered new peer '%s' (%s) at %s", peer.DeviceName, peer.DeviceID, peer.LANEndpoint)
		}

		for _, h := range handlers {
			h(peer)
		}
	}
}

func (s *DiscoveryService) announceLoop(ctx context.Context) {
	ticker := time.NewTicker(AnnounceInterval)
	defer ticker.Stop()

	s.broadcastOnce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.broadcastOnce()
		}
	}
}

func (s *DiscoveryService) broadcastOnce() {
	broadcastAddr := fmt.Sprintf("255.255.255.255:%d", s.port)
	addr, err := net.ResolveUDPAddr("udp4", broadcastAddr)
	if err != nil {
		s.log.Debug("Failed to resolve broadcast address: %v", err)
		return
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		s.log.Debug("Failed to dial UDP broadcast: %v", err)
		return
	}
	defer conn.Close()

	packet := s.localPacket
	packet.Timestamp = time.Now().Unix()

	data, err := json.Marshal(packet)
	if err != nil {
		return
	}

	_, _ = conn.Write(data)
}

func (s *DiscoveryService) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for id, peer := range s.peers {
				if now.Sub(peer.LastSeen) > PeerExpiryDuration {
					s.log.Debug("Peer expired from cache: %s (%s)", peer.DeviceName, id)
					delete(s.peers, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *DiscoveryService) GetDiscoveredPeers() []DiscoveredPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]DiscoveredPeer, 0, len(s.peers))
	for _, p := range s.peers {
		result = append(result, p)
	}
	return result
}

func (s *DiscoveryService) Stop() {
	close(s.stopChan)
}
