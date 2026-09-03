package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/coord"
	"isthmus/pkg/discovery"
	"isthmus/pkg/fileserver"
	"isthmus/pkg/gui"
	"isthmus/pkg/mesh"
	"isthmus/pkg/pairing"
	"isthmus/pkg/runner"
	"isthmus/pkg/service"
	"isthmus/pkg/tui"
	"isthmus/pkg/turbo"
	"isthmus/pkg/vault"
	"isthmus/pkg/watcher"
	"isthmus/pkg/webdav"
)

const version = "0.5.0-phase4"

func printUsage() {
	fmt.Printf("%s\n", tui.RetroTitleBar("ISTHMUS - CROSS-DEVICE SECURE TUNNEL & FILE SYSTEM", 78))
	fmt.Printf("Version: %s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  isthmus <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init                  Initialize device identity and configuration")
	fmt.Println("  status                Display local node status and configuration")
	fmt.Println("  devices               List discovered and configured peer nodes")
	fmt.Println("  discover              Scan LAN for other active Isthmus nodes")
	fmt.Println("  serve                 Start local file server and LAN beacon")
	fmt.Println("  daemon                Run persistent background node service with WAN sync")
	fmt.Println("  ui <peer> [path]      Open Retro Windows interactive TUI file explorer [--at <endpoint>]")
	fmt.Println("  gui, app              Launch dedicated Retro Windows Desktop GUI")
	fmt.Println("  browse <peer> [path]  Browse remote files on a peer [--at <endpoint>]")
	fmt.Println("  pull <peer> <remote>  Pull a file from a peer [--at <endpoint>] [--limit-rate <rate>] [--turbo]")
	fmt.Println("  push <peer> <file>    Upload a local file to a peer [--at <endpoint>] [--limit-rate <rate>] [--turbo]")
	fmt.Println("  turbo <peer> <file>   High-performance parallel multi-stream upload/download/bench")
	fmt.Println("  sync <peer> [dir]     Synchronize a directory bidirectionally [--at <endpoint>]")
	fmt.Println("  watch [peer] [dir]    Live watch directory and auto-sync in real time [--at <endpoint>]")
	fmt.Println("  acl <set|list>        Configure access control policies for peers")
	fmt.Println("  mesh <sync|status>    Synchronize real-time N-device mesh tailnet")
	fmt.Println("  service <action>      Manage headless OS service (install/start/stop)")
	fmt.Println("  coord <set|status>    Manage coordination server connection")
	fmt.Println("  peer <add|list|rm>    Manage configured trusted peers")
	fmt.Println("  pair                  One-click PIN and QR code device pairing")
	fmt.Println("  pair-code             Display a 6-digit pairing PIN and QR code")
	fmt.Println("  pair-join <pin/qr>    Pair with another device using PIN or QR code")
	fmt.Println("  exec [opts] <cmd>     Execute command across mesh nodes or --all")
	fmt.Println("  vault <action>        Manage zero-trust encrypted file vault")
	fmt.Println("  mount [drive]         Mount mesh storage as a native virtual drive letter")
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
	case "devices":
		cmdDevices(os.Args[2:])
	case "discover":
		cmdDiscover(os.Args[2:])
	case "serve":
		cmdServe(os.Args[2:])
	case "daemon":
		cmdDaemon(os.Args[2:])
	case "ui", "tui":
		cmdUI(os.Args[2:])
	case "gui", "app":
		cmdGUI(os.Args[2:])
	case "browse":
		cmdBrowse(os.Args[2:])
	case "pull":
		cmdPull(os.Args[2:])
	case "push":
		cmdPush(os.Args[2:])
	case "turbo":
		cmdTurbo(os.Args[2:])
	case "sync":
		cmdSync(os.Args[2:])
	case "watch":
		cmdWatch(os.Args[2:])
	case "acl":
		cmdACL(os.Args[2:])
	case "mesh":
		cmdMesh(os.Args[2:])
	case "service":
		cmdService(os.Args[2:])
	case "coord":
		cmdCoord(os.Args[2:])
	case "peer":
		cmdPeer(os.Args[2:])
	case "pair":
		cmdPair(os.Args[2:])
	case "pair-code":
		cmdPairCode(os.Args[2:])
	case "pair-join":
		cmdPairJoin(os.Args[2:])
	case "exec":
		cmdExec(os.Args[2:])
	case "vault":
		cmdVault(os.Args[2:])
	case "mount":
		cmdMount(os.Args[2:])
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

	fmt.Println(tui.RetroTitleBar("LOCAL NODE CONFIGURATION", 78))
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
}

