package share

import (
	"testing"
	"time"
)

func TestShareManager(t *testing.T) {
	mgr := NewManager()

	st := mgr.CreateLink("local", "docs/manual.pdf", "manual.pdf", 100*time.Millisecond, 2)
	if st.Token == "" {
		t.Fatalf("expected non-empty token")
	}

	// 1st consumption
	consumed, err := mgr.ValidateAndConsume(st.Token)
	if err != nil || consumed.DownloadCount != 1 {
		t.Fatalf("1st consume failed: %v", err)
	}

	// 2nd consumption
	consumed2, err := mgr.ValidateAndConsume(st.Token)
	if err != nil || consumed2.DownloadCount != 2 {
		t.Fatalf("2nd consume failed: %v", err)
	}

	// 3rd consumption should exceed limit
	_, err = mgr.ValidateAndConsume(st.Token)
	if err == nil {
		t.Fatalf("expected download limit error on 3rd consume")
	}
}
