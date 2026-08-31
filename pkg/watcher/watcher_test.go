package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFolderWatcher(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus-watcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fw, err := NewFolderWatcher(tempDir, Options{
		DebounceDelay: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("failed to init folder watcher: %v", err)
	}

	eventReceived := make(chan WatchEvent, 10)
	fw.OnChange(func(we WatchEvent) {
		eventReceived <- we
	})

	if err := fw.Start(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer fw.Stop()

	// 1. Create a file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	select {
	case ev := <-eventReceived:
		if ev.RelPath != "test.txt" {
			t.Errorf("expected rel path test.txt, got %s", ev.RelPath)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file create event")
	}

	// 2. Modify the file
	if err := os.WriteFile(testFile, []byte("hello world updated"), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	select {
	case ev := <-eventReceived:
		if ev.RelPath != "test.txt" {
			t.Errorf("expected rel path test.txt, got %s", ev.RelPath)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for file modify event")
	}
}
