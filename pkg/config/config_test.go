package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfgPath := filepath.Join(tempDir, "config.json")

	cfg, err := NewDefaultConfig("test-node-1")
	if err != nil {
		t.Fatalf("NewDefaultConfig failed: %v", err)
	}

	if cfg.DeviceName != "test-node-1" {
		t.Fatalf("expected device name 'test-node-1', got '%s'", cfg.DeviceName)
	}

	if cfg.DeviceID == "" || cfg.PrivateKey == "" || cfg.PublicKey == "" {
		t.Fatal("config fields must not be empty")
	}

	err = cfg.AddPeer(Peer{
		DeviceID:   "peer-device-id-12345",
		DeviceName: "remote-laptop",
		PublicKey:  "dGVzdHB1YmtleTEyMzQ1Njc4OTA=",
		VirtualIP:  "10.77.0.2",
		Allowed:    true,
	})
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.DeviceName != cfg.DeviceName {
		t.Fatalf("loaded name '%s' does not match '%s'", loaded.DeviceName, cfg.DeviceName)
	}

	peer, ok := loaded.GetPeer("peer-device-id-12345")
	if !ok {
		t.Fatal("peer not found in loaded config")
	}
	if peer.DeviceName != "remote-laptop" {
		t.Fatalf("unexpected peer name '%s'", peer.DeviceName)
	}

	loaded.RemovePeer("peer-device-id-12345")
	if _, ok := loaded.GetPeer("peer-device-id-12345"); ok {
		t.Fatal("peer should have been removed")
	}
}
