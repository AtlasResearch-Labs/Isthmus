package mobile

import (
	"encoding/json"
	"testing"
)

func TestMobileStatusResponseSerialization(t *testing.T) {
	respJSON := formatStatus(true, "Agent initialized", "dev-12345", "10.77.0.5")
	var resp MobileStatusResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if resp.DeviceID != "dev-12345" {
		t.Fatalf("expected device ID 'dev-12345', got '%s'", resp.DeviceID)
	}
	if resp.VirtualIP != "10.77.0.5" {
		t.Fatalf("expected virtual IP '10.77.0.5', got '%s'", resp.VirtualIP)
	}
}
