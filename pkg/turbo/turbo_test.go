package turbo

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestTurboEngine(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_turbo_test")
	if err != nil {
		t.Fatalf("temp dir err: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create 5MB test file
	data := make([]byte, 5*1024*1024)
	_, _ = rand.Read(data)

	srcFile := filepath.Join(tempDir, "source_large.bin")
	dstFile := filepath.Join(tempDir, "dest_large.bin")

	if err := os.WriteFile(srcFile, data, 0644); err != nil {
		t.Fatalf("write src file err: %v", err)
	}

	engine := NewEngine(1024*1024, 4) // 1MB chunks, 4 workers

	// 1. Test Manifest Generation
	manifest, err := engine.GenerateManifest(srcFile)
	if err != nil {
		t.Fatalf("generate manifest err: %v", err)
	}
	if manifest.TotalChunks != 5 {
		t.Fatalf("expected 5 chunks for 5MB file, got %d", manifest.TotalChunks)
	}
	if manifest.FileHash == "" {
		t.Fatalf("expected non-empty file hash")
	}

	// 2. Test Parallel Turbo Copy
	var lastPercent float64
	progress, err := engine.TurboCopyParallel(srcFile, dstFile, func(p TransferProgress) {
		lastPercent = p.Percent
	})
	if err != nil {
		t.Fatalf("turbo copy err: %v", err)
	}

	if lastPercent <= 0 {
		t.Fatalf("expected positive progress percent, got %f", lastPercent)
	}

	if progress.TransferredBytes != int64(len(data)) {
		t.Fatalf("expected transferred bytes %d, got %d", len(data), progress.TransferredBytes)
	}

	dstData, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("read dst file err: %v", err)
	}

	if !bytes.Equal(data, dstData) {
		t.Fatalf("destination file content does not match source file!")
	}
}
