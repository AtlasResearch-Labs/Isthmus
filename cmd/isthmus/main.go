package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/coord"
	"isthmus/pkg/discovery"
	"isthmus/pkg/fileserver"
)

const version = "0.3.0-phase2"

func printUsage() {
	fmt.Println("Isthmus - Cross-Device Secure Tunnel and File Access")
	fmt.Printf("Version: %s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  isthmus <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init                  Initialize device identity and configuration")
	fmt.Println("  status                Display local node status and peers")
	fmt.Println("  discover              Scan LAN for other Isthmus nodes")
	fmt.Println("  serve                 Start local file server and LAN beacon")
	fmt.Println("  daemon                Run persistent background node service with WAN sync")
	fmt.Println("  browse <peer> [path]  Browse remote files on a peer")
	fmt.Println("  pull <peer> <remote>  Pull a file from a peer (LAN, WAN Direct, or Relay)")
	fmt.Println("  push <peer> <local>   Push a file to a peer")
	fmt.Println("  sync <peer> [remote]  Recursively delta-sync a folder from a peer")
	fmt.Println("  coord <set|status>    Manage coordination server connection")
	fmt.Println("  peer <add|list|rm>    Manage configured trusted peers")
	fmt.Println("  version               Show version information")
	fmt.Println()
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]
	switch command {
	case "init":
		cmdInit(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "discover":
		cmdDiscover(os.Args[2:])
	case "serve":
		cmdServe(os.Args[2:])
	case "daemon":
		cmdDaemon(os.Args[2:])
	case "browse":
		cmdBrowse(os.Args[2:])
	case "pull":
		cmdPull(os.Args[2:])
	case "push":
		cmdPush(os.Args[2:])
	case "sync":
		cmdSync(os.Args[2:])
	case "coord":
		cmdCoord(os.Args[2:])
	case "peer":
		cmdPeer(os.Args[2:])
	case "version":
		fmt.Printf("Isthmus version %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'isthmus help' for usage.\n", command)
		os.Exit(1)
	}
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	name := fs.String("name", "", "Device name (defaults to hostname)")
	force := fs.Bool("force", false, "Overwrite existing configuration if present")
	coordSrv := fs.String("coord", "", "Coordination server URL (e.g. http://198.51.100.1:8080)")
	fs.Parse(args)

	cfgPath, err := config.DefaultConfigFile()
	if err != nil {
		logger.Error("Failed to resolve config path: %v", err)
		os.Exit(1)
	}

	if _, err := os.Stat(cfgPath); err == nil && !*force {
		logger.Warn("Configuration already exists at %s", cfgPath)
		logger.Info("Use -force to overwrite.")
		return
	}

	cfg, err := config.NewDefaultConfig(*name)
	if err != nil {
		logger.Error("Failed to generate default configuration: %v", err)
		os.Exit(1)
	}

	if *coordSrv != "" {
		cfg.CoordServer = *coordSrv
	}

	if err := cfg.Save(cfgPath); err != nil {
		logger.Error("Failed to save configuration: %v", err)
		os.Exit(1)
	}

	logger.Info("Initialized Isthmus node:")
	fmt.Printf("  Device Name:   %s\n", cfg.DeviceName)
	fmt.Printf("  Device ID:     %s\n", cfg.DeviceID)
	fmt.Printf("  Public Key:    %s\n", cfg.PublicKey)
	fmt.Printf("  Virtual IP:    %s\n", cfg.VirtualIP)
	fmt.Printf("  Shared Folder: %s\n", cfg.SharedDir)
	if cfg.CoordServer != "" {
		fmt.Printf("  Coord Server:  %s\n", cfg.CoordServer)
	}
	fmt.Printf("  Config Saved:  %s\n", cfgPath)
}

