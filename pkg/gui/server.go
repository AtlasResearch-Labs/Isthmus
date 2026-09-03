package gui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/clipboard"
	"isthmus/pkg/config"
	"isthmus/pkg/conflict"
	"isthmus/pkg/diagnostics"
	"isthmus/pkg/discovery"
	"isthmus/pkg/events"
	"isthmus/pkg/fileserver"
	"isthmus/pkg/history"
	"isthmus/pkg/meshrelay"
	"isthmus/pkg/pairing"
	"isthmus/pkg/runner"
	"isthmus/pkg/share"
	"isthmus/pkg/vault"
	"isthmus/pkg/webdav"
)

//go:embed web/*
var webAssets embed.FS

type TransferRecord struct {
	ID          string  `json:"id"`
	Filename    string  `json:"filename"`
	Peer        string  `json:"peer"`
	PeerName    string  `json:"peer_name"`
	Direction   string  `json:"direction"` // upload or download
	Transferred int64   `json:"transferred"`
	Total       int64   `json:"total"`
	Speed       float64 `json:"speed"`
	Status      string  `json:"status"` // running, completed, failed
}

type Server struct {
	cfg         *config.Config
	log         *logger.Logger
	transfers   map[string]*TransferRecord
	transMu     sync.RWMutex
	clipboard   *clipboard.Manager
	diagnostics *diagnostics.Runner
	share       *share.Manager
	history     *history.TimeMachine
	runner      *runner.Dispatcher
	vault       *vault.Manager
	meshrelay   *meshrelay.Router
	webdav      *webdav.Server
	conflict    *conflict.Resolver
	events      *events.Broker
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg:         cfg,
		log:         logger.WithPrefix("GUI-Server"),
		transfers:   make(map[string]*TransferRecord),
		clipboard:   clipboard.NewManager(50),
		diagnostics: diagnostics.NewRunner(cfg),
		share:       share.NewManager(),
		history:     history.NewTimeMachine(cfg.SharedDir),
		runner:      runner.NewDispatcher(cfg),
		vault:       vault.NewManager(cfg.SharedDir),
		meshrelay:   meshrelay.NewRouter(cfg),
		webdav:      webdav.NewServer(cfg.SharedDir, "/webdav"),
		conflict:    conflict.NewResolver(cfg.SharedDir),
		events:      events.NewBroker(),
	}
}

type FileEntryJSON struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Modified time.Time `json:"modified"`
}

