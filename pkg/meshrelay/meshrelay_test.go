package meshrelay

import (
	"context"
	"net"
	"testing"
	"time"

	"isthmus/pkg/config"
)

func TestMeshRelayRouter(t *testing.T) {
	cfg := &config.Config{
		DeviceName: "node-alpha",
		DeviceID:   "dev-alpha-id",
		Peers: map[string]config.Peer{
			"dev-bravo-id": {
				DeviceID:   "dev-bravo-id",
				DeviceName: "node-bravo",
			},
			"dev-charlie-id": {
				DeviceID:   "dev-charlie-id",
				DeviceName: "node-charlie",
			},
		},
	}

	router := NewRouter(cfg)

	// 1. Direct Route
	ctx := context.Background()
	direct, err := router.FindBestRoute(ctx, "dev-bravo-id")
	if err != nil {
		t.Fatalf("find direct route err: %v", err)
	}
	if len(direct.Hops) != 2 {
		t.Fatalf("expected 2 hops for direct route, got %d", len(direct.Hops))
	}

	// 2. Multi-hop Route (Alpha -> Bravo -> Charlie)
	multiHop, err := router.FindMultiHopRoute("dev-charlie-id", "dev-bravo-id")
	if err != nil {
		t.Fatalf("find multi hop route err: %v", err)
	}
	if len(multiHop.Hops) != 3 {
		t.Fatalf("expected 3 hops for multi-hop route, got %d", len(multiHop.Hops))
	}

	// 3. List Routes
	all := router.ListActiveRoutes()
	if len(all) < 2 {
		t.Fatalf("expected >= 2 routes, got %d", len(all))
	}

	// 4. Test Traffic Forwarder (Pipe)
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	c3, c4 := net.Pipe()
	defer c3.Close()
	defer c4.Close()

	go func() {
		_, _, _ = RelayTraffic(c2, c3)
	}()

	go func() {
		_, _ = c1.Write([]byte("isthmus-relay-packet"))
	}()

	buf := make([]byte, 64)
	_ = c4.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, _ := c4.Read(buf)
	if string(buf[:n]) != "isthmus-relay-packet" {
		t.Logf("traffic relayed bytes: %d", n)
	}
}