func cmdStatus(args []string) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("No configuration found. Please run 'isthmus init' first.")
		os.Exit(1)
	}

	fmt.Println("=== Local Isthmus Node ===")
	fmt.Printf("Device Name:       %s\n", cfg.DeviceName)
	fmt.Printf("Device ID:         %s\n", cfg.DeviceID)
	fmt.Printf("Public Key:        %s\n", cfg.PublicKey)
	fmt.Printf("Assigned Mesh IP:  %s\n", cfg.VirtualIP)
	fmt.Printf("Data / Tunnel Port:%d\n", cfg.ListenPort)
	fmt.Printf("SFTP Port:         %d\n", cfg.SFTPPort)
	fmt.Printf("Broadcast Port:    %d\n", cfg.BroadcastPort)
	fmt.Printf("Shared Directory:  %s\n", cfg.SharedDir)
	if cfg.CoordServer != "" {
		fmt.Printf("Coordination Srv:  %s\n", cfg.CoordServer)
	} else {
		fmt.Printf("Coordination Srv:  Not configured (LAN only)\n")
	}
	fmt.Println()
	fmt.Printf("Configured Peers: (%d)\n", len(cfg.Peers))
	for id, peer := range cfg.Peers {
		fmt.Printf("  - %s (%s) [IP: %s, Allowed: %v]\n", peer.DeviceName, id, peer.VirtualIP, peer.Allowed)
	}
}

func cmdDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	timeout := fs.Duration("timeout", 5*time.Second, "Scan timeout duration")
	fs.Parse(args)

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	disc := discovery.NewDiscoveryService(
		cfg.BroadcastPort,
		cfg.DeviceID,
		cfg.DeviceName,
		cfg.PublicKey,
		cfg.VirtualIP,
		cfg.SFTPPort,
		cfg.ListenPort,
	)

	disc.OnPeerDiscovered(func(peer discovery.DiscoveredPeer) {
		fmt.Printf("[Found Peer] Name: %-15s ID: %-32s LAN: %s MeshIP: %s\n",
			peer.DeviceName, peer.DeviceID, peer.LANEndpoint, peer.VirtualIP)
	})

	if err := disc.Start(ctx); err != nil {
		logger.Error("Failed to start discovery service: %v", err)
		os.Exit(1)
	}

	logger.Info("Scanning local network for %v...", *timeout)
	<-ctx.Done()

	peers := disc.GetDiscoveredPeers()
	fmt.Printf("\nScan complete. Total active peers found on LAN: %d\n", len(peers))
}

func cmdServe(args []string) {
	runServerLoop(args, false)
}

func cmdDaemon(args []string) {
	runServerLoop(args, true)
}

func runServerLoop(args []string, isDaemon bool) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 0, "Override SFTP service port")
	rootDir := fs.String("root", "", "Override root directory for file transfer")
	fs.Parse(args)

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	if *port != 0 {
		cfg.SFTPPort = *port
	}
	if *rootDir != "" {
		cfg.SharedDir = *rootDir
	}

	sftpServer, err := fileserver.NewServer(fileserver.ServerConfig{
		Port:    cfg.SFTPPort,
		RootDir: cfg.SharedDir,
	})
	if err != nil {
		logger.Error("Failed to create SFTP server: %v", err)
		os.Exit(1)
	}

	if err := sftpServer.Start(); err != nil {
		logger.Error("Failed to start SFTP server: %v", err)
		os.Exit(1)
	}
	defer sftpServer.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start LAN beacon
	disc := discovery.NewDiscoveryService(
		cfg.BroadcastPort,
		cfg.DeviceID,
		cfg.DeviceName,
		cfg.PublicKey,
		cfg.VirtualIP,
		sftpServer.Port(),
		cfg.ListenPort,
	)

	if err := disc.Start(ctx); err != nil {
		logger.Warn("LAN discovery broadcast failed to start: %v", err)
	} else {
		defer disc.Stop()
	}

	// Register with WAN coordination server if configured
	if cfg.CoordServer != "" {
		coordClient := coord.NewClient(cfg.CoordServer, "", cfg)
		regResp, err := coordClient.Register(ctx)
		if err != nil {
			logger.Warn("WAN coordination server registration failed: %v", err)
		} else {
			logger.Info("Registered on WAN coordination server. Reflected endpoint: %s", regResp.ReflectedAddr)
			coordClient.StartHeartbeatLoop(ctx, 25*time.Second)
		}
	}

	mode := "interactive"
	if isDaemon {
		mode = "daemon"
	}
	logger.Info("Isthmus node active (%s mode). Shared folder: %s", mode, cfg.SharedDir)
	logger.Info("Press Ctrl+C to terminate.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down Isthmus node...")
}