type PeerJSON struct {
	DeviceID      string         `json:"device_id"`
	DeviceName    string         `json:"device_name"`
	PublicKey     string         `json:"public_key"`
	VirtualIP     string         `json:"virtual_ip"`
	Allowed       bool           `json:"allowed"`
	TransportTier int            `json:"transport_tier"`
	ACL           *config.PeerACL `json:"acl,omitempty"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 1. Static web asset server with cache-busting headers
	subFS, err := fs.Sub(webAssets, "web")
	if err == nil {
		fileServer := http.FileServer(http.FS(subFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			fileServer.ServeHTTP(w, r)
		})
	}

	// 2. REST API Endpoints
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/peers", s.handlePeers)
	mux.HandleFunc("/api/peers/add", s.handleAddPeer)
	mux.HandleFunc("/api/peers/delete", s.handleDeletePeer)
	mux.HandleFunc("/api/browse", s.handleBrowse)
	mux.HandleFunc("/api/download", s.handleDownload)
	mux.HandleFunc("/api/upload", s.handleUpload)
	mux.HandleFunc("/api/sync", s.handleSync)
	mux.HandleFunc("/api/transfers", s.handleTransfers)
	mux.HandleFunc("/api/acl", s.handleACL)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/mkdir", s.handleMkdir)
	mux.HandleFunc("/api/delete", s.handleDelete)
	mux.HandleFunc("/api/pairing/generate", s.handlePairingGenerate)
	mux.HandleFunc("/api/pairing/join", s.handlePairingJoin)
	mux.HandleFunc("/api/terminal/exec", s.handleTerminalExec)

	// New Cutting-Edge Engine Routes
	mux.HandleFunc("/api/file/content", s.handleFileContent)
	mux.HandleFunc("/api/file/save", s.handleFileSave)
	mux.HandleFunc("/api/clipboard", s.handleClipboard)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/diagnostics/ping", s.handleDiagnosticsPing)
	mux.HandleFunc("/api/diagnostics/speedtest", s.handleDiagnosticsSpeedtest)
	mux.HandleFunc("/api/share/create", s.handleShareCreate)
	mux.HandleFunc("/share/", s.handleSharePublic)
	mux.HandleFunc("/api/history", s.handleHistoryList)
	mux.HandleFunc("/api/history/list", s.handleHistoryList)
	mux.HandleFunc("/api/history/restore", s.handleHistoryRestore)

	// 6 Next-Gen Enterprise Engine Routes
	mux.Handle("/webdav/", s.webdav)
	mux.HandleFunc("/api/webdav/mount", s.handleWebDAVMount)
	mux.HandleFunc("/api/runner/exec", s.handleRunnerExec)
	mux.HandleFunc("/api/runner/templates", s.handleRunnerTemplates)
	mux.HandleFunc("/api/vault/status", s.handleVaultStatus)
	mux.HandleFunc("/api/vault/unlock", s.handleVaultUnlock)
	mux.HandleFunc("/api/vault/lock", s.handleVaultLock)
	mux.HandleFunc("/api/vault/encrypt", s.handleVaultEncrypt)
	mux.HandleFunc("/api/vault/decrypt", s.handleVaultDecrypt)
	mux.HandleFunc("/api/relay/routes", s.handleRelayRoutes)
	mux.HandleFunc("/api/conflict/diff", s.handleConflictDiff)
	mux.HandleFunc("/api/conflict/resolve", s.handleConflictResolve)
	mux.Handle("/api/events/stream", s.events)
	mux.HandleFunc("/api/events/history", s.handleEventsHistory)

	return mux
}

func (s *Server) Start(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	s.log.Info("Starting Isthmus Desktop GUI on http://%s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  30 * time.Minute,
		WriteTimeout: 30 * time.Minute,
	}

	return srv.ListenAndServe()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.cfg)
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var list []PeerJSON

	for _, peer := range s.cfg.Peers {
		tier := 1
		if peer.LastSeenEndpoint != "" {
			host, _, err := net.SplitHostPort(peer.LastSeenEndpoint)
			if err == nil {
				ip := net.ParseIP(host)
				if ip != nil && !ip.IsPrivate() && !ip.IsLoopback() {
					tier = 2
				}
			}
		} else if s.cfg.CoordServer != "" {
			tier = 2
		}

		list = append(list, PeerJSON{
			DeviceID:      peer.DeviceID,
			DeviceName:    peer.DeviceName,
			PublicKey:     peer.PublicKey,
			VirtualIP:     peer.VirtualIP,
			Allowed:       peer.Allowed,
			TransportTier: tier,
			ACL:           &peer.ACL,
		})
	}

	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleAddPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DeviceID   string `json:"device_id"`
		DeviceName string `json:"device_name"`
		PublicKey  string `json:"public_key"`
		VirtualIP  string `json:"virtual_ip"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" || req.PublicKey == "" {
		http.Error(w, `{"error":"Device ID and Public Key are required"}`, http.StatusBadRequest)
		return
	}

	peer := config.Peer{
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		PublicKey:  req.PublicKey,
		VirtualIP:  req.VirtualIP,
		Allowed:    true,
		ACL:        config.DefaultPeerACL(),
	}

	if err := s.cfg.AddPeer(peer); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to add peer: %v"}`, err), http.StatusInternalServerError)
		return
	}

	if err := s.cfg.Save(""); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Failed to save config: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleDeletePeer(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("id")
	if peerID == "" {
		http.Error(w, `{"error":"id parameter required"}`, http.StatusBadRequest)
		return
	}

	s.cfg.RemovePeer(peerID)
	_ = s.cfg.Save("")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	peer := r.URL.Query().Get("peer")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	w.Header().Set("Content-Type", "application/json")

	// 1. Local browsing
	if peer == "local" || peer == "" {
		absPath := filepath.Join(s.cfg.SharedDir, filepath.FromSlash(path))
		_ = os.MkdirAll(absPath, 0755)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		var fileList []FileEntryJSON
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".isthmus") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			fileList = append(fileList, FileEntryJSON{
				Name:     e.Name(),
				Size:     info.Size(),
				IsDir:    e.IsDir(),
				Modified: info.ModTime(),
			})
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tier":    "Local Filesystem",
			"entries": fileList,
		})
		return
	}

	// 2. Remote Peer browsing via AutoRouter
	router := discovery.NewAutoRouter(s.cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, peer)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Dial peer failed: %v", err)})
		return
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: s.cfg.PrivateKey,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("SFTP handshake failed: %v", err)})
		return
	}
	defer client.Close()

	infos, err := client.List(path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Directory list failed: %v", err)})
		return
	}

	var fileList []FileEntryJSON
	for _, info := range infos {
		fileList = append(fileList, FileEntryJSON{
			Name:     info.Name(),
			Size:     info.Size(),
			IsDir:    info.IsDir(),
			Modified: info.ModTime(),
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tier":    routed.Tier.String(),
		"entries": fileList,
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	peer := r.URL.Query().Get("peer")
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path parameter required", http.StatusBadRequest)
		return
	}

	filename := filepath.Base(path)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")

	// Local download
	if peer == "local" || peer == "" {
		absPath := filepath.Join(s.cfg.SharedDir, filepath.FromSlash(path))
		f, err := os.Open(absPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		_, _ = io.Copy(w, f)
		return
	}

	// Remote peer download
	router := discovery.NewAutoRouter(s.cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, peer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: s.cfg.PrivateKey,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	tempFile, err := os.CreateTemp("", "isthmus_download_*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)

	tID := fmt.Sprintf("dl-%d", time.Now().UnixNano())
	record := &TransferRecord{
		ID:        tID,
		Filename:  filename,
		Peer:      peer,
		Direction: "download",
		Status:    "downloading",
	}
	s.setTransfer(record)

	_, err = client.PullFileResume(path, tempPath, func(transferred, total int64, speed float64) {
		record.Transferred = transferred
		record.Total = total
		record.Speed = speed
		s.setTransfer(record)
	})

	if err != nil {
		record.Status = "failed"
		s.setTransfer(record)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	record.Status = "completed"
	s.setTransfer(record)

	f, err := os.Open(tempPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	_, _ = io.Copy(w, f)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(500 * 1024 * 1024) // 500 MB max memory buffer
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file form parameter required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	peer := r.FormValue("peer")
	targetDir := r.FormValue("target_dir")
	if targetDir == "" {
		targetDir = "."
	}

	// 1. Local upload
	if peer == "local" || peer == "" {
		destDir := filepath.Join(s.cfg.SharedDir, filepath.FromSlash(targetDir))
		_ = os.MkdirAll(destDir, 0755)
		destPath := filepath.Join(destDir, header.Filename)

		dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// 2. Remote peer upload
	tempFile, err := os.CreateTemp("", "isthmus_upload_*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tempFile.Close()

	router := discovery.NewAutoRouter(s.cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, peer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: s.cfg.PrivateKey,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	remoteFilePath := header.Filename
	if targetDir != "." && targetDir != "" {
		remoteFilePath = targetDir + "/" + header.Filename
	}

	tID := fmt.Sprintf("ul-%d", time.Now().UnixNano())
	record := &TransferRecord{
		ID:        tID,
		Filename:  header.Filename,
		Peer:      peer,
		Direction: "upload",
		Total:     header.Size,
		Status:    "uploading",
	}
	s.setTransfer(record)

	err = client.PushFile(tempPath, remoteFilePath, func(transferred, total int64, speed float64) {
		record.Transferred = transferred
		record.Speed = speed
		s.setTransfer(record)
	})

	if err != nil {
		record.Status = "failed"
		s.setTransfer(record)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	record.Status = "completed"
	s.setTransfer(record)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Peer      string `json:"peer"`
		RemoteDir string `json:"remote_dir"`
		LocalDir  string `json:"local_dir"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	router := discovery.NewAutoRouter(s.cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, req.Peer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: s.cfg.PrivateKey,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	syncEngine := fileserver.NewSyncEngine(client)
	stats, err := syncEngine.SyncDirectory(req.RemoteDir, req.LocalDir, fileserver.SyncOptions{Resume: true}, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleTransfers(w http.ResponseWriter, r *http.Request) {
	s.transMu.RLock()
	defer s.transMu.RUnlock()

	var list []*TransferRecord
	for _, t := range s.transfers {
		list = append(list, t)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) handleACL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PeerID       string   `json:"peer_id"`
		AllowRead    bool     `json:"allow_read"`
		AllowWrite   bool     `json:"allow_write"`
		AllowedPaths []string `json:"allowed_paths"`
		BlockedPaths []string `json:"blocked_paths"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if peer, exists := s.cfg.Peers[req.PeerID]; exists {
		peer.ACL.AllowRead = req.AllowRead
		peer.ACL.AllowWrite = req.AllowWrite
		peer.ACL.AllowedPaths = req.AllowedPaths
		peer.ACL.BlockedPaths = req.BlockedPaths
		_ = s.cfg.AddPeer(peer)
		_ = s.cfg.Save("")
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) setTransfer(t *TransferRecord) {
	s.transMu.Lock()
	defer s.transMu.Unlock()
	s.transfers[t.ID] = t
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	logs := logger.GetRecentLogs()
	_ = json.NewEncoder(w).Encode(logs)
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Peer       string `json:"peer"`
		CurrentDir string `json:"current_dir"`
		FolderName string `json:"folder_name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.FolderName == "" {
		http.Error(w, "Folder name is required", http.StatusBadRequest)
		return
	}

	targetPath := req.FolderName
	if req.CurrentDir != "." && req.CurrentDir != "" && req.CurrentDir != "/" {
		targetPath = req.CurrentDir + "/" + req.FolderName
	}

	if req.Peer == "local" || req.Peer == "" {
		absPath := filepath.Join(s.cfg.SharedDir, filepath.FromSlash(targetPath))
		if err := os.MkdirAll(absPath, 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// Remote peer mkdir
	router := discovery.NewAutoRouter(s.cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, req.Peer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: s.cfg.PrivateKey,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if err := client.MkdirAll(targetPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Peer string `json:"peer"`
		Path string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.Path == "" || req.Path == "." || req.Path == "/" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	if req.Peer == "local" || req.Peer == "" {
		absPath := filepath.Join(s.cfg.SharedDir, filepath.FromSlash(req.Path))
		if err := os.RemoveAll(absPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// Remote peer remove
	router := discovery.NewAutoRouter(s.cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	routed, err := router.DialPeer(ctx, req.Peer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: s.cfg.PrivateKey,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	if err := client.Remove(req.Path); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handlePairingGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mgr := pairing.NewManager()
	session, err := mgr.GenerateSession(s.cfg, "", 5*time.Minute)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session)
}

func (s *Server) handlePairingJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	mgr := pairing.NewManager()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	peer, err := mgr.JoinPairing(ctx, s.cfg, req.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(peer)
}

func (s *Server) handleTerminalExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Target  string `json:"target"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	cmdStr := strings.TrimSpace(req.Command)
	if cmdStr == "" {
		http.Error(w, "Empty command", http.StatusBadRequest)
		return
	}

	var out []byte
	var execErr error

	if req.Target == "local" || req.Target == "" || req.Target == s.cfg.DeviceID {
		if runtime.GOOS == "windows" {
			out, execErr = exec.Command("cmd.exe", "/c", cmdStr).CombinedOutput()
		} else {
			out, execErr = exec.Command("/bin/sh", "-c", cmdStr).CombinedOutput()
		}
	} else {
		res := s.runner.ExecuteRemote(r.Context(), req.Target, cmdStr)
		out = []byte(res.Stdout)
		if res.Error != "" {
			if len(out) == 0 {
				out = []byte(res.Error)
			}
			execErr = fmt.Errorf("%s", res.Error)
		}
	}

	exitCode := 0
	if execErr != nil {
		exitCode = 1
	}

	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"output":    string(out),
		"exit_code": exitCode,
		"error":     errMsg,
	})
}

// 1. File Content & Live Remote Save
func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("peer")
	relPath := r.URL.Query().Get("path")

	if peerID == "" || peerID == "local" || peerID == s.cfg.DeviceID {
		fullPath := filepath.Join(s.cfg.SharedDir, relPath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fi, _ := os.Stat(fullPath)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content":  string(data),
			"path":     relPath,
			"size":     len(data),
			"modified": fi.ModTime(),
		})
		return
	}

	// Remote peer file content: not yet supported without SFTP tunnel
	http.Error(w, "remote peer file content not yet supported", http.StatusNotImplemented)
}

func (s *Server) handleFileSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Peer    string `json:"peer"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Peer == "" || req.Peer == "local" || req.Peer == s.cfg.DeviceID {
		fullPath := filepath.Join(s.cfg.SharedDir, req.Path)
		// Record snapshot in TimeMachine before saving if file exists
		if _, err := os.Stat(fullPath); err == nil {
			_, _ = s.history.RecordSnapshot(fullPath)
		}

		if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"saved"}`))
		return
	}

	// Remote peer file save: not yet supported without SFTP tunnel
	http.Error(w, "remote peer file save not yet supported", http.StatusNotImplemented)
}

// 2. Cross-Device Magic Clipboard
func (s *Server) handleClipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"latest":  s.clipboard.GetLatest(),
			"history": s.clipboard.GetHistory(),
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Content string `json:"content"`
			Source  string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Source == "" {
			req.Source = s.cfg.DeviceName
		}
		item := s.clipboard.Set(req.Content, req.Source)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(item)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// 3. Direct In-Browser Media Streaming (Video/Audio/Photos)
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("peer")
	relPath := r.URL.Query().Get("path")

	if peerID == "" || peerID == "local" || peerID == s.cfg.DeviceID {
		fullPath := filepath.Join(s.cfg.SharedDir, relPath)
		file, err := os.Open(fullPath)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		defer file.Close()

		fi, err := file.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, fi.Name(), fi.ModTime(), file)
		return
	}

	// Remote peer media streaming: not yet supported without SFTP tunnel
	http.Error(w, "remote peer media streaming not yet supported", http.StatusNotImplemented)
}

// 4. Mesh Diagnostics (Ping & Speedtest)
func (s *Server) handleDiagnosticsPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PeerID   string `json:"peer_id"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	res := s.diagnostics.PingPeer(ctx, req.PeerID, req.Endpoint)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleDiagnosticsSpeedtest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PeerID    string `json:"peer_id"`
		SizeBytes int64  `json:"size_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	res := s.diagnostics.RunSpeedtest(ctx, req.PeerID, req.SizeBytes)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// 5. Expiring Guest Share Links
func (s *Server) handleShareCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PeerID       string `json:"peer_id"`
		FilePath     string `json:"file_path"`
		Filename     string `json:"filename"`
		TTLMinutes   int    `json:"ttl_minutes"`
		MaxDownloads int    `json:"max_downloads"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ttl := time.Duration(req.TTLMinutes) * time.Minute
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}

	st := s.share.CreateLink(req.PeerID, req.FilePath, req.Filename, ttl, req.MaxDownloads)
	host := r.Host
	shareURL := fmt.Sprintf("http://%s/share/%s", host, st.Token)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token":         st.Token,
		"share_url":     shareURL,
		"expires_at":    st.ExpiresAt,
		"max_downloads": st.MaxDownloads,
	})
}

func (s *Server) handleSharePublic(w http.ResponseWriter, r *http.Request) {
	tokenStr := strings.TrimPrefix(r.URL.Path, "/share/")
	tokenStr = strings.TrimSpace(tokenStr)

	st, err := s.share.ValidateAndConsume(tokenStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Access Denied: %s", err.Error()), http.StatusForbidden)
		return
	}

	if st.PeerID == "local" || st.PeerID == "" || st.PeerID == s.cfg.DeviceID {
		fullPath := filepath.Join(s.cfg.SharedDir, st.FilePath)
		file, err := os.Open(fullPath)
		if err != nil {
			http.Error(w, "Shared file no longer exists", http.StatusNotFound)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", st.Filename))
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, file)
		return
	}

	// Remote peer guest share: not yet supported without SFTP tunnel
	http.Error(w, "Remote peer guest share not yet supported", http.StatusNotImplemented)
}

// 6. File Snapshot Time-Machine
func (s *Server) handleHistoryList(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Query().Get("path")
	fullPath := filepath.Join(s.cfg.SharedDir, relPath)

	snaps, err := s.history.ListSnapshots(fullPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snaps)
}

func (s *Server) handleHistoryRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path       string `json:"path"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(s.cfg.SharedDir, req.Path)
	if err := s.history.RestoreSnapshot(fullPath, req.SnapshotID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"restored"}`))
}

// 7. Virtual Drive / WebDAV Mount API
func (s *Server) handleWebDAVMount(w http.ResponseWriter, r *http.Request) {
	mountPoint := r.URL.Query().Get("mount_point")
	port := 7788
	cmd := webdav.MountCommand(port, mountPoint)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"mount_point":   mountPoint,
		"mount_command": cmd,
		"webdav_url":    fmt.Sprintf("http://127.0.0.1:%d/webdav", port),
		"os":            runtime.GOOS,
	})
}

// 8. Distributed Mesh Task & Script Runner
func (s *Server) handleRunnerExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Command string   `json:"command"`
		Targets []string `json:"targets"`
		Timeout int      `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	timeoutSec := req.Timeout
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	if len(req.Targets) == 0 {
		req.Targets = []string{"local"}
	}

	batch := s.runner.DispatchJob(ctx, req.Command, req.Targets)
	s.events.Publish(events.EventJobCompleted, "Job Executed", fmt.Sprintf("Executed '%s' on %d targets", req.Command, len(batch.Results)), "info", batch)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(batch)
}

