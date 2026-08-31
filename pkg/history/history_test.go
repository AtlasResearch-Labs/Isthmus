package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTimeMachine(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_tm_test")
	if err != nil {
		t.Fatalf("temp dir err: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tm := NewTimeMachine(tempDir)
	testFile := filepath.Join(tempDir, "sample.txt")

	// Version 1
	_ = os.WriteFile(testFile, []byte("Hello Version 1"), 0644)
	snap1, err := tm.RecordSnapshot(testFile)
	if err != nil {
		t.Fatalf("record snap1 err: %v", err)
	}

	// Version 2
	_ = os.WriteFile(testFile, []byte("Hello Version 2 Modified"), 0644)
	snap2, err := tm.RecordSnapshot(testFile)
	if err != nil {
		t.Fatalf("record snap2 err: %v", err)
	}

	snaps, err := tm.ListSnapshots(testFile)
	if err != nil || len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	// Restore Version 1
	if err := tm.RestoreSnapshot(testFile, snap1.ID); err != nil {
		t.Fatalf("restore snap1 err: %v", err)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "Hello Version 1" {
		t.Fatalf("expected 'Hello Version 1', got '%s'", string(data))
	}

	// Restore Version 2
	if err := tm.RestoreSnapshot(testFile, snap2.ID); err != nil {
		t.Fatalf("restore snap2 err: %v", err)
	}

	data2, _ := os.ReadFile(testFile)
	if string(data2) != "Hello Version 2 Modified" {
		t.Fatalf("expected 'Hello Version 2 Modified', got '%s'", string(data2))
	}
}
