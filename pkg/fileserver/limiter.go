package fileserver

import (
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ParseRateLimit converts strings like "5MB", "500KB", "10M" to bytes/sec
func ParseRateLimit(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" || s == "0" {
		return 0, nil
	}
	multiplier := int64(1)
	if strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K")
	} else if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	} else if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	}

	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}
	return val * multiplier, nil
}

// RateLimiter controls byte transfer rate using token bucket algorithm
type RateLimiter struct {
	bytesPerSec int64
	tokens      int64
	lastCheck   time.Time
	mu          sync.Mutex
}

func NewRateLimiter(bytesPerSec int64) *RateLimiter {
	return &RateLimiter{
		bytesPerSec: bytesPerSec,
		tokens:      bytesPerSec,
		lastCheck:   time.Now(),
	}
}

func (rl *RateLimiter) Wait(ctx context.Context, bytesCount int) error {
	if rl == nil || rl.bytesPerSec <= 0 {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rl.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(rl.lastCheck)
		rl.lastCheck = now

		// Add new tokens based on elapsed time
		rl.tokens += int64(elapsed.Seconds() * float64(rl.bytesPerSec))
		if rl.tokens > rl.bytesPerSec {
			rl.tokens = rl.bytesPerSec
		}

		if rl.tokens >= int64(bytesCount) {
			rl.tokens -= int64(bytesCount)
			rl.mu.Unlock()
			return nil
		}

		// Calculate required sleep duration
		needed := int64(bytesCount) - rl.tokens
		sleepSecs := float64(needed) / float64(rl.bytesPerSec)
		rl.mu.Unlock()

		sleepDuration := time.Duration(sleepSecs * float64(time.Second))
		if sleepDuration > 100*time.Millisecond {
			sleepDuration = 100 * time.Millisecond
		}

		time.Sleep(sleepDuration)
	}
}

type RateLimitedReader struct {
	reader  io.Reader
	limiter *RateLimiter
	ctx     context.Context
}

func NewRateLimitedReader(ctx context.Context, r io.Reader, bytesPerSec int64) io.Reader {
	if bytesPerSec <= 0 {
		return r
	}
	return &RateLimitedReader{
		reader:  r,
		limiter: NewRateLimiter(bytesPerSec),
		ctx:     ctx,
	}
}

func (r *RateLimitedReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		_ = r.limiter.Wait(r.ctx, n)
	}
	return n, err
}

type RateLimitedWriter struct {
	writer  io.Writer
	limiter *RateLimiter
	ctx     context.Context
}

func NewRateLimitedWriter(ctx context.Context, w io.Writer, bytesPerSec int64) io.Writer {
	if bytesPerSec <= 0 {
		return w
	}
	return &RateLimitedWriter{
		writer:  w,
		limiter: NewRateLimiter(bytesPerSec),
		ctx:     ctx,
	}
}

func (w *RateLimitedWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		_ = w.limiter.Wait(w.ctx, len(p))
	}
	return w.writer.Write(p)
}
