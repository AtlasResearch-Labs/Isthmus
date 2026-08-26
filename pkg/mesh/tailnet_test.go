package mesh

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"isthmus/pkg/config"
	"isthmus/pkg/coord"
)

func TestTailnetMeshSync(t *testing.T) {
	cfg, err := config.NewDefaultConfig("node-local")
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	// Mock coordination server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/devices" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[
				{
					"device_id": "remote-node-12345",
					"device_name": "laptop-alice",
					"public_key": "pubkey-alice-base64",
					"virtual_ip": "10.77.0.2",
					"reflected_addr": "198.51.100.20:2222",
					"last_seen": "2026-08-26T20:00:00Z"
				},
				{
					"device_id": "` + cfg.DeviceID + `",
					"device_name": "` + cfg.DeviceName + `",
					"public_key": "` + cfg.PublicKey + `",
					"virtual_ip": "` + cfg.VirtualIP + `",
					"reflected_addr": "198.51.100.10:2222",
					"last_seen": "2026-08-26T20:00:00Z"
				}
			]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	coordClient := coord.NewClient(ts.URL, "", cfg)
	tailnet := NewTailnetMesh(cfg, coordClient)

	nodes, err := tailnet.SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("SyncOnce failed: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("expected 2 mesh nodes, got %d", len(nodes))
	}

	// Verify remote node was auto-registered in cfg.Peers
	peer, exists := cfg.GetPeer("remote-node-12345")
	if !exists {
		t.Fatal("remote-node-12345 was not auto-registered in local peers map")
	}

	if peer.DeviceName != "laptop-alice" {
		t.Fatalf("expected peer name 'laptop-alice', got '%s'", peer.DeviceName)
	}

	if peer.VirtualIP != "10.77.0.2" {
		t.Fatalf("expected virtual IP 10.77.0.2, got '%s'", peer.VirtualIP)
	}
}
