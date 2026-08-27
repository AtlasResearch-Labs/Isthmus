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
	"path/filepath"
	"sync"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/discovery"
	"isthmus/pkg/fileserver"
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
	cfg       *config.Config
	log       *logger.Logger
	transfers map[string]*TransferRecord
	transMu   sync.RWMutex
}

func NewServer(cfg *config.Config) *Server {
	return &Server{
		cfg:       cfg,
		log:       logger.WithPrefix("GUI-Server"),
		transfers: make(map[string]*TransferRecord),
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

	// 1. Static web asset server
	subFS, err := fs.Sub(webAssets, "web")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(subFS)))
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
