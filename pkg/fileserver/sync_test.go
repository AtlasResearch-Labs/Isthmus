package fileserver

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncDirectoryAndResume(t *testing.T) {
	serverDir, err := os.MkdirTemp("", "isthmus_sync_srv_*")
	if err != nil {
		t.Fatalf("Failed to create server temp dir: %v", err)
	}
	defer os.RemoveAll(serverDir)

	clientDir, err := os.MkdirTemp("", "isthmus_sync_cli_*")
	if err != nil {
		t.Fatalf("Failed to create client temp dir: %v", err)
	}
	defer os.RemoveAll(clientDir)

	// Create nested structure on server
	// serverDir/
	//   doc1.txt
	//   sub/
	//     doc2.txt
	//     nested/
	//       doc3.txt
	if err := os.MkdirAll(filepath.Join(serverDir, "sub", "nested"), 0755); err != nil {
		t.Fatalf("Failed to create server subdirs: %v", err)
	}

	f1Content := []byte("Document 1 Content - root file.")
	f2Content := []byte("Document 2 Content - in subfolder.")
	f3Content := []byte("Document 3 Content - deep nested.")

	if err := os.WriteFile(filepath.Join(serverDir, "doc1.txt"), f1Content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "sub", "doc2.txt"), f2Content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "sub", "nested", "doc3.txt"), f3Content, 0644); err != nil {
		t.Fatal(err)
	}

	port, err := getFreePort()
	if err != nil {
		t.Fatalf("Failed to get free port: %v", err)
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

	client, err := NewClient(ClientConfig{
		Endpoint: fmt.Sprintf("127.0.0.1:%d", port),
		Timeout:  3 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	syncEngine := NewSyncEngine(client)

	// First sync pass: all 3 files should download
	stats, err := syncEngine.SyncDirectory("", clientDir, SyncOptions{Resume: true}, nil)
	if err != nil {
		t.Fatalf("SyncDirectory failed: %v", err)
	}

	if stats.FilesDownloaded != 3 {
		t.Fatalf("expected 3 downloaded files on first pass, got %d", stats.FilesDownloaded)
	}
	if stats.FilesSkipped != 0 {
		t.Fatalf("expected 0 skipped files on first pass, got %d", stats.FilesSkipped)
	}

	// Verify client received all files correctly
	c1, err := os.ReadFile(filepath.Join(clientDir, "doc1.txt"))
	if err != nil || !bytes.Equal(c1, f1Content) {
		t.Fatal("doc1.txt content mismatch on client")
	}
	c2, err := os.ReadFile(filepath.Join(clientDir, "sub", "doc2.txt"))
	if err != nil || !bytes.Equal(c2, f2Content) {
		t.Fatal("doc2.txt content mismatch on client")
	}
	c3, err := os.ReadFile(filepath.Join(clientDir, "sub", "nested", "doc3.txt"))
	if err != nil || !bytes.Equal(c3, f3Content) {
		t.Fatal("doc3.txt content mismatch on client")
	}

	// Second sync pass: no changes on server, all 3 should be skipped
	stats2, err := syncEngine.SyncDirectory("", clientDir, SyncOptions{Resume: true}, nil)
	if err != nil {
		t.Fatalf("Second SyncDirectory failed: %v", err)
	}
	if stats2.FilesDownloaded != 0 {
		t.Fatalf("expected 0 downloaded files on second pass, got %d", stats2.FilesDownloaded)
	}
	if stats2.FilesSkipped != 3 {
		t.Fatalf("expected 3 skipped files on second pass, got %d", stats2.FilesSkipped)
	}

	// Modify one file and add a new file on server
	newContent := []byte("Document 1 Content MODIFIED.")
	if err := os.WriteFile(filepath.Join(serverDir, "doc1.txt"), newContent, 0644); err != nil {
		t.Fatal(err)
	}
	f4Content := []byte("Document 4 Content - newly added.")
	if err := os.WriteFile(filepath.Join(serverDir, "doc4.txt"), f4Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Third sync pass: exactly 2 files downloaded (doc1 and doc4), 2 files skipped (doc2 and doc3)
	stats3, err := syncEngine.SyncDirectory("", clientDir, SyncOptions{Resume: true}, nil)
	if err != nil {
		t.Fatalf("Third SyncDirectory failed: %v", err)
	}
	if stats3.FilesDownloaded != 2 {
		t.Fatalf("expected 2 downloaded files on third pass, got %d", stats3.FilesDownloaded)
	}
	if stats3.FilesSkipped != 2 {
		t.Fatalf("expected 2 skipped files on third pass, got %d", stats3.FilesSkipped)
	}

	// Test Resume: write partial file
	largeContent := []byte("012345678901234567890123456789012345678901234567890123456789")
	largeFilePath := filepath.Join(serverDir, "large.bin")
	if err := os.WriteFile(largeFilePath, largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	partialClientPath := filepath.Join(clientDir, "large_resumed.bin")
	// write first 20 bytes locally
	if err := os.WriteFile(partialClientPath, largeContent[:20], 0644); err != nil {
		t.Fatal(err)
	}

	checksum, err := client.PullFileResume("large.bin", partialClientPath, nil)
	if err != nil {
		t.Fatalf("PullFileResume failed: %v", err)
	}
	if len(checksum) != 64 {
		t.Fatalf("expected 64 char sha256 checksum, got '%s'", checksum)
	}

	resumedData, err := os.ReadFile(partialClientPath)
	if err != nil || !bytes.Equal(resumedData, largeContent) {
		t.Fatal("resumed file data does not match original full content")
	}
}