func cmdDevices(args []string) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	fmt.Println(tui.RetroTitleBar("ISTHMUS PEER DIRECTORY", 78))
	fmt.Printf("%-20s %-34s %-15s %-8s\n", "DEVICE NAME", "DEVICE ID", "MESH IP", "STATUS")
	fmt.Println(tui.RetroHorizontalDivider(78))

	if len(cfg.Peers) == 0 {
		fmt.Println("  (No peers configured. Run 'isthmus discover' or 'isthmus peer add')")
	} else {
		for id, peer := range cfg.Peers {
			status := "ALLOWED"
			if !peer.Allowed {
				status = "BLOCKED"
			}
			fmt.Printf("%-20s %-34s %-15s %-8s\n", peer.DeviceName, id, peer.VirtualIP, status)
		}
	}
	fmt.Println()
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
		fmt.Printf("  %s Name: %-15s ID: %-32s LAN: %s MeshIP: %s\n",
			tui.SymOK, peer.DeviceName, peer.DeviceID, peer.LANEndpoint, peer.VirtualIP)
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

	var allowedKeys []string
	for _, peer := range cfg.Peers {
		if peer.Allowed && peer.PublicKey != "" {
			allowedKeys = append(allowedKeys, peer.PublicKey)
		}
	}

	sftpServer, err := fileserver.NewServer(fileserver.ServerConfig{
		Port:        cfg.SFTPPort,
		RootDir:     cfg.SharedDir,
		AllowedKeys: allowedKeys,
		AllowedKeysFunc: func() []string {
			latestCfg, err := config.LoadConfig("")
			if err != nil {
				return allowedKeys
			}
			var keys []string
			for _, peer := range latestCfg.Peers {
				if peer.Allowed && peer.PublicKey != "" {
					keys = append(keys, peer.PublicKey)
				}
			}
			return keys
		},
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

	disc := discovery.NewDiscoveryService(
		cfg.BroadcastPort,
		cfg.DeviceID,
		cfg.DeviceName,
		cfg.PublicKey,
		cfg.VirtualIP,
		sftpServer.Port(),
		cfg.ListenPort,
	)

	disc.OnPeerDiscovered(func(peer discovery.DiscoveredPeer) {
		latestCfg, err := config.LoadConfig("")
		if err == nil {
			updated := false
			if p, exists := latestCfg.GetPeer(peer.DeviceID); exists {
				p.LastSeenEndpoint = peer.LANEndpoint
				p.LastSeenTime = peer.LastSeen
				_ = latestCfg.AddPeer(p)
				updated = true
			} else {
				for _, p := range latestCfg.Peers {
					if strings.EqualFold(p.DeviceName, peer.DeviceName) {
						p.LastSeenEndpoint = peer.LANEndpoint
						p.LastSeenTime = peer.LastSeen
						_ = latestCfg.AddPeer(p)
						updated = true
						break
					}
				}
			}
			if updated {
				_ = latestCfg.Save("")
			}
		}
	})

	if err := disc.Start(ctx); err != nil {
		logger.Warn("LAN discovery broadcast failed to start: %v", err)
	} else {
		defer disc.Stop()
	}

	// WebDAV service for native OS virtual drive mounting on port 7788
	wdServer := webdav.NewServer(cfg.SharedDir, "/webdav")
	wdListener, wdErr := net.Listen("tcp", "127.0.0.1:7788")
	if wdErr == nil {
		defer wdListener.Close()
		go func() {
			logger.Info("WebDAV virtual drive service listening on http://127.0.0.1:7788/webdav")
			_ = http.Serve(wdListener, wdServer)
		}()
	} else {
		logger.Debug("WebDAV service already active on port 7788")
	}

	if cfg.CoordServer != "" {
		coordClient := coord.NewClient(cfg.CoordServer, "", cfg)
		regResp, err := coordClient.Register(ctx)
		if err != nil {
			logger.Warn("WAN coordination registration failed: %v", err)
		} else {
			logger.Info("Registered on WAN coordination server (%s)", regResp.ReflectedAddr)
			coordClient.StartHeartbeatLoop(ctx, 25*time.Second)

			// Start tailnet mesh convergence loop
			tailnet := mesh.NewTailnetMesh(cfg, coordClient)
			tailnet.StartConvergenceLoop(ctx, 30*time.Second)
			defer tailnet.Stop()
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

func connectViaRouter(target string, directEndpoint ...string) (*fileserver.Client, string, error) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		return nil, "", fmt.Errorf("please run 'isthmus init' first: %w", err)
	}

	recordEndpoint := func(ep string) {
		if strings.HasPrefix(ep, "127.") || ep == "" {
			return
		}
		for id, p := range cfg.Peers {
			if strings.EqualFold(id, target) || strings.EqualFold(p.DeviceName, target) {
				p.LastSeenEndpoint = ep
				p.LastSeenTime = time.Now()
				_ = cfg.AddPeer(p)
				_ = cfg.Save("")
				break
			}
		}
	}

	if len(directEndpoint) > 0 && directEndpoint[0] != "" {
		endpoint := directEndpoint[0]
		logger.Info("Bypassing router, connecting directly to %s...", endpoint)
		conn, err := net.DialTimeout("tcp", endpoint, 8*time.Second)
		if err != nil {
			return nil, "", fmt.Errorf("direct connection to %s failed: %w", endpoint, err)
		}
		client, err := fileserver.NewClientFromConn(conn, fileserver.ClientConfig{
			Endpoint:   endpoint,
			PrivateKey: cfg.PrivateKey,
		})
		if err != nil {
			conn.Close()
			return nil, "", fmt.Errorf("SFTP handshake with %s failed: %w", endpoint, err)
		}
		recordEndpoint(endpoint)
		return client, "Direct Endpoint", nil
	}

	router := discovery.NewAutoRouter(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, target)
	if err != nil {
		return nil, "", err
	}

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: cfg.PrivateKey,
	})
	if err != nil {
		routed.Conn.Close()
		return nil, "", fmt.Errorf("SFTP handshake over %s connection failed: %w", routed.Tier.String(), err)
	}

	recordEndpoint(routed.Addr)
	return client, routed.Tier.String(), nil
}

