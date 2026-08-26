package mobile

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/coord"
	"isthmus/pkg/discovery"
	"isthmus/pkg/fileserver"
)

var (
	agentMu     sync.Mutex
	agentCancel context.CancelFunc
	activeCfg   *config.Config
	activeDisc  *discovery.DiscoveryService
	activeSFTP  *fileserver.Server
	log         = logger.WithPrefix("AndroidBridge")
)

type MobileStatusResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`
	VirtualIP string `json:"virtual_ip,omitempty"`
}

type MobilePeerInfo struct {
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	LANEndpoint string `json:"lan_endpoint"`
	VirtualIP   string `json:"virtual_ip"`
}

func StartAgent(configJSON string) string {
	agentMu.Lock()
	defer agentMu.Unlock()

	if agentCancel != nil {
		return formatStatus(true, "Agent is already running", "", "")
	}

	var cfg *config.Config
	if configJSON != "" {
		var parsed config.Config
		if err := json.Unmarshal([]byte(configJSON), &parsed); err != nil {
			return formatStatus(false, fmt.Sprintf("invalid config JSON: %v", err), "", "")
		}
		cfg = &parsed
	} else {
		defaultCfg, err := config.NewDefaultConfig("android-device")
		if err != nil {
			return formatStatus(false, fmt.Sprintf("failed to init default config: %v", err), "", "")
		}
		cfg = defaultCfg
	}

	srv, err := fileserver.NewServer(fileserver.ServerConfig{
		Port:    cfg.SFTPPort,
		RootDir: cfg.SharedDir,
	})
	if err != nil {
		return formatStatus(false, fmt.Sprintf("SFTP server init error: %v", err), "", "")
	}

	if err := srv.Start(); err != nil {
		return formatStatus(false, fmt.Sprintf("SFTP start error: %v", err), "", "")
	}

	ctx, cancel := context.WithCancel(context.Background())
	agentCancel = cancel
	activeCfg = cfg
	activeSFTP = srv

	disc := discovery.NewDiscoveryService(
		cfg.BroadcastPort,
		cfg.DeviceID,
		cfg.DeviceName,
		cfg.PublicKey,
		cfg.VirtualIP,
		srv.Port(),
		cfg.ListenPort,
	)
	activeDisc = disc
	_ = disc.Start(ctx)

	if cfg.CoordServer != "" {
		coordClient := coord.NewClient(cfg.CoordServer, "", cfg)
		_, _ = coordClient.Register(ctx)
		coordClient.StartHeartbeatLoop(ctx, 25*time.Second)
	}

	log.Info("Android Isthmus agent started for device '%s' (%s)", cfg.DeviceName, cfg.DeviceID)
	return formatStatus(true, "Isthmus mobile agent active", cfg.DeviceID, cfg.VirtualIP)
}

func StopAgent() string {
	agentMu.Lock()
	defer agentMu.Unlock()

	if agentCancel != nil {
		agentCancel()
		agentCancel = nil
	}
	if activeDisc != nil {
		activeDisc.Stop()
		activeDisc = nil
	}
	if activeSFTP != nil {
		activeSFTP.Stop()
		activeSFTP = nil
	}
	activeCfg = nil

	return formatStatus(true, "Agent stopped successfully", "", "")
}

func GetDiscoveredPeersJSON() string {
	agentMu.Lock()
	defer agentMu.Unlock()

	if activeDisc == nil {
		return "[]"
	}

	peers := activeDisc.GetDiscoveredPeers()
	var list []MobilePeerInfo
	for _, p := range peers {
		list = append(list, MobilePeerInfo{
			DeviceID:    p.DeviceID,
			DeviceName:  p.DeviceName,
			LANEndpoint: p.LANEndpoint,
			VirtualIP:   p.VirtualIP,
		})
	}

	bytes, _ := json.Marshal(list)
	return string(bytes)
}

func PullFile(peerTarget, remotePath, localDest string) string {
	agentMu.Lock()
	cfg := activeCfg
	agentMu.Unlock()

	if cfg == nil {
		return formatStatus(false, "agent not started", "", "")
	}

	router := discovery.NewAutoRouter(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, peerTarget)
	if err != nil {
		return formatStatus(false, fmt.Sprintf("connection error: %v", err), "", "")
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint: routed.Addr,
	})
	if err != nil {
		return formatStatus(false, fmt.Sprintf("SFTP handshake error: %v", err), "", "")
	}
	defer client.Close()

	checksum, err := client.PullFileResume(remotePath, localDest, nil)
	if err != nil {
		return formatStatus(false, fmt.Sprintf("pull failed: %v", err), "", "")
	}

	return formatStatus(true, fmt.Sprintf("Pull complete (SHA256: %s)", checksum), cfg.DeviceID, "")
}

func PushFile(peerTarget, localFile, remoteDest string) string {
	agentMu.Lock()
	cfg := activeCfg
	agentMu.Unlock()

	if cfg == nil {
		return formatStatus(false, "agent not started", "", "")
	}

	router := discovery.NewAutoRouter(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, peerTarget)
	if err != nil {
		return formatStatus(false, fmt.Sprintf("connection error: %v", err), "", "")
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint: routed.Addr,
	})
	if err != nil {
		return formatStatus(false, fmt.Sprintf("SFTP handshake error: %v", err), "", "")
	}
	defer client.Close()

	err = client.PushFile(localFile, remoteDest, nil)
	if err != nil {
		return formatStatus(false, fmt.Sprintf("push failed: %v", err), "", "")
	}

	return formatStatus(true, "Push complete", cfg.DeviceID, "")
}

func formatStatus(success bool, msg, deviceID, virtualIP string) string {
	resp := MobileStatusResponse{
		Success:   success,
		Message:   msg,
		DeviceID:  deviceID,
		VirtualIP: virtualIP,
	}
	bytes, _ := json.Marshal(resp)
	return string(bytes)
}
