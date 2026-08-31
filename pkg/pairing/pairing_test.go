package pairing

import (
	"context"
	"testing"
	"time"

	"isthmus/pkg/config"
)

func TestPairingQRHandshake(t *testing.T) {
	// Node A
	cfgA, err := config.NewDefaultConfig("node-alpha")
	if err != nil {
		t.Fatalf("failed to init cfgA: %v", err)
	}

	// Node B
	cfgB, err := config.NewDefaultConfig("node-beta")
	if err != nil {
		t.Fatalf("failed to init cfgB: %v", err)
	}

	mgr := NewManager()

	// 1. Node A generates pairing session
	session, err := mgr.GenerateSession(cfgA, "127.0.0.1", 10*time.Second)
	if err != nil {
		t.Fatalf("failed to generate session: %v", err)
	}
	defer session.server.Close()

	if len(session.PIN) != 6 {
		t.Errorf("expected 6-digit PIN, got %s", session.PIN)
	}

	if session.QRURL == "" || session.QRBase64PNG == "" {
		t.Errorf("expected non-empty QR URL and PNG base64")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 2. Node B joins via QR URL
	peerOnB, err := mgr.JoinPairing(ctx, cfgB, session.QRURL)
	if err != nil {
		t.Fatalf("failed to join pairing: %v", err)
	}

	if peerOnB.DeviceID != cfgA.DeviceID {
		t.Errorf("expected peer ID %s, got %s", cfgA.DeviceID, peerOnB.DeviceID)
	}

	if peerOnB.PublicKey != cfgA.PublicKey {
		t.Errorf("expected public key %s, got %s", cfgA.PublicKey, peerOnB.PublicKey)
	}

	// 3. Node A receives Node B
	peerOnA, err := mgr.WaitForPairing(ctx, session)
	if err != nil {
		t.Fatalf("failed waiting for pairing on Node A: %v", err)
	}

	if peerOnA.DeviceID != cfgB.DeviceID {
		t.Errorf("expected peer ID %s on A, got %s", cfgB.DeviceID, peerOnA.DeviceID)
	}

	if peerOnA.PublicKey != cfgB.PublicKey {
		t.Errorf("expected public key %s on A, got %s", cfgB.PublicKey, peerOnA.PublicKey)
	}
}
