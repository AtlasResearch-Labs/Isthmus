package service

import (
	"testing"
)

func TestServiceManagerDefaults(t *testing.T) {
	mgr := NewManager()
	if mgr.ServiceName != "isthmus" {
		t.Fatalf("expected service name 'isthmus', got '%s'", mgr.ServiceName)
	}
	if mgr.DisplayName == "" {
		t.Fatal("expected non-empty display name")
	}
}
