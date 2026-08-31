package clipboard

import (
	"testing"
)

func TestClipboardManager(t *testing.T) {
	mgr := NewManager(5)

	item1 := mgr.Set("https://github.com/istmus", "my-pc")
	if item1.Content != "https://github.com/istmus" {
		t.Fatalf("unexpected content: %s", item1.Content)
	}

	item2 := mgr.Set("secret_token_123", "my-phone")
	if item2.Content != "secret_token_123" {
		t.Fatalf("unexpected content for item2: %s", item2.Content)
	}
	latest := mgr.GetLatest()
	if latest == nil || latest.Content != "secret_token_123" {
		t.Fatalf("latest clipboard mismatch: %+v", latest)
	}

	history := mgr.GetHistory()
	if len(history) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(history))
	}

	mgr.Clear()
	if len(mgr.GetHistory()) != 0 {
		t.Fatalf("expected 0 items after clear")
	}
}