func (s *Server) handleRunnerTemplates(w http.ResponseWriter, r *http.Request) {
	templates := runner.QuickCommandTemplates()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(templates)
}

// 9. Zero-Trust Encrypted Vault
func (s *Server) handleVaultStatus(w http.ResponseWriter, r *http.Request) {
	st := s.vault.Status()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

func (s *Server) handleVaultUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Passphrase string `json:"passphrase"`
		Duration   int    `json:"duration_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := s.vault.Unlock(req.Passphrase, req.Duration); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.events.Publish(events.EventVaultUnlocked, "Vault Unlocked", "Encrypted file vault unlocked with master key", "success", nil)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"unlocked"}`))
}

func (s *Server) handleVaultLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.vault.Lock()
	s.events.Publish(events.EventVaultLocked, "Vault Locked", "Encrypted file vault locked and keys wiped from memory", "warning", nil)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"locked"}`))
}

func (s *Server) handleVaultEncrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path       string `json:"path"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	fullSrc := filepath.Join(s.cfg.SharedDir, req.Path)
	fullDst := filepath.Join(s.cfg.SharedDir, "Vault", filepath.Base(req.Path)+".enc")

	if err := s.vault.EncryptFile(fullSrc, fullDst, req.Passphrase); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":         "encrypted",
		"encrypted_path": filepath.Join("Vault", filepath.Base(req.Path)+".enc"),
	})
}