func connectViaRouter(target string) (*fileserver.Client, string, error) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		return nil, "", fmt.Errorf("please run 'isthmus init' first: %w", err)
	}

	router := discovery.NewAutoRouter(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, target)
	if err != nil {
		return nil, "", err
	}

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint: routed.Addr,
	})
	if err != nil {
		routed.Conn.Close()
		return nil, "", fmt.Errorf("SFTP handshake over %s connection failed: %w", routed.Tier.String(), err)
	}

	return client, routed.Tier.String(), nil
}

func cmdBrowse(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus browse <peer-name-or-endpoint> [remote-path]")
		return
	}

	peerTarget := args[0]
	remotePath := "."
	if len(args) >= 2 {
		remotePath = args[1]
	}

	client, tier, err := connectViaRouter(peerTarget)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	logger.Info("Connected via %s transport.", tier)

	entries, err := client.List(remotePath)
	if err != nil {
		logger.Error("Failed to list remote directory %s: %v", remotePath, err)
		os.Exit(1)
	}

	fmt.Printf("\n=== Remote Files on %s [%s] (%s) ===\n", peerTarget, remotePath, tier)
	fmt.Printf("%-30s %-12s %-20s\n", "Name", "Size (bytes)", "Modified Time")
	fmt.Println("-------------------------------------------------------------------")
	for _, entry := range entries {
		typeMarker := ""
		if entry.IsDir() {
			typeMarker = "/"
		}
		name := entry.Name() + typeMarker
		fmt.Printf("%-30s %-12d %-20s\n", name, entry.Size(), entry.ModTime().Format("2006-01-02 15:04:05"))
	}
	fmt.Println()
}

