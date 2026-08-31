package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"isthmus/pkg/config"
)

func TestJobRunner(t *testing.T) {
	cfg := &config.Config{
		DeviceName: "test-node",
		DeviceID:   "node-id-123",
		Peers: map[string]config.Peer{
			"peer-remote-1": {
				DeviceID:   "peer-remote-1",
				DeviceName: "jack-vm",
			},
		},
	}

	disp := NewDispatcher(cfg)

	// 1. Test Local Execution
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := disp.ExecuteLocal(ctx, "echo 'isthmus-runner-ok'")
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (err: %s)", res.ExitCode, res.Error)
	}
	if !strings.Contains(res.Stdout, "isthmus-runner-ok") {
		t.Fatalf("expected stdout to contain 'isthmus-runner-ok', got '%s'", res.Stdout)
	}

	// 2. Test Batch Dispatch
	batch := disp.DispatchJob(ctx, "echo 'hello'", []string{"local", "peer-remote-1"})
	if len(batch.Results) != 2 {
		t.Fatalf("expected 2 batch results, got %d", len(batch.Results))
	}
	if batch.JobID == "" {
		t.Fatalf("expected non-empty job ID")
	}

	// 3. Test Templates
	templates := QuickCommandTemplates()
	if len(templates) == 0 {
		t.Fatalf("expected non-empty command templates")
	}
}
