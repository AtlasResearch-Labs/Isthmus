package fileserver

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

func ParseRateLimit(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" || s == "0" {
		return 0, nil
	}

	multiplier := int64(1)
	if strings.HasSuffix(s, "K") || strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K")
	} else if strings.HasSuffix(s, "M") || strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	} else if strings.HasSuffix(s, "G") || strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate limit format: %s", s)
	}

	if val <= 0 {
		return 0, errors.New("rate limit must be greater than zero")
	}

	return int64(val * float64(multiplier)), nil
}

type RateLimiter struct {
	mu           sync.Mutex
	bytesPerSec  int64
	tokens       float64
	lastRefill   time.Time
	maxBurstSize float64
}

func NewRateLimiter(bytesPerSec int64) *RateLimiter {
	if bytesPerSec <= 0 {
		return nil
	}
	burst := float64(bytesPerSec) / 10.0
	if burst < 4096 {
		burst = 4096
	}
	return &RateLimiter{
		bytesPerSec:  bytesPerSec,
		tokens:       burst,
		maxBurstSize: burst,
		lastRefill:   time.Now(),
	}
}

func (l *RateLimiter) Wait(n int) {
	if l == nil || l.bytesPerSec <= 0 {
		return
	}

	l.mu.Lock()

	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.lastRefill = now

	// Refill tokens
	l.tokens += elapsed * float64(l.bytesPerSec)
	if l.tokens > l.maxBurstSize {
		l.tokens = l.maxBurstSize
	}

	needed := float64(n)
	if l.tokens >= needed {
		l.tokens -= needed
		l.mu.Unlock()
		return
	}

	// Calculate wait duration
	deficit := needed - l.tokens
	waitSec := deficit / float64(l.bytesPerSec)
	l.tokens = 0
	l.mu.Unlock()

	time.Sleep(time.Duration(waitSec * float64(time.Second)))
}

type RateLimitedReader struct {
	r       io.Reader
	limiter *RateLimiter
}

func NewRateLimitedReader(r io.Reader, limiter *RateLimiter) io.Reader {
	if limiter == nil {
		return r
	}
	return &RateLimitedReader{r: r, limiter: limiter}
}

func (r *RateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 && r.limiter != nil {
		r.limiter.Wait(n)
	}
	return n, err
}

type RateLimitedWriter struct {
	w       io.Writer
	limiter *RateLimiter
}

func NewRateLimitedWriter(w io.Writer, limiter *RateLimiter) io.Writer {
	if limiter == nil {
		return w
	}
	return &RateLimitedWriter{w: w, limiter: limiter}
}

func (w *RateLimitedWriter) Write(p []byte) (int, error) {
	if w.limiter != nil {
		w.limiter.Wait(len(p))
	}
	return w.w.Write(p)
}