func cmdUI(args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	atEndpoint := fs.String("at", "", "Direct peer endpoint (e.g. 192.168.1.6:2222)")
	fs.Parse(reorderFlags(args))

	remain := fs.Args()
	if len(remain) < 1 {
		fmt.Println("Usage: isthmus ui [--at <endpoint>] <peer-name-or-endpoint> [initial-path]")
		return
	}

	peerTarget := remain[0]
	initialPath := "."
	if len(remain) >= 2 {
		initialPath = remain[1]
	}

	client, tier, err := connectViaRouter(peerTarget, *atEndpoint)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	browser := tui.NewBrowser(client, peerTarget, tier, initialPath)
	if err := browser.Run(context.Background()); err != nil {
		logger.Error("TUI error: %v", err)
	}
}

func reorderFlags(args []string) []string {
	var flags []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			positionals = append(positionals, arg)
		}
	}
	return append(flags, positionals...)
}

func cmdBrowse(args []string) {
	fs := flag.NewFlagSet("browse", flag.ExitOnError)
	atEndpoint := fs.String("at", "", "Direct peer endpoint (e.g. 192.168.1.6:2222)")
	fs.Parse(reorderFlags(args))

	remain := fs.Args()
	if len(remain) < 1 {
		fmt.Println("Usage: isthmus browse [--at <endpoint>] <peer-name-or-endpoint> [remote-path]")
		return
	}

	peerTarget := remain[0]
	remotePath := "."
	if len(remain) >= 2 {
		remotePath = remain[1]
	}

	client, tier, err := connectViaRouter(peerTarget, *atEndpoint)
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

	fmt.Println()
	title := fmt.Sprintf("REMOTE FILES ON %s [%s] (%s)", strings.ToUpper(peerTarget), remotePath, tier)
	fmt.Println(tui.RetroTitleBar(title, 78))
	fmt.Printf("%-6s %-36s %-12s %-20s\n", "TYPE", "NAME", "SIZE", "MODIFIED")
	fmt.Println(tui.RetroHorizontalDivider(78))
	for _, entry := range entries {
		typeStr := tui.SymFile
		if entry.IsDir() {
			typeStr = tui.SymDir
		}
		name := entry.Name()
		sizeStr := fileserver.FormatBytes(entry.Size())
		if entry.IsDir() {
			sizeStr = "<DIR>"
		}
		fmt.Printf("%-6s %-36s %-12s %-20s\n", typeStr, name, sizeStr, entry.ModTime().Format("2006-01-02 15:04:05"))
	}
	fmt.Println()
}

