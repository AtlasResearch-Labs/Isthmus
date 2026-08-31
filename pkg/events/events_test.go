package events

import (
	"testing"
	"time"
)

func TestEventBroker(t *testing.T) {
	broker := NewBroker()

	// 1. Test Publish and History
	ev := broker.Publish(EventPeerConnected, "Node Online", "jack-vm connected over LAN", "success", map[string]string{"peer": "jack-vm"})
	if ev.ID == "" {
		t.Fatalf("expected non-empty event ID")
	}

	history := broker.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 event in history, got %d", len(history))
	}
	if history[0].Title != "Node Online" {
		t.Fatalf("expected title 'Node Online', got '%s'", history[0].Title)
	}

	// 2. Test Channel Subscriptions
	ch := make(chan Event, 5)
	broker.mu.Lock()
	broker.subscribers[ch] = struct{}{}
	broker.mu.Unlock()

	broker.Publish(EventBatteryLow, "Battery Warning", "Phone battery at 12%", "warning", nil)

	select {
	case received := <-ch:
		if received.Type != EventBatteryLow {
			t.Fatalf("expected EventBatteryLow, got: %s", received.Type)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for event on channel")
	}

	broker.mu.Lock()
	delete(broker.subscribers, ch)
	broker.mu.Unlock()
}
