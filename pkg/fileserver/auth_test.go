package fileserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"isthmus/pkg/identity"
)

func TestPublicKeyAuthentication(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_auth_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate Authorized Client KeyPair
	clientKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate client keypair: %v", err)
	}

	// Generate Rogue / Unauthorized Client KeyPair
	rogueKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate rogue keypair: %v", err)
	}

	// Start Server configured strictly with clientKP.PublicKey.String()
	server, err := NewServer(ServerConfig{
		Port:        0,
		RootDir:     filepath.Join(tempDir, "shared"),
		AllowedKeys: []string{clientKP.PublicKey.String()},
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer server.Stop()

	serverEndpoint := server.listener.Addr().String()

	// 1. Test authorized client connects successfully
	authorizedClient, err := NewClient(ClientConfig{
		Endpoint:   serverEndpoint,
		PrivateKey: clientKP.PrivateKey.String(),
		Timeout:    3 * time.Second,
	})
	if err != nil {
		t.Fatalf("authorized client failed to connect: %v", err)
	}
	defer authorizedClient.Close()

	// 2. Test rogue / unauthorized client is rejected
	rogueClient, err := NewClient(ClientConfig{
		Endpoint:   serverEndpoint,
		PrivateKey: rogueKP.PrivateKey.String(),
		Timeout:    2 * time.Second,
	})
	if err == nil {
		rogueClient.Close()
		t.Fatal("expected rogue client to be rejected by public key authentication, but connection succeeded")
	}

	// 3. Test unauthenticated client (no key) is rejected
	unauthClient, err := NewClient(ClientConfig{
		Endpoint: serverEndpoint,
		Timeout:  2 * time.Second,
	})
	if err == nil {
		unauthClient.Close()
		t.Fatal("expected unauthenticated client to be rejected, but connection succeeded")
	}
}