func cmdPull(args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	limitRate := fs.String("limit-rate", "", "Limit transfer speed (e.g. 500k, 2M, 10M)")
	atEndpoint := fs.String("at", "", "Direct peer endpoint (e.g. 192.168.1.6:2222)")
	isTurbo := fs.Bool("turbo", false, "Use parallel multi-stream Turbo transfer engine")
	fs.Parse(reorderFlags(args))

	remain := fs.Args()
	if len(remain) < 2 {
		fmt.Println("Usage: isthmus pull [--at <endpoint>] [--limit-rate <rate>] [--turbo] <peer> <remote-file> [local-destination]")
		os.Exit(1)
	}

	peerTarget := remain[0]
	remoteFile := remain[1]
	localDest := filepath.Base(remoteFile)
	if len(remain) >= 3 {
		localDest = remain[2]
	}

	client, tier, err := connectViaRouter(peerTarget, *atEndpoint)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	if *limitRate != "" {
		bytesPerSec, err := fileserver.ParseRateLimit(*limitRate)
		if err == nil && bytesPerSec > 0 {
			logger.Info("Bandwidth throttled to %s/s", fileserver.FormatBytes(bytesPerSec))
		}
	}

	logger.Info("Connected via %s transport.", tier)
	logger.Info("Starting download: %s -> %s", remoteFile, localDest)

	if *isTurbo {
		logger.Info("Starting Turbo multi-stream parallel download (6 streams)...")
		lastReport := time.Now()
		prog, err := client.PullFileTurbo(remoteFile, localDest, 6, func(p turbo.TransferProgress) {
			if time.Since(lastReport) >= 150*time.Millisecond || p.CompletedChunks == p.TotalChunks {
				bar := fileserver.RenderProgressBar(p.TransferredBytes, p.TotalBytes, p.SpeedMBps*1024*1024, 25)
				fmt.Printf("\r%s [Turbo: %d/%d chunks]", bar, p.CompletedChunks, p.TotalChunks)
				lastReport = time.Now()
			}
		})
		fmt.Println()
		if err != nil {
			logger.Error("Turbo pull failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Turbo pull complete! %s downloaded at %.2f MB/s", fileserver.FormatBytes(prog.TotalBytes), prog.SpeedMBps)
		return
	}

	lastReport := time.Now()
	checksum, err := client.PullFileResume(remoteFile, localDest, func(transferred, total int64, speed float64) {
		if time.Since(lastReport) >= 200*time.Millisecond || transferred == total {
			bar := fileserver.RenderProgressBar(transferred, total, speed, 25)
			fmt.Printf("\r%s", bar)
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
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	limitRate := fs.String("limit-rate", "", "Limit transfer speed (e.g. 500k, 2M, 10M)")
	atEndpoint := fs.String("at", "", "Direct peer endpoint (e.g. 192.168.1.6:2222)")
	isTurbo := fs.Bool("turbo", false, "Use parallel multi-stream Turbo transfer engine")
	fs.Parse(reorderFlags(args))

	remain := fs.Args()
	if len(remain) < 2 {
		fmt.Println("Usage: isthmus push [--at <endpoint>] [--limit-rate <rate>] [--turbo] <peer> <local-file> [remote-destination]")
		os.Exit(1)
	}

	peerTarget := remain[0]
	localFile := remain[1]
	remoteDest := filepath.Base(localFile)
	if len(remain) >= 3 {
		remoteDest = remain[2]
	}

	client, tier, err := connectViaRouter(peerTarget, *atEndpoint)
	if err != nil {
		logger.Error("Connection to '%s' failed: %v", peerTarget, err)
		os.Exit(1)
	}
	defer client.Close()

	if *limitRate != "" {
		bytesPerSec, err := fileserver.ParseRateLimit(*limitRate)
		if err == nil && bytesPerSec > 0 {
			logger.Info("Bandwidth throttled to %s/s", fileserver.FormatBytes(bytesPerSec))
		}
	}

	logger.Info("Connected via %s transport.", tier)
	logger.Info("Starting upload: %s -> %s", localFile, remoteDest)

	if *isTurbo {
		logger.Info("Starting Turbo multi-stream parallel upload (6 streams)...")
		lastReport := time.Now()
		prog, err := client.PushFileTurbo(localFile, remoteDest, 6, func(p turbo.TransferProgress) {
			if time.Since(lastReport) >= 150*time.Millisecond || p.CompletedChunks == p.TotalChunks {
				bar := fileserver.RenderProgressBar(p.TransferredBytes, p.TotalBytes, p.SpeedMBps*1024*1024, 25)
				fmt.Printf("\r%s [Turbo: %d/%d chunks]", bar, p.CompletedChunks, p.TotalChunks)
				lastReport = time.Now()
			}
		})
		fmt.Println()
		if err != nil {
			logger.Error("Turbo push failed: %v", err)
			os.Exit(1)
		}
		logger.Info("Turbo push complete! %s uploaded at %.2f MB/s", fileserver.FormatBytes(prog.TotalBytes), prog.SpeedMBps)
		return
	}

	lastReport := time.Now()
	checksum, err := client.PushFileResume(localFile, remoteDest, func(transferred, total int64, speed float64) {
		if time.Since(lastReport) >= 200*time.Millisecond || transferred == total {
			bar := fileserver.RenderProgressBar(transferred, total, speed, 25)
			fmt.Printf("\r%s", bar)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		logger.Error("Push failed: %v", err)
		os.Exit(1)
	}

	logger.Info("Push complete. SHA256 checksum: %s", checksum)
}

func cmdTurbo(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  isthmus turbo <peer> <local-file> [remote-dest]      Upload file using parallel Turbo engine")
		fmt.Println("  isthmus turbo --pull <peer> <remote-file> [dest]     Download file using parallel Turbo engine")
		fmt.Println("  isthmus turbo bench <file>                           Run local parallel chunk pipeline benchmark")
		return
	}

	if args[0] == "bench" {
		if len(args) < 2 {
			fmt.Println("Usage: isthmus turbo bench <file>")
			return
		}
		filePath := args[1]
		engine := turbo.NewEngine(turbo.DefaultChunkSize, 6)
		fmt.Printf("Analyzing and chunking '%s'...\n", filePath)
		manifest, err := engine.GenerateManifest(filePath)
		if err != nil {
			logger.Error("Manifest error: %v", err)
			os.Exit(1)
		}
		fmt.Printf("[TURBO MANIFEST OK]\n")
		fmt.Printf("  File:         %s\n", manifest.Filename)
		fmt.Printf("  Total Size:   %s (%d bytes)\n", fileserver.FormatBytes(manifest.TotalSize), manifest.TotalSize)
		fmt.Printf("  Chunk Slices: %d (Slice Size: %s)\n", manifest.TotalChunks, fileserver.FormatBytes(manifest.ChunkSize))
		fmt.Printf("  SHA-256 Hash: %s\n", manifest.FileHash)
		return
	}

	if args[0] == "--pull" || args[0] == "-pull" {
		pullArgs := append([]string{"--turbo"}, args[1:]...)
		cmdPull(pullArgs)
		return
	}

	pushArgs := append([]string{"--turbo"}, args...)
	cmdPush(pushArgs)
}

func cmdSync(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	atEndpoint := fs.String("at", "", "Direct peer endpoint (e.g. 192.168.1.6:2222)")
	fs.Parse(reorderFlags(args))

	remain := fs.Args()
	if len(remain) < 1 {
		fmt.Println("Usage: isthmus sync [--at <endpoint>] <peer> [remote-dir] [local-dir]")
		os.Exit(1)
	}

	peerTarget := remain[0]
	remoteDir := ""
	if len(remain) >= 2 {
		remoteDir = remain[1]
	}

	cfg, _ := config.LoadConfig("")
	localDir := "./sync_output"
	if cfg != nil && cfg.SharedDir != "" {
		localDir = cfg.SharedDir
	}
	if len(remain) >= 3 {
		localDir = remain[2]
	}

	client, tier, err := connectViaRouter(peerTarget, *atEndpoint)
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
			fmt.Printf("\rSyncing [%d/%d files] %-30s", doneFiles, totalFiles, relPath)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		logger.Error("Sync failed: %v", err)
		os.Exit(1)
	}

	logger.Info("Sync completed. %d files downloaded, %d skipped, %s in %v",
		stats.FilesDownloaded, stats.FilesSkipped, fileserver.FormatBytes(stats.BytesTransferred), stats.Duration)
}

func cmdWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	atEndpoint := fs.String("at", "", "Direct peer endpoint (e.g. 192.168.1.6:2222)")
	fs.Parse(reorderFlags(args))

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	remain := fs.Args()
	watchDir := cfg.SharedDir
	var targetPeer string
	if len(remain) >= 1 {
		targetPeer = remain[0]
	}
	if len(remain) >= 2 {
		watchDir = remain[1]
	}

	fw, err := watcher.NewFolderWatcher(watchDir, watcher.Options{
		DebounceDelay: 500 * time.Millisecond,
	})
	if err != nil {
		logger.Error("Failed to initialize folder watcher: %v", err)
		os.Exit(1)
	}

	var clientMu sync.Mutex
	var activeClient *fileserver.Client

	getClient := func() (*fileserver.Client, error) {
		clientMu.Lock()
		defer clientMu.Unlock()
		if activeClient != nil {
			if _, err := activeClient.List("."); err == nil {
				return activeClient, nil
			}
			activeClient.Close()
			activeClient = nil
		}
		c, tier, err := connectViaRouter(targetPeer, *atEndpoint)
		if err != nil {
			return nil, err
		}
		logger.Info("Established live sync session with '%s' via %s", targetPeer, tier)
		activeClient = c
		return activeClient, nil
	}

	fw.OnChange(func(we watcher.WatchEvent) {
		logger.Info("[%s] %s (%s)", we.Type, we.RelPath, we.Timestamp.Format("15:04:05"))
		if targetPeer != "" {
			c, err := getClient()
			if err != nil {
				logger.Error("Sync connection to '%s' failed: %v", targetPeer, err)
				return
			}
			if we.Type == watcher.EventCreate || we.Type == watcher.EventModify {
				logger.Info("Syncing %s -> %s...", we.RelPath, targetPeer)
				if chk, pushErr := c.PushFileResume(we.Path, we.RelPath, nil); pushErr != nil {
					logger.Error("Sync error for %s: %v", we.RelPath, pushErr)
				} else {
					shortHash := chk
					if len(shortHash) > 8 {
						shortHash = shortHash[:8]
					}
					logger.Info("Sync OK: %s (SHA256: %s)", we.RelPath, shortHash)
				}
			} else if we.Type == watcher.EventDelete {
				logger.Info("Deleting remote %s on %s...", we.RelPath, targetPeer)
				if rmErr := c.Remove(we.RelPath); rmErr != nil {
					logger.Error("Remote delete error for %s: %v", we.RelPath, rmErr)
				} else {
					logger.Info("Remote delete OK: %s", we.RelPath)
				}
			}
		}
	})

	if err := fw.Start(); err != nil {
		logger.Error("Failed to start watcher: %v", err)
		os.Exit(1)
	}
	defer fw.Stop()

	fmt.Println()
	fmt.Println(tui.RetroTitleBar("REAL-TIME CONTINUOUS FILE WATCHER & AUTO-SYNC", 78))
	fmt.Printf("  Directory:   %s\n", watchDir)
	if targetPeer != "" {
		fmt.Printf("  Target Peer: %s (Auto-Syncing on save)\n", targetPeer)
		if *atEndpoint != "" {
			fmt.Printf("  Endpoint:    %s\n", *atEndpoint)
		}
	} else {
		fmt.Printf("  Target Peer: Local change monitor\n")
	}
	fmt.Println("  Press Ctrl+C to terminate...")
	fmt.Println()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	clientMu.Lock()
	if activeClient != nil {
		activeClient.Close()
	}
	clientMu.Unlock()
}

func cmdACL(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: isthmus acl <peer-id-or-name> <allow-read|allow-write|deny-write|scope <path>|block <path>>")
		return
	}

	peerTarget := args[0]
	action := strings.ToLower(args[1])

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	peer, ok := cfg.GetPeer(peerTarget)
	if !ok {
		found := false
		for id, p := range cfg.Peers {
			if strings.EqualFold(p.DeviceName, peerTarget) {
				peer = p
				peerTarget = id
				found = true
				break
			}
		}
		if !found {
			logger.Error("Peer '%s' not found in config.", peerTarget)
			return
		}
	}

	if !peer.ACL.AllowRead && !peer.ACL.AllowWrite && len(peer.ACL.AllowedPaths) == 0 && len(peer.ACL.BlockedPaths) == 0 {
		peer.ACL = config.DefaultPeerACL()
	}

	switch action {
	case "allow-read":
		peer.ACL.AllowRead = true
	case "deny-read":
		peer.ACL.AllowRead = false
	case "allow-write":
		peer.ACL.AllowWrite = true
	case "deny-write":
		peer.ACL.AllowWrite = false
	case "scope":
		if len(args) < 3 {
			fmt.Println("Usage: isthmus acl <peer> scope <path>")
			return
		}
		peer.ACL.AllowedPaths = append(peer.ACL.AllowedPaths, args[2])
	case "block":
		if len(args) < 3 {
			fmt.Println("Usage: isthmus acl <peer> block <path>")
			return
		}
		peer.ACL.BlockedPaths = append(peer.ACL.BlockedPaths, args[2])
	default:
		logger.Error("Unknown ACL action: %s", action)
		return
	}

	cfg.Peers[peerTarget] = peer
	if err := cfg.Save(""); err != nil {
		logger.Error("Failed to save config: %v", err)
		return
	}

	logger.Info("Updated ACL for peer '%s': Read=%v, Write=%v, Scopes=%v, Blocked=%v",
		peer.DeviceName, peer.ACL.AllowRead, peer.ACL.AllowWrite, peer.ACL.AllowedPaths, peer.ACL.BlockedPaths)
}

func cmdMesh(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus mesh <sync|status>")
		return
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	if cfg.CoordServer == "" {
		logger.Error("No coordination server configured. Run 'isthmus coord set <url>' first.")
		return
	}

	coordClient := coord.NewClient(cfg.CoordServer, "", cfg)
	tailnet := mesh.NewTailnetMesh(cfg, coordClient)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch args[0] {
	case "sync", "status":
		logger.Info("Querying mesh tailnet from %s...", cfg.CoordServer)
		nodes, err := tailnet.SyncOnce(ctx)
		if err != nil {
			logger.Error("Mesh sync failed: %v", err)
			return
		}

		fmt.Println()
		fmt.Println(tui.RetroTitleBar("ISTHMUS MESH TAILNET TOPOLOGY", 78))
		fmt.Printf("%-18s %-32s %-15s %-20s\n", "NODE NAME", "DEVICE ID", "VIRTUAL IP", "ENDPOINT")
		fmt.Println(tui.RetroHorizontalDivider(78))
		for _, n := range nodes {
			tag := ""
			if n.IsSelf {
				tag = " (self)"
			}
			fmt.Printf("%-18s %-32s %-15s %-20s\n",
				n.DeviceName+tag, n.DeviceID, n.VirtualIP, n.ReflectedAddr)
		}
		fmt.Println()
		logger.Info("Mesh synchronized. Total active nodes: %d", len(nodes))
	default:
		fmt.Println("Usage: isthmus mesh <sync|status>")
	}
}

func cmdService(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus service <install|start|stop|status|uninstall>")
		return
	}

	action := strings.ToLower(args[0])
	mgr := service.NewManager()

	switch action {
	case "install":
		logger.Info("Installing Isthmus background service...")
		if err := mgr.Install("", ""); err != nil {
			logger.Error("Install failed: %v", err)
			return
		}
		logger.Info("Service installed successfully. Use 'isthmus service start' to launch.")
	case "start":
		logger.Info("Starting Isthmus service...")
		if err := mgr.Start(); err != nil {
			logger.Error("Start failed: %v", err)
			return
		}
		logger.Info("Service started.")
	case "stop":
		logger.Info("Stopping Isthmus service...")
		if err := mgr.Stop(); err != nil {
			logger.Error("Stop failed: %v", err)
			return
		}
		logger.Info("Service stopped.")
	case "status":
		status, err := mgr.Status()
		if err != nil {
			logger.Error("Status error: %v", err)
		}
		fmt.Println()
		fmt.Println(tui.RetroTitleBar("ISTHMUS SYSTEM SERVICE STATUS", 78))
		fmt.Println(status)
		fmt.Println()
	case "uninstall":
		logger.Info("Uninstalling Isthmus service...")
		if err := mgr.Uninstall(); err != nil {
			logger.Error("Uninstall failed: %v", err)
			return
		}
		logger.Info("Service uninstalled successfully.")
	default:
		fmt.Println("Usage: isthmus service <install|start|stop|status|uninstall>")
	}
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
		fmt.Println()
		fmt.Println(tui.RetroTitleBar("COORDINATION SERVER STATUS", 78))
		fmt.Printf("Server URL:     %s\n", cfg.CoordServer)
		fmt.Printf("Status:         %s ONLINE\n", tui.SymOK)
		fmt.Printf("Reflected WAN:  %s\n", stunResp.ReflectedAddr)
		fmt.Println()
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
		fmt.Println()
		fmt.Println(tui.RetroTitleBar("CONFIGURED TRUSTED PEERS", 78))
		for id, peer := range cfg.Peers {
			fmt.Printf("  [%s] Name: %-15s MeshIP: %-15s Allowed: %v\n", id, peer.DeviceName, peer.VirtualIP, peer.Allowed)
		}
		fmt.Println()
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
			ACL:        config.DefaultPeerACL(),
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
	default:
		fmt.Println("Usage: isthmus peer <list|add|remove> [args...]")
	}
}