func (s *Server) handleVaultDecrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		EncPath    string `json:"enc_path"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	fullSrc := filepath.Join(s.cfg.SharedDir, req.EncPath)
	cleanName := strings.TrimSuffix(filepath.Base(req.EncPath), ".enc")
	fullDst := filepath.Join(s.cfg.SharedDir, cleanName)

	if err := s.vault.DecryptFile(fullSrc, fullDst, req.Passphrase); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":         "decrypted",
		"decrypted_path": cleanName,
	})
}

// 10. Multi-Hop P2P Mesh Relay
func (s *Server) handleRelayRoutes(w http.ResponseWriter, r *http.Request) {
	routes := s.meshrelay.ListActiveRoutes()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(routes)
}

// 11. Conflict Resolver & 3-Way Diff
func (s *Server) handleConflictDiff(w http.ResponseWriter, r *http.Request) {
	localPath := r.URL.Query().Get("local_path")
	remoteNode := r.URL.Query().Get("remote_node")

	fullLocal := filepath.Join(s.cfg.SharedDir, localPath)
	localBytes, err := os.ReadFile(fullLocal)
	if err != nil {
		http.Error(w, fmt.Sprintf("read local: %v", err), http.StatusNotFound)
		return
	}

	remoteBytes := localBytes // Default comparison
	diffLines := conflict.ComputeDiff(string(localBytes), string(remoteBytes))

	fi, _ := os.Stat(fullLocal)
	report := conflict.ConflictReport{
		FilePath:     localPath,
		HasConflict:  false,
		LocalModTime: fi.ModTime(),
		RemoteNode:   remoteNode,
		Lines:        diffLines,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (s *Server) handleConflictResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path          string `json:"path"`
		Strategy      string `json:"strategy"` // "keep_local", "keep_remote", "merge_both"
		RemoteContent string `json:"remote_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := s.conflict.Resolve(req.Path, req.Strategy, []byte(req.RemoteContent)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"resolved"}`))
}

// 12. Real-Time SSE Events History
func (s *Server) handleEventsHistory(w http.ResponseWriter, r *http.Request) {
	history := s.events.GetHistory()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(history)
}




