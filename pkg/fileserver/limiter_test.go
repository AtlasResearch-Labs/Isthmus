package fileserver

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestParseRateLimit(t *testing.T) {
	cases := []struct {
		input    string
		expected int64
	}{
		{"500k", 500 * 1024},
		{"2M", 2 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"1048576", 1048576},
		{"", 0},
		{"0", 0},
	}

	for _, c := range cases {
		got, err := ParseRateLimit(c.input)
		if err != nil {
			t.Fatalf("ParseRateLimit(%s) unexpected error: %v", c.input, err)
		}
		if got != c.expected {
			t.Fatalf("ParseRateLimit(%s) = %d, expected %d", c.input, got, c.expected)
		}
	}
}

func TestRateLimitedReader(t *testing.T) {
	data := make([]byte, 100*1024) // 100 KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Limit to 50 KB/sec -> 100 KB should take ~1.5 - 2.5 seconds
	rateLimit := int64(50 * 1024)
	limiter := NewRateLimiter(rateLimit)

	src := bytes.NewReader(data)
	limitedReader := NewRateLimitedReader(src, limiter)

	start := time.Now()
	var dst bytes.Buffer
	buf := make([]byte, 10*1024)

	for {
		n, err := limitedReader.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read error: %v", err)
		}
	}

	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Fatalf("RateLimiter failed to throttle: 100KB took %v at 50KB/s", elapsed)
	}

	if !bytes.Equal(dst.Bytes(), data) {
		t.Fatal("throttled data does not match source")
	}
}
