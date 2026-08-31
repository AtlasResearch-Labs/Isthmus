package diagnostics

import (
	"context"
	"testing"
	"time"

	"isthmus/pkg/config"
)

func TestDiagnosticsSpeedtest(t *testing.T) {
	cfg := &config.Config{
		Peers: map[string]config.Peer{
			"peer-1": {
				DeviceName: "test-node",
			},
		},
	}

	runner := NewRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := runner.RunSpeedtest(ctx, "peer-1", 1024*1024)
	if res.Error != "" {
		t.Fatalf("speedtest error: %s", res.Error)
	}
	if res.SpeedMBps <= 0 {
		t.Fatalf("expected positive throughput, got %f", res.SpeedMBps)
	}
}
