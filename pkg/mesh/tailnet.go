package mesh

import (
	"context"
	"fmt"
	"sync"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/coord"
)

type TailnetMesh struct {
	mu          sync.Mutex
	cfg         *config.Config
	coordClient *coord.Client
	stopChan    chan struct{}
	running     bool
	log         *logger.Logger
}

type MeshNodeInfo struct {
	DeviceID      string    `json:"device_id"`
	DeviceName    string    `json:"device_name"`
	PublicKey     string    `json:"public_key"`
	VirtualIP     string    `json:"virtual_ip"`
	ReflectedAddr string    `json:"reflected_addr"`
	LastSeen      time.Time `json:"last_seen"`
	IsSelf        bool      `json:"is_self"`
}

func NewTailnetMesh(cfg *config.Config, coordClient *coord.Client) *TailnetMesh {
	return &TailnetMesh{
		cfg:         cfg,
		coordClient: coordClient,
		stopChan:    make(chan struct{}),
		log:         logger.WithPrefix("TailnetMesh"),
	}
}

func (m *TailnetMesh) SyncOnce(ctx context.Context) ([]MeshNodeInfo, error) {
	if m.coordClient == nil {
		return nil, fmt.Errorf("no coordination server configured")
	}

	devices, err := m.coordClient.ListDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mesh devices: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var meshNodes []MeshNodeInfo

	for _, dev := range devices {
		isSelf := (dev.DeviceID == m.cfg.DeviceID)

		node := MeshNodeInfo{
			DeviceID:      dev.DeviceID,
			DeviceName:    dev.DeviceName,
			PublicKey:     dev.PublicKey,
			VirtualIP:     dev.VirtualIP,
			ReflectedAddr: dev.ReflectedAddr,
			LastSeen:      dev.LastSeen,
			IsSelf:        isSelf,
		}
		meshNodes = append(meshNodes, node)

		if isSelf {
			continue
		}

		// Reconcile with local peer map
		existingPeer, exists := m.cfg.GetPeer(dev.DeviceID)
		if !exists {
			// Auto-register new mesh peer
			newPeer := config.Peer{
				DeviceID:         dev.DeviceID,
				DeviceName:       dev.DeviceName,
				PublicKey:        dev.PublicKey,
				VirtualIP:        dev.VirtualIP,
				LastSeenEndpoint: dev.ReflectedAddr,
				LastSeenTime:     dev.LastSeen,
				Allowed:          true,
				ACL:              config.DefaultPeerACL(),
			}
			_ = m.cfg.AddPeer(newPeer)
			m.log.Info("Auto-discovered new mesh peer '%s' (%s)", dev.DeviceName, dev.DeviceID)
		} else {
			// Update endpoint and timestamp
			existingPeer.DeviceName = dev.DeviceName
			existingPeer.PublicKey = dev.PublicKey
			existingPeer.VirtualIP = dev.VirtualIP
			existingPeer.LastSeenEndpoint = dev.ReflectedAddr
			existingPeer.LastSeenTime = dev.LastSeen
			_ = m.cfg.AddPeer(existingPeer)
		}
	}

	_ = m.cfg.Save("")
	return meshNodes, nil
}

func (m *TailnetMesh) StartConvergenceLoop(ctx context.Context, interval time.Duration) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Initial sync
		_, _ = m.SyncOnce(ctx)

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopChan:
				return
			case <-ticker.C:
				_, err := m.SyncOnce(ctx)
				if err != nil {
					m.log.Debug("Mesh convergence sync error: %v", err)
				}
			}
		}
	}()
}

func (m *TailnetMesh) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		close(m.stopChan)
		m.running = false
	}
}
