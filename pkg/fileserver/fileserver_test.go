package fileserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
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

func TestSFTPTransferIntegration(t *testing.T) {
	serverDir, err := os.MkdirTemp("", "isthmus_server_*")
	if err != nil {
		t.Fatalf("Failed to create server temp dir: %v", err)
	}
	defer os.RemoveAll(serverDir)

	clientDir, err := os.MkdirTemp("", "isthmus_client_*")
	if err != nil {
		t.Fatalf("Failed to create client temp dir: %v", err)
	}
	defer os.RemoveAll(clientDir)

	testFileName := "hello.txt"
	testContent := []byte("Isthmus secure transfer protocol test content. 1234567890.")
	testFilePath := filepath.Join(serverDir, testFileName)
	if err := os.WriteFile(testFilePath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	h := sha256.Sum256(testContent)
	expectedChecksum := hex.EncodeToString(h[:])

	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to allocate port: %v", err)
	}

	server, err := NewServer(ServerConfig{
		Port:    port,
		RootDir: serverDir,
	})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Server.Start failed: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	client, err := NewClient(ClientConfig{
		Endpoint: endpoint,
		Timeout:  3 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	entries, err := client.List("")
	if err != nil {
		t.Fatalf("Client.List failed: %v", err)
	}
	found := false
	for _, entry := range entries {
		if entry.Name() == testFileName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("test file '%s' not found in server directory listing", testFileName)
	}

	pulledFilePath := filepath.Join(clientDir, "pulled_hello.txt")
	checksum, err := client.PullFile(testFileName, pulledFilePath, nil)
	if err != nil {
		t.Fatalf("PullFile failed: %v", err)
	}

	if checksum != expectedChecksum {
		t.Fatalf("checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
	}

	pulledData, err := os.ReadFile(pulledFilePath)
	if err != nil {
		t.Fatalf("Failed to read pulled file: %v", err)
	}
	if !bytes.Equal(pulledData, testContent) {
		t.Fatal("pulled file data does not match original")
	}

	uploadName := "uploaded.txt"
	uploadContent := []byte("Upload from client back to server via Isthmus.")
	uploadLocalPath := filepath.Join(clientDir, "local_upload.txt")
	if err := os.WriteFile(uploadLocalPath, uploadContent, 0644); err != nil {
		t.Fatalf("Failed to create upload file: %v", err)
	}

	if err := client.PushFile(uploadLocalPath, uploadName, nil); err != nil {
		t.Fatalf("PushFile failed: %v", err)
	}

	serverUploadedPath := filepath.Join(serverDir, uploadName)
	serverUploadedData, err := os.ReadFile(serverUploadedPath)
	if err != nil {
		t.Fatalf("Failed to read uploaded file on server: %v", err)
	}
	if !bytes.Equal(serverUploadedData, uploadContent) {
		t.Fatal("uploaded file content mismatch on server")
	}
}
