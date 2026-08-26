package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

func getFreeUDPPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port, nil
}

func TestDiscoveryPacketParsingAndCallback(t *testing.T) {
	port, err := getFreeUDPPort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	svc := NewDiscoveryService(port, "node-local-id", "LocalNode", "pubkeyLocal", "10.77.0.1", 2222, 51820)

	discoveredChan := make(chan DiscoveredPeer, 5)
	svc.OnPeerDiscovered(func(peer DiscoveredPeer) {
		discoveredChan <- peer
	})

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Failed to start discovery service: %v", err)
	}
	defer svc.Stop()

	time.Sleep(100 * time.Millisecond)

	// Simulate incoming beacon packet from remote peer
	remotePacket := AnnouncePacket{
		Magic:      BroadcastMagic,
		DeviceID:   "node-remote-id",
		DeviceName: "RemoteNode",
		PublicKey:  "remotePubKey123=",
		VirtualIP:  "10.77.0.2",
		SFTPPort:   2223,
		TunnelPort: 51821,
		Timestamp:  time.Now().Unix(),
	}

	data, err := json.Marshal(remotePacket)
	if err != nil {
		t.Fatalf("Failed to marshal remote packet: %v", err)
	}

	senderConn, err := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("Failed to dial discovery port: %v", err)
	}
	defer senderConn.Close()

	if _, err := senderConn.Write(data); err != nil {
		t.Fatalf("Failed to send packet: %v", err)
	}

	select {
	case peer := <-discoveredChan:
		if peer.DeviceName != "RemoteNode" {
			t.Fatalf("expected peer name 'RemoteNode', got '%s'", peer.DeviceName)
		}
		if peer.DeviceID != "node-remote-id" {
			t.Fatalf("expected device ID 'node-remote-id', got '%s'", peer.DeviceID)
		}
		if peer.VirtualIP != "10.77.0.2" {
			t.Fatalf("expected virtual IP '10.77.0.2', got '%s'", peer.VirtualIP)
		}
		if peer.SFTPPort != 2223 {
			t.Fatalf("expected SFTP port 2223, got %d", peer.SFTPPort)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for peer discovery callback")
	}

	peers := svc.GetDiscoveredPeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 cached peer, got %d", len(peers))
	}
}
