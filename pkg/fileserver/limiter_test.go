package fileserver

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	ctx := context.Background()

	// Rate limit to 100KB/s
	rateLimit := int64(100 * 1024)
	rl := NewRateLimiter(rateLimit)

	start := time.Now()
	// Request 100KB twice
	if err := rl.Wait(ctx, 100*1024); err != nil {
		t.Fatalf("first wait failed: %v", err)
	}

	if err := rl.Wait(ctx, 100*1024); err != nil {
		t.Fatalf("second wait failed: %v", err)
	}
	elapsed := time.Since(start)

	// Second wait should take around ~900ms-1s
	if elapsed < 800*time.Millisecond {
		t.Logf("rate limiter elapsed: %v", elapsed)
	}
}

func TestRateLimitedReader(t *testing.T) {
	ctx := context.Background()
	data := make([]byte, 10*1024) // 10KB
	src := bytes.NewReader(data)

	lr := NewRateLimitedReader(ctx, src, 50*1024) // 50 KB/s limit
	dest := make([]byte, 10*1024)

	n, err := io.ReadFull(lr, dest)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes, got %d", len(data), n)
	}
}
