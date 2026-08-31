package webdav

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebDAVServer(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_webdav_test")
	if err != nil {
		t.Fatalf("temp dir err: %v", err)
	}
	defer os.RemoveAll(tempDir)

	server := NewServer(tempDir, "/webdav")

	// 1. Test OPTIONS
	req := httptest.NewRequest("OPTIONS", "/webdav/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for OPTIONS, got %d", w.Code)
	}

	// 2. Test PUT (Create file)
	payload := []byte("Hello WebDAV Virtual Disk!")
	req = httptest.NewRequest("PUT", "/webdav/mounted_doc.txt", bytes.NewReader(payload))
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for PUT, got %d", w.Code)
	}

	// 3. Test GET
	req = httptest.NewRequest("GET", "/webdav/mounted_doc.txt", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET, got %d", w.Code)
	}
	if w.Body.String() != string(payload) {
		t.Fatalf("expected '%s', got '%s'", string(payload), w.Body.String())
	}

	// 4. Test PROPFIND
	req = httptest.NewRequest("PROPFIND", "/webdav/", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 207 { // 207 Multi-Status
		t.Fatalf("expected 207 Multi-Status for PROPFIND, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "mounted_doc.txt") {
		t.Fatalf("expected PROPFIND XML to list 'mounted_doc.txt'")
	}

	// 5. Test Mount Command
	cmd := MountCommand(7788, "Z:")
	if cmd == "" {
		t.Fatalf("expected non-empty mount command")
	}

	// 6. Test DELETE
	req = httptest.NewRequest("DELETE", "/webdav/mounted_doc.txt", nil)
	w = httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for DELETE, got %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "mounted_doc.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted")
	}
}
