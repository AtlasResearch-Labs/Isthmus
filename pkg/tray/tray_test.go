package tray

import (
	"testing"

	"isthmus/pkg/config"
)

func TestTrayManager(t *testing.T) {
	cfg, err := config.NewDefaultConfig("node-tray")
	if err != nil {
		t.Fatalf("failed to init config: %v", err)
	}

	tm := NewTrayManager(cfg, 7788)
	tm.Start()
	defer tm.Stop()

	tm.UpdatePeerCount(4)
	if tm.state.ActiveNodes != 4 {
		t.Errorf("expected 4 active nodes, got %d", tm.state.ActiveNodes)
	}

	curSync := tm.state.AutoSync
	toggled := tm.ToggleAutoSync()
	if toggled == curSync {
		t.Errorf("expected toggle to change state, got %v", toggled)
	}
}
