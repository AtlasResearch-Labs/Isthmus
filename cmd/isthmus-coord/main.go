package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/coord"
	"isthmus/pkg/relay"
)

type DeviceRecord struct {
	DeviceID      string    `json:"device_id"`
	DeviceName    string    `json:"device_name"`
	PublicKey     string    `json:"public_key"`
	VirtualIP     string    `json:"virtual_ip"`
	ReflectedAddr string    `json:"reflected_addr"`
	ListenPort    int       `json:"listen_port"`
	SFTPPort      int       `json:"sftp_port"`
	LastSeen      time.Time `json:"last_seen"`
}

type CoordServer struct {
	mu        sync.RWMutex
	devices   map[string]DeviceRecord
	relayPort int
	log       *logger.Logger
}

func NewCoordServer(relayPort int) *CoordServer {
	return &CoordServer{
		devices:   make(map[string]DeviceRecord),
		relayPort: relayPort,
		log:       logger.WithPrefix("CoordServer"),
	}
}

func (s *CoordServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req coord.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}

	sftpPort := req.SFTPPort
	if sftpPort <= 0 {
		sftpPort = 2222
	}

	reflectedAddr := fmt.Sprintf("%s:%d", clientIP, sftpPort)

	s.mu.Lock()
	record := DeviceRecord{
		DeviceID:      req.DeviceID,
		DeviceName:    req.DeviceName,
		PublicKey:     req.PublicKey,
		VirtualIP:     req.VirtualIP,
		ReflectedAddr: reflectedAddr,
		ListenPort:    req.ListenPort,
		SFTPPort:      sftpPort,
		LastSeen:      time.Now(),
	}
	s.devices[req.DeviceID] = record
	s.mu.Unlock()

	s.log.Info("Registered device '%s' (%s) from %s", req.DeviceName, req.DeviceID, reflectedAddr)

	resp := coord.RegisterResponse{
		Type:          coord.MsgRegisterAck,
		Success:       true,
		AssignedIP:    req.VirtualIP,
		ReflectedAddr: reflectedAddr,
		Timestamp:     time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *CoordServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req coord.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	clientIP, clientPortStr, _ := net.SplitHostPort(r.RemoteAddr)
	clientPort, _ := strconv.Atoi(clientPortStr)

	s.mu.Lock()
	if dev, ok := s.devices[req.DeviceID]; ok {
		dev.LastSeen = time.Now()
		s.devices[req.DeviceID] = dev
	}
	s.mu.Unlock()

	resp := coord.HeartbeatResponse{
		Type:          coord.MsgHeartbeatAck,
		Success:       true,
		ReflectedAddr: fmt.Sprintf("%s:%d", clientIP, clientPort),
		Timestamp:     time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *CoordServer) handleSTUN(w http.ResponseWriter, r *http.Request) {
	clientIP, clientPortStr, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}
	clientPort, _ := strconv.Atoi(clientPortStr)

	resp := coord.STUNResponse{
		Type:          coord.MsgSTUNResponse,
		ReflectedIP:   clientIP,
		ReflectedPort: clientPort,
		ReflectedAddr: fmt.Sprintf("%s:%d", clientIP, clientPort),
		Timestamp:     time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *CoordServer) handlePeerExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req coord.PeerExchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	var target *DeviceRecord
	for id, dev := range s.devices {
		if id == req.TargetDevice || strings.EqualFold(dev.DeviceName, req.TargetDevice) {
			t := dev
			target = &t
			break
		}
	}
	s.mu.RUnlock()

	if target == nil {
		resp := coord.PeerExchangeResponse{
			Type:         coord.MsgPeerUpdate,
			TargetDevice: req.TargetDevice,
			Error:        "target device not found or offline",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	host, _, _ := net.SplitHostPort(r.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	relayEndpoint := fmt.Sprintf("%s:%d", host, s.relayPort)

	resp := coord.PeerExchangeResponse{
		Type:          coord.MsgPeerUpdate,
		TargetDevice:  target.DeviceID,
		TargetName:    target.DeviceName,
		PublicKey:     target.PublicKey,
		VirtualIP:     target.VirtualIP,
		PublicAddr:    target.ReflectedAddr,
		SFTPPort:      target.SFTPPort,
		TunnelPort:    target.ListenPort,
		RelayEndpoint: relayEndpoint,
		RelayEnabled:  true,
		LastSeen:      target.LastSeen,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *CoordServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]DeviceRecord, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, d)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func main() {
	port := flag.Int("port", 8080, "HTTP control plane listen port")
	relayPort := flag.Int("relay-port", 8081, "DERP packet relay listen port")
	flag.Parse()

	server := NewCoordServer(*relayPort)
	relayServer := relay.NewServer()

	// Start DERP relay server
	go func() {
		relayAddr := fmt.Sprintf("0.0.0.0:%d", *relayPort)
		if err := relayServer.ListenAndServe(relayAddr); err != nil {
			logger.Error("Relay server error: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", server.handleRegister)
	mux.HandleFunc("/api/v1/heartbeat", server.handleHeartbeat)
	mux.HandleFunc("/api/v1/stun", server.handleSTUN)
	mux.HandleFunc("/api/v1/peer-exchange", server.handlePeerExchange)
	mux.HandleFunc("/api/v1/devices", server.handleDevices)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	logger.Info("Isthmus coordination server starting on %s (Relay on :%d)", addr, *relayPort)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server listen failed: %v", err)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Coordination server shutting down...")
	httpServer.Close()
	relayServer.Close()
}
