package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"isthmus/pkg/identity"
)

type Peer struct {
	DeviceID         string    `json:"device_id"`
	DeviceName       string    `json:"device_name"`
	PublicKey        string    `json:"public_key"`
	VirtualIP        string    `json:"virtual_ip"`
	LastSeenEndpoint string    `json:"last_seen_endpoint,omitempty"`
	LastSeenTime     time.Time `json:"last_seen_time,omitempty"`
	Allowed          bool      `json:"allowed"`
	ACL              PeerACL   `json:"acl,omitempty"`
}

type Config struct {
	mu sync.RWMutex `json:"-"`

	DeviceName    string          `json:"device_name"`
	DeviceID      string          `json:"device_id"`
	PrivateKey    string          `json:"private_key"`
	PublicKey     string          `json:"public_key"`
	VirtualIP     string          `json:"virtual_ip"`
	ListenPort    int             `json:"listen_port"`
	SFTPPort      int             `json:"sftp_port"`
	BroadcastPort int             `json:"broadcast_port"`
	SharedDir     string          `json:"shared_dir"`
	CoordServer   string          `json:"coord_server,omitempty"`
	Peers         map[string]Peer `json:"peers"`
}

func NewDefaultConfig(deviceName string) (*Config, error) {
	if deviceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			deviceName = "isthmus-node"
		} else {
			deviceName = hostname
		}
	}

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair: %w", err)
	}

	sharedDir, err := DefaultSharedDir()
	if err != nil {
		sharedDir = "./IsthmusShare"
	}

	return &Config{
		DeviceName:    deviceName,
		DeviceID:      identity.DeviceIDFromPublicKey(kp.PublicKey),
		PrivateKey:    kp.PrivateKey.String(),
		PublicKey:     kp.PublicKey.String(),
		VirtualIP:     "10.77.0.1",
		ListenPort:    51820,
		SFTPPort:      2222,
		BroadcastPort: 7755,
		SharedDir:     sharedDir,
		Peers:         make(map[string]Peer),
	}, nil
}

func LoadConfig(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigFile()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.Peers == nil {
		cfg.Peers = make(map[string]Peer)
	}

	return &cfg, nil
}

func (c *Config) Save(path string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if path == "" {
		var err error
		path, err = DefaultConfigFile()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (c *Config) AddPeer(peer Peer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if peer.DeviceID == "" {
		return errors.New("peer device_id cannot be empty")
	}
	if peer.PublicKey == "" {
		return errors.New("peer public_key cannot be empty")
	}

	c.Peers[peer.DeviceID] = peer
	return nil
}

func (c *Config) RemovePeer(deviceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.Peers, deviceID)
}

func (c *Config) GetPeer(deviceID string) (Peer, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, ok := c.Peers[deviceID]
	return p, ok
}
