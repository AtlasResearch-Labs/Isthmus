package coord

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"isthmus/pkg/config"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestCoordClientAndServer(t *testing.T) {
	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to allocate port: %v", err)
	}

	devices := make(map[string]RegisterRequest)
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		devices[req.DeviceID] = req
		resp := RegisterResponse{
			Type:          MsgRegisterAck,
			Success:       true,
			AssignedIP:    req.VirtualIP,
			ReflectedAddr: "198.51.100.10:2222",
			Timestamp:     time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		resp := HeartbeatResponse{
			Type:          MsgHeartbeatAck,
			Success:       true,
			ReflectedAddr: "198.51.100.10:55432",
			Timestamp:     time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/stun", func(w http.ResponseWriter, r *http.Request) {
		resp := STUNResponse{
			Type:          MsgSTUNResponse,
			ReflectedIP:   "198.51.100.10",
			ReflectedPort: 55432,
			ReflectedAddr: "198.51.100.10:55432",
			Timestamp:     time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/peer-exchange", func(w http.ResponseWriter, r *http.Request) {
		var req PeerExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		resp := PeerExchangeResponse{
			Type:          MsgPeerUpdate,
			TargetDevice:  req.TargetDevice,
			TargetName:    "target-pc",
			PublicKey:     "targetPubKey123=",
			VirtualIP:     "10.77.0.2",
			PublicAddr:    "198.51.100.20:2222",
			RelayEndpoint: "198.51.100.1:8081",
			RelayEnabled:  true,
			LastSeen:      time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}
	go server.ListenAndServe()
	defer server.Close()

	time.Sleep(50 * time.Millisecond)

	cfg := &config.Config{
		DeviceID:   "my-device-id-1234567890123456",
		DeviceName: "boss-pc",
		PublicKey:  "myPubKey=",
		VirtualIP:  "10.77.0.1",
		ListenPort: 51820,
		SFTPPort:   2222,
	}

	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := NewClient(serverURL, "test-token", cfg)

	ctx := context.Background()

	// 1. Test Register
	regResp, err := client.Register(ctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if !regResp.Success || regResp.ReflectedAddr != "198.51.100.10:2222" {
		t.Fatalf("unexpected regResp: %+v", regResp)
	}

	// 2. Test Heartbeat
	hbResp, err := client.Heartbeat(ctx)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	if !hbResp.Success || hbResp.ReflectedAddr != "198.51.100.10:55432" {
		t.Fatalf("unexpected hbResp: %+v", hbResp)
	}

	// 3. Test STUN
	stunResp, err := client.STUN(ctx)
	if err != nil {
		t.Fatalf("STUN failed: %v", err)
	}
	if stunResp.ReflectedIP != "198.51.100.10" {
		t.Fatalf("unexpected STUN IP: %s", stunResp.ReflectedIP)
	}

	// 4. Test Peer Exchange
	exResp, err := client.ExchangePeer(ctx, "target-device-id")
	if err != nil {
		t.Fatalf("ExchangePeer failed: %v", err)
	}
	if exResp.PublicAddr != "198.51.100.20:2222" || !exResp.RelayEnabled {
		t.Fatalf("unexpected exResp: %+v", exResp)
	}
}
