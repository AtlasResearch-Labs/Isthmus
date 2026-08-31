package fileserver

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestChunkerManifestAndAssembly(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus-chunker-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create 2.5 MB random test file
	srcPath := filepath.Join(tempDir, "source.bin")
	data := make([]byte, int(2.5*1024*1024))
	_, _ = rand.Read(data)
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// 1. Generate Manifest (512KB blocks)
	blockSize := 512 * 1024
	chunker := NewChunker(blockSize)
	manifest, err := chunker.GenerateManifest(srcPath)
	if err != nil {
		t.Fatalf("failed to generate manifest: %v", err)
	}

	if manifest.TotalBlocks != 5 {
		t.Errorf("expected 5 blocks, got %d", manifest.TotalBlocks)
	}

	// 2. Assemble to destination using out-of-order blocks
	destPath := filepath.Join(tempDir, "dest.bin")
	bw, err := NewBlockWriter(destPath, manifest.TotalSize)
	if err != nil {
		t.Fatalf("failed to init block writer: %v", err)
	}

	// Write blocks in reverse order
	for i := len(manifest.Blocks) - 1; i >= 0; i-- {
		blk := manifest.Blocks[i]
		blockData := data[blk.Offset : blk.Offset+int64(blk.Size)]

		// Verify checksum
		if !chunker.VerifyBlock(blockData, blk.Checksum) {
			t.Fatalf("block %d failed checksum verification", blk.Index)
		}

		if err := bw.WriteBlock(blk.Offset, blockData); err != nil {
			t.Fatalf("failed to write block %d: %v", blk.Index, err)
		}
	}

	if err := bw.Close(); err != nil {
		t.Fatalf("failed to close block writer: %v", err)
	}

	// 3. Verify assembled destination file matches source exactly
	destData, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}

	if !bytes.Equal(data, destData) {
		t.Fatal("assembled file content does not match source file")
	}
}
