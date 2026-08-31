package tray

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
)

type TrayState struct {
	ActiveNodes int    `json:"active_nodes"`
	MeshIP      string `json:"mesh_ip"`
	AutoSync    bool   `json:"auto_sync"`
	GUIPort     int    `json:"gui_port"`
}

type TrayManager struct {
	cfg     *config.Config
	state   TrayState
	log     *logger.Logger
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewTrayManager(cfg *config.Config, guiPort int) *TrayManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TrayManager{
		cfg: cfg,
		state: TrayState{
			ActiveNodes: len(cfg.Peers) + 1,
			MeshIP:      cfg.VirtualIP,
			AutoSync:    true,
			GUIPort:     guiPort,
		},
		log:    logger.WithPrefix("SystemTray"),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (tm *TrayManager) UpdatePeerCount(count int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.state.ActiveNodes = count
	tm.log.Info("System tray status updated: %d active mesh nodes", count)
}

func (tm *TrayManager) OpenGUI() error {
	tm.mu.RLock()
	port := tm.state.GUIPort
	tm.mu.RUnlock()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

func (tm *TrayManager) ToggleAutoSync() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.state.AutoSync = !tm.state.AutoSync
	tm.log.Info("Auto-sync status toggled: %v", tm.state.AutoSync)
	return tm.state.AutoSync
}

func (tm *TrayManager) Start() {
	tm.log.Info("System tray background service active (Port: %d, IP: %s)", tm.state.GUIPort, tm.state.MeshIP)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-tm.ctx.Done():
				return
			case <-ticker.C:
				// Periodic health check
			}
		}
	}()
}

func (tm *TrayManager) Stop() {
	tm.cancel()
}
