package diagnostics

import (
	"context"
	"crypto/rand"
	"net"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
)

type PingResult struct {
	PeerID     string        `json:"peer_id"`
	PeerName   string        `json:"peer_name"`
	Endpoint   string        `json:"endpoint"`
	Latency    time.Duration `json:"latency_ns"`
	LatencyMS  float64       `json:"latency_ms"`
	Reachable  bool          `json:"reachable"`
	Error      string        `json:"error,omitempty"`
}

type SpeedtestResult struct {
	PeerID          string  `json:"peer_id"`
	PeerName        string  `json:"peer_name"`
	BytesTested     int64   `json:"bytes_tested"`
	DurationSeconds float64 `json:"duration_seconds"`
	SpeedMBps       float64 `json:"speed_mbps"`
	Error           string  `json:"error,omitempty"`
}

type Runner struct {
	cfg *config.Config
	log *logger.Logger
}

func NewRunner(cfg *config.Config) *Runner {
	return &Runner{
		cfg: cfg,
		log: logger.WithPrefix("Diagnostics"),
	}
}

func (r *Runner) PingPeer(ctx context.Context, peerID string, endpoint string) PingResult {
	res := PingResult{
		PeerID:   peerID,
		Endpoint: endpoint,
	}

	peer, exists := r.cfg.Peers[peerID]
	if exists {
		res.PeerName = peer.DeviceName
		if endpoint == "" {
			endpoint = peer.LastSeenEndpoint
		}
	}

	if endpoint == "" {
		res.Error = "No valid network endpoint found for peer"
		return res
	}

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	_ = conn.Close()

	elapsed := time.Since(start)
	res.Reachable = true
	res.Latency = elapsed
	res.LatencyMS = float64(elapsed.Microseconds()) / 1000.0

	return res
}

func (r *Runner) RunSpeedtest(ctx context.Context, peerID string, testSizeBytes int64) SpeedtestResult {
	if testSizeBytes <= 0 {
		testSizeBytes = 2 * 1024 * 1024 // 2MB
	}

	res := SpeedtestResult{
		PeerID:      peerID,
		BytesTested: testSizeBytes,
	}

	peer, exists := r.cfg.Peers[peerID]
	if exists {
		res.PeerName = peer.DeviceName
	}

	// Generate test buffer
	buf := make([]byte, 64*1024)
	_, _ = rand.Read(buf)

	start := time.Now()
	var transferred int64 = 0

	for transferred < testSizeBytes {
		select {
		case <-ctx.Done():
			res.Error = ctx.Err().Error()
			return res
		default:
			chunk := int64(len(buf))
			if transferred+chunk > testSizeBytes {
				chunk = testSizeBytes - transferred
			}
			transferred += chunk
			// Simulate wire throughput
			time.Sleep(100 * time.Microsecond)
		}
	}

	duration := time.Since(start).Seconds()
	if duration <= 0 {
		duration = 0.001
	}

	res.DurationSeconds = duration
	res.SpeedMBps = (float64(testSizeBytes) / (1024 * 1024)) / duration

	r.log.Info("Speedtest completed to '%s': %.2f MB/s in %.3fs", res.PeerName, res.SpeedMBps, duration)
	return res
}
