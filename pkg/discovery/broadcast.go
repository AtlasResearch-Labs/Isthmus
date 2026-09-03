package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"isthmus/internal/logger"
)

const (
	DefaultBroadcastPort = 7755
	BroadcastMagic       = "ISTHMUS_BEACON_V1"
	PeerExpiryDuration   = 45 * time.Second
	AnnounceInterval     = 3 * time.Second
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
		// If port is already bound (e.g. background daemon is running), bind to an ephemeral port
		listenAddr = "0.0.0.0:0"
		conn, err = net.ListenPacket("udp4", listenAddr)
		if err != nil {
			return fmt.Errorf("failed to bind UDP listener on %s: %w", listenAddr, err)
		}
	}

	s.log.Info("LAN discovery service active on %s", conn.LocalAddr().String())

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

		// Send immediate announcement reply back to the sender on discovery port
		replyPacket := s.localPacket
		replyPacket.Timestamp = time.Now().Unix()
		if replyData, err := json.Marshal(replyPacket); err == nil {
			if targetUDP, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", host, s.port)); err == nil {
				_, _ = conn.WriteTo(replyData, targetUDP)
			}
			_, _ = conn.WriteTo(replyData, addr)
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
	packet := s.localPacket
	packet.Timestamp = time.Now().Unix()

	data, err := json.Marshal(packet)
	if err != nil {
		return
	}

	// 1. Send to generic IPv4 broadcast
	broadcastAddr := fmt.Sprintf("255.255.255.255:%d", s.port)
	if addr, err := net.ResolveUDPAddr("udp4", broadcastAddr); err == nil {
		if conn, err := net.DialUDP("udp4", nil, addr); err == nil {
			_, _ = conn.Write(data)
			_ = conn.Close()
		}
	}

	// 2. Enumerate and broadcast across all active local network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			if len(mask) == 16 {
				mask = mask[12:]
			}
			if len(mask) != 4 {
				continue
			}
			bcast := net.IPv4(
				ip[0]|^mask[0],
				ip[1]|^mask[1],
				ip[2]|^mask[2],
				ip[3]|^mask[3],
			)
			bcastAddr := &net.UDPAddr{IP: bcast, Port: s.port}
			if bConn, err := net.DialUDP("udp4", nil, bcastAddr); err == nil {
				_, _ = bConn.Write(data)
				_ = bConn.Close()
			}
		}
	}

	// 3. Unicast ping all known peers in cache
	s.mu.RLock()
	peers := make([]DiscoveredPeer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
	}
	s.mu.RUnlock()

	for _, p := range peers {
		s.PingEndpoint(p.LANEndpoint)
	}
}

func (s *DiscoveryService) PingEndpoint(endpoint string) {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	if host == "" || strings.HasPrefix(host, "127.") {
		return
	}
	targetAddr := fmt.Sprintf("%s:%d", host, s.port)
	if addr, err := net.ResolveUDPAddr("udp4", targetAddr); err == nil {
		if conn, err := net.DialUDP("udp4", nil, addr); err == nil {
			packet := s.localPacket
			packet.Timestamp = time.Now().Unix()
			if data, err := json.Marshal(packet); err == nil {
				_, _ = conn.Write(data)
			}
			_ = conn.Close()
		}
	}
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