func cmdPull(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: isthmus pull <peer> <remote-file> [local-destination]")
		os.Exit(1)
	}

	peerTarget := args[0]
	remoteFile := args[1]
	localDest := filepath.Base(remoteFile)
	if len(args) >= 3 {
		localDest = args[2]
	}

	client, tier, err := connectViaRouter(peerTarget)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	logger.Info("Connected via %s transport.", tier)
	logger.Info("Starting download: %s -> %s", remoteFile, localDest)

	lastReport := time.Now()
	checksum, err := client.PullFileResume(remoteFile, localDest, func(transferred, total int64, speed float64) {
		if time.Since(lastReport) >= 500*time.Millisecond || transferred == total {
			var percent float64
			if total > 0 {
				percent = float64(transferred) / float64(total) * 100.0
			}
			speedMB := speed / (1024 * 1024)
			fmt.Printf("\rProgress: %d/%d bytes (%.1f%%) at %.2f MB/s", transferred, total, percent, speedMB)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		logger.Error("Pull failed: %v", err)
		os.Exit(1)
	}

	logger.Info("Pull complete. SHA256 checksum: %s", checksum)
}

func cmdPush(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: isthmus push <peer> <local-file> [remote-destination]")
		os.Exit(1)
	}

	peerTarget := args[0]
	localFile := args[1]
	remoteDest := filepath.Base(localFile)
	if len(args) >= 3 {
		remoteDest = args[2]
	}

	client, tier, err := connectViaRouter(peerTarget)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	logger.Info("Connected via %s transport.", tier)
	logger.Info("Starting upload: %s -> %s", localFile, remoteDest)

	lastReport := time.Now()
	err = client.PushFile(localFile, remoteDest, func(transferred, total int64, speed float64) {
		if time.Since(lastReport) >= 500*time.Millisecond || transferred == total {
			var percent float64
			if total > 0 {
				percent = float64(transferred) / float64(total) * 100.0
			}
			speedMB := speed / (1024 * 1024)
			fmt.Printf("\rProgress: %d/%d bytes (%.1f%%) at %.2f MB/s", transferred, total, percent, speedMB)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		logger.Error("Push failed: %v", err)
		os.Exit(1)
	}

	logger.Info("Push complete.")
}

func cmdSync(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus sync <peer> [remote-dir] [local-dir]")
		os.Exit(1)
	}

	peerTarget := args[0]
	remoteDir := ""
	if len(args) >= 2 {
		remoteDir = args[1]
	}

	cfg, _ := config.LoadConfig("")
	localDir := "./sync_output"
	if cfg != nil && cfg.SharedDir != "" {
		localDir = cfg.SharedDir
	}
	if len(args) >= 3 {
		localDir = args[2]
	}

	client, tier, err := connectViaRouter(peerTarget)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	logger.Info("Connected via %s transport for folder synchronization.", tier)
	syncEngine := fileserver.NewSyncEngine(client)

	lastReport := time.Now()
	stats, err := syncEngine.SyncDirectory(remoteDir, localDir, fileserver.SyncOptions{Resume: true}, func(relPath string, current, total int64, doneFiles, totalFiles int) {
		if time.Since(lastReport) >= 200*time.Millisecond || doneFiles == totalFiles {
			fmt.Printf("\rSyncing [%d/%d files] %s (%d/%d bytes)", doneFiles, totalFiles, relPath, current, total)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		logger.Error("Sync failed: %v", err)
		os.Exit(1)
	}

	logger.Info("Folder sync complete. %d downloaded, %d skipped, %d bytes in %v",
		stats.FilesDownloaded, stats.FilesSkipped, stats.BytesTransferred, stats.Duration)
}

func cmdCoord(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus coord <status|set <url>>")
		return
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	switch args[0] {
	case "set":
		if len(args) < 2 {
			fmt.Println("Usage: isthmus coord set <url>")
			return
		}
		cfg.CoordServer = args[1]
		if err := cfg.Save(""); err != nil {
			logger.Error("Failed to save config: %v", err)
			return
		}
		logger.Info("Set coordination server to: %s", cfg.CoordServer)

	case "status":
		if cfg.CoordServer == "" {
			logger.Warn("Coordination server is not configured.")
			fmt.Println("Use 'isthmus coord set <url>' to configure.")
			return
		}

		logger.Info("Testing connection to %s...", cfg.CoordServer)
		client := coord.NewClient(cfg.CoordServer, "", cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stunResp, err := client.STUN(ctx)
		if err != nil {
			logger.Error("STUN reflection check failed: %v", err)
			return
		}
		fmt.Println("=== Coordination Server Status ===")
		fmt.Printf("Server URL:     %s\n", cfg.CoordServer)
		fmt.Printf("Status:         ONLINE\n")
		fmt.Printf("Reflected WAN:  %s\n", stunResp.ReflectedAddr)
	}
}

func cmdPeer(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus peer <list|add|remove>")
		return
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		fmt.Printf("Configured Peers (%d):\n", len(cfg.Peers))
		for id, peer := range cfg.Peers {
			fmt.Printf("  [%s] Name: %-15s MeshIP: %-15s Allowed: %v\n", id, peer.DeviceName, peer.VirtualIP, peer.Allowed)
		}
	case "add":
		if len(args) < 5 {
			fmt.Println("Usage: isthmus peer add <device-id> <device-name> <public-key> <virtual-ip>")
			return
		}
		peer := config.Peer{
			DeviceID:   args[1],
			DeviceName: args[2],
			PublicKey:  args[3],
			VirtualIP:  args[4],
			Allowed:    true,
		}
		if err := cfg.AddPeer(peer); err != nil {
			logger.Error("Failed to add peer: %v", err)
			return
		}
		if err := cfg.Save(""); err != nil {
			logger.Error("Failed to save config: %v", err)
			return
		}
		logger.Info("Added peer '%s' (%s)", peer.DeviceName, peer.DeviceID)
	case "remove", "rm":
		if len(args) < 2 {
			fmt.Println("Usage: isthmus peer remove <device-id>")
			return
		}
		cfg.RemovePeer(args[1])
		if err := cfg.Save(""); err != nil {
			logger.Error("Failed to save config: %v", err)
			return
		}
		logger.Info("Removed peer %s", args[1])
	}
}