func cmdPair(args []string) {
	if len(args) == 0 || args[0] == "code" {
		cmdPairCode(args)
		return
	}
	if args[0] == "join" {
		cmdPairJoin(args[1:])
		return
	}
	cmdPairJoin(args)
}

func cmdPairCode(args []string) {
	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	mgr := pairing.NewManager()
	session, err := mgr.GenerateSession(cfg, "", 3*time.Minute)
	if err != nil {
		logger.Error("Failed to generate pairing session: %v", err)
		os.Exit(1)
	}
	defer session.Close()

	fmt.Println()
	fmt.Println(tui.RetroTitleBar("ONE-CLICK MAGIC PAIRING", 78))
	fmt.Printf("  6-Digit PIN:       \033[1;34m%s\033[0m\n", session.PIN)
	fmt.Printf("  Expires In:        3 minutes\n")
	fmt.Printf("  QR URL:            %s\n", session.QRURL)
	fmt.Println()
	if session.ASCIIQR != "" {
		fmt.Println("  Scan with phone camera or mobile Isthmus app:")
		fmt.Println(session.ASCIIQR)
	}
	fmt.Println()
	fmt.Printf("  Run on other device: isthmus pair-join %s\n", session.PIN)
	fmt.Println("  Waiting for other device to connect (Press Ctrl+C to cancel)...")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	peer, err := mgr.WaitForPairing(ctx, session)
	if err != nil {
		logger.Error("Pairing failed or timed out: %v", err)
		return
	}

	fmt.Println()
	logger.Info("Successfully paired with '%s' (%s)!", peer.DeviceName, peer.DeviceID)
	logger.Info("Device added to trusted peer directory. Virtual IP: %s", peer.VirtualIP)
}

