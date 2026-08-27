package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"isthmus/pkg/config"
)

func TestGUISeverStaticAndAPI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_gui_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, err := config.NewDefaultConfig("test-gui-device")
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}
	cfg.SharedDir = tempDir

	// Create a test file in shared root
	testFile := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(testFile, []byte("Hello GUI"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	guiServer := NewServer(cfg)
	handler := guiServer.Handler()

	// 1. Test static index.html
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for root, got %d", w.Code)
	}

	// 2. Test static CSS
	req = httptest.NewRequest("GET", "/style.css", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for style.css, got %d", w.Code)
	}

	// 3. Test /api/status
	req = httptest.NewRequest("GET", "/api/status", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/status, got %d", w.Code)
	}

	var statusResp config.Config
	if err := json.Unmarshal(w.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed to decode /api/status: %v", err)
	}
	if statusResp.DeviceName != "test-gui-device" {
		t.Fatalf("expected device name 'test-gui-device', got '%s'", statusResp.DeviceName)
	}

	// 4. Test /api/browse?peer=local
	req = httptest.NewRequest("GET", "/api/browse?peer=local&path=.", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/browse, got %d", w.Code)
	}

	var browseResp struct {
		Tier    string          `json:"tier"`
		Entries []FileEntryJSON `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &browseResp); err != nil {
		t.Fatalf("failed to decode browse response: %v", err)
	}

	if len(browseResp.Entries) != 1 || browseResp.Entries[0].Name != "sample.txt" {
		t.Fatalf("expected 1 file entry 'sample.txt', got %v", browseResp.Entries)
	}
}