func cmdPairJoin(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: isthmus pair-join <6-digit-pin-or-qr-url>")
		return
	}
	target := args[0]

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	mgr := pairing.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Info("Attempting one-click handshake with '%s'...", target)
	peer, err := mgr.JoinPairing(ctx, cfg, target)
	if err != nil {
		logger.Error("Failed to pair: %v", err)
		os.Exit(1)
	}

	fmt.Println()
	logger.Info("Successfully paired with '%s' (%s)!", peer.DeviceName, peer.DeviceID)
	logger.Info("Device added to trusted peer directory. Virtual IP: %s", peer.VirtualIP)
}

func cmdGUI(args []string) {
	fs := flag.NewFlagSet("gui", flag.ExitOnError)
	port := fs.Int("port", 7788, "HTTP port for GUI web server")
	noOpen := fs.Bool("no-open", false, "Do not automatically open the web browser")
	fs.Parse(args)

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first: %v", err)
		os.Exit(1)
	}

	guiServer := gui.NewServer(cfg)
	url := fmt.Sprintf("http://127.0.0.1:%d", *port)

	// Start local node SFTP server & discovery beacon in background if not already running
	go func() {
		var allowedKeys []string
		for _, peer := range cfg.Peers {
			if peer.Allowed && peer.PublicKey != "" {
				allowedKeys = append(allowedKeys, peer.PublicKey)
			}
		}

		sftpServer, err := fileserver.NewServer(fileserver.ServerConfig{
			Port:        cfg.SFTPPort,
			RootDir:     cfg.SharedDir,
			AllowedKeys: allowedKeys,
			AllowedKeysFunc: func() []string {
				latestCfg, err := config.LoadConfig("")
				if err != nil {
					return allowedKeys
				}
				var keys []string
				for _, peer := range latestCfg.Peers {
					if peer.Allowed && peer.PublicKey != "" {
						keys = append(keys, peer.PublicKey)
					}
				}
				return keys
			},
		})
		if err == nil && sftpServer.Start() == nil {
			defer sftpServer.Stop()
			disc := discovery.NewDiscoveryService(
				cfg.BroadcastPort,
				cfg.DeviceID,
				cfg.DeviceName,
				cfg.PublicKey,
				cfg.VirtualIP,
				sftpServer.Port(),
				cfg.ListenPort,
			)
			disc.OnPeerDiscovered(func(peer discovery.DiscoveredPeer) {
				latestCfg, err := config.LoadConfig("")
				if err == nil {
					updated := false
					if p, exists := latestCfg.GetPeer(peer.DeviceID); exists {
						p.LastSeenEndpoint = peer.LANEndpoint
						p.LastSeenTime = peer.LastSeen
						_ = latestCfg.AddPeer(p)
						updated = true
					} else {
						for _, p := range latestCfg.Peers {
							if strings.EqualFold(p.DeviceName, peer.DeviceName) {
								p.LastSeenEndpoint = peer.LANEndpoint
								p.LastSeenTime = peer.LastSeen
								_ = latestCfg.AddPeer(p)
								updated = true
								break
							}
						}
					}
					if updated {
						_ = latestCfg.Save("")
					}
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := disc.Start(ctx); err == nil {
				defer disc.Stop()
			}
			select {}
		}
	}()

	if !*noOpen {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	logger.Info("Starting Isthmus Desktop GUI on %s", url)
	logger.Info("Press Ctrl+C to stop the GUI server.")

	if err := guiServer.Start(*port); err != nil {
		logger.Error("GUI server error: %v", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func cmdExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	target := fs.String("target", "local", "Target peer ID or 'all' for all mesh nodes")
	allNodes := fs.Bool("all", false, "Execute across all mesh nodes simultaneously")
	timeout := fs.Int("timeout", 10, "Command timeout in seconds")
	fs.Parse(args)

	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		fmt.Println("Usage: isthmus exec [--target=<peer>|--all] \"<command>\"")
		os.Exit(1)
	}

	commandStr := strings.Join(cmdArgs, " ")

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first: %v", err)
		os.Exit(1)
	}

	disp := runner.NewDispatcher(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	targets := []string{*target}
	if *allNodes {
		targets = []string{"all"}
	}

	fmt.Printf("[RUNNER] Dispatching '%s' across target(s): %v\n\n", commandStr, targets)
	batch := disp.DispatchJob(ctx, commandStr, targets)

	for _, r := range batch.Results {
		fmt.Printf("=== Node: %s (%s) [Exit Code: %d, Runtime: %.2f ms] ===\n", r.TargetName, r.TargetID, r.ExitCode, r.DurationMs)
		if r.Stdout != "" {
			fmt.Print(r.Stdout)
			if !strings.HasSuffix(r.Stdout, "\n") {
				fmt.Println()
			}
		}
		if r.Error != "" {
			fmt.Printf("[ERROR] %s\n", r.Error)
		}
		fmt.Println()
	}
}

func cmdVault(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: isthmus vault <status|encrypt|decrypt|lock|unlock> [file] [passphrase]")
		os.Exit(0)
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first: %v", err)
		os.Exit(1)
	}

	vm := vault.NewManager(cfg.SharedDir)
	subcmd := args[0]

	switch subcmd {
	case "status":
		st := vm.Status()
		fmt.Printf("=== Isthmus Zero-Trust Encrypted Vault ===\n")
		fmt.Printf("Directory:       %s\n", st.VaultDirectory)
		fmt.Printf("Encrypted Files: %d\n", st.EncryptedFiles)
		fmt.Printf("Lock State:      %v\n", map[bool]string{true: "UNLOCKED", false: "LOCKED"}[st.Unlocked])

	case "encrypt":
		if len(args) < 3 {
			fmt.Println("Usage: isthmus vault encrypt <src-file> <passphrase>")
			os.Exit(1)
		}
		srcFile := args[1]
		pass := args[2]
		dstFile := filepath.Join(cfg.SharedDir, "Vault", filepath.Base(srcFile)+".enc")
		if err := vm.EncryptFile(srcFile, dstFile, pass); err != nil {
			logger.Error("Encryption failed: %v", err)
			os.Exit(1)
		}
		fmt.Printf("[VAULT OK] Encrypted '%s' -> '%s' (AES-256-GCM)\n", srcFile, dstFile)

	case "decrypt":
		if len(args) < 3 {
			fmt.Println("Usage: isthmus vault decrypt <enc-file> <passphrase>")
			os.Exit(1)
		}
		encFile := args[1]
		pass := args[2]
		cleanName := strings.TrimSuffix(filepath.Base(encFile), ".enc")
		dstFile := filepath.Join(cfg.SharedDir, cleanName)
		if err := vm.DecryptFile(encFile, dstFile, pass); err != nil {
			logger.Error("Decryption failed: %v", err)
			os.Exit(1)
		}
		fmt.Printf("[VAULT OK] Decrypted '%s' -> '%s'\n", encFile, dstFile)

	case "unlock":
		if len(args) < 2 {
			fmt.Println("Usage: isthmus vault unlock <passphrase> [minutes]")
			os.Exit(1)
		}
		pass := args[1]
		if err := vm.Unlock(pass, 30); err != nil {
			logger.Error("Unlock failed: %v", err)
			os.Exit(1)
		}
		fmt.Println("[VAULT OK] Vault unlocked for 30 minutes.")

	case "lock":
		vm.Lock()
		fmt.Println("[VAULT OK] Vault locked. Master keys wiped from memory.")

	default:
		fmt.Printf("Unknown vault action: %s\n", subcmd)
	}
}

func cmdMount(args []string) {
	drive := "Z:"
	unmount := false
	for _, arg := range args {
		if arg == "--unmount" || arg == "-u" || arg == "unmount" {
			unmount = true
		} else if !strings.HasPrefix(arg, "-") {
			drive = arg
		}
	}

	cfg, err := config.LoadConfig("")
	if err != nil {
		logger.Error("Please run 'isthmus init' first.")
		os.Exit(1)
	}

	if runtime.GOOS == "windows" {
		drive = strings.ToUpper(strings.TrimSuffix(drive, ":")) + ":"
	}

	if unmount {
		fmt.Printf("Disconnecting Isthmus virtual drive '%s'...\n", drive)
		if runtime.GOOS == "windows" {
			_ = exec.Command("subst", drive, "/d").Run()
			_ = exec.Command("net", "use", drive, "/delete", "/y").Run()
		} else if runtime.GOOS == "darwin" {
			_ = exec.Command("umount", drive).Run()
		} else {
			_ = exec.Command("sudo", "umount", drive).Run()
		}
		fmt.Printf("[OK] Virtual drive '%s' successfully disconnected.\n", drive)
		return
	}

	// Verify WebDAV service is active on port 7788
	conn, err := net.DialTimeout("tcp", "127.0.0.1:7788", 500*time.Millisecond)
	if err != nil {
		wdServer := webdav.NewServer(cfg.SharedDir, "/webdav")
		wdListener, err := net.Listen("tcp", "127.0.0.1:7788")
		if err == nil {
			go func() {
				_ = http.Serve(wdListener, wdServer)
			}()
		}
	} else {
		conn.Close()
	}

	fmt.Printf("Mounting Isthmus mesh storage as native virtual drive '%s'...\n", drive)
	if runtime.GOOS == "windows" {
		cmd := exec.Command("subst", drive, cfg.SharedDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			outputStr := strings.TrimSpace(string(out))
			if strings.Contains(outputStr, "already") {
				fmt.Printf("[OK] Drive '%s' is already mounted and accessible.\n", drive)
			} else {
				logger.Error("Mount failed: %s (%v)", outputStr, err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("[OK] Successfully mounted Isthmus storage as drive '%s'!\n", drive)
			fmt.Printf("  Local Drive: %s\\ -> %s\n", drive, cfg.SharedDir)
			fmt.Printf("  WebDAV LAN:  http://127.0.0.1:7788/webdav\n")
		}
		return
	}

	// macOS / Linux mount
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("mkdir -p %s && mount_webdav -i http://127.0.0.1:7788/webdav %s", drive, drive))
	} else {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("sudo mkdir -p %s && sudo mount -t davfs http://127.0.0.1:7788/webdav %s", drive, drive))
	}

	out, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(out))
	if err != nil {
		if strings.Contains(outputStr, "already mounted") {
			fmt.Printf("[OK] Drive '%s' is mounted and accessible.\n", drive)
		} else {
			logger.Error("Mount failed: %s (%v)", outputStr, err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("[OK] Successfully mounted Isthmus mesh storage to '%s'!\n", drive)
		fmt.Printf("Access via: http://127.0.0.1:7788/webdav\n")
	}
}


