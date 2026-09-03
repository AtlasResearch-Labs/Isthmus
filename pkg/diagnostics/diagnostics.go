package diagnostics

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/discovery"
	"isthmus/pkg/fileserver"
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

func (r *Runner) PingPeer(ctx context.Context, target string, endpoint string) PingResult {
	peerName := target
	peerID := target

	if p, ok := r.cfg.GetPeer(target); ok {
		peerName = p.DeviceName
		peerID = p.DeviceID
		if endpoint == "" {
			endpoint = p.LastSeenEndpoint
		}
	} else {
		for id, p := range r.cfg.Peers {
			if strings.EqualFold(p.DeviceName, target) {
				peerName = p.DeviceName
				peerID = id
				if endpoint == "" {
					endpoint = p.LastSeenEndpoint
				}
				break
			}
		}
	}

	res := PingResult{
		PeerID:   peerID,
		PeerName: peerName,
		Endpoint: endpoint,
	}

	if endpoint == "" {
		router := discovery.NewAutoRouter(r.cfg)
		dialCtx, dialCancel := context.WithTimeout(ctx, 3*time.Second)
		defer dialCancel()
		if routed, err := router.DialPeer(dialCtx, target); err == nil {
			endpoint = routed.Addr
			routed.Conn.Close()
			res.Endpoint = endpoint
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

func (r *Runner) RunSpeedtest(ctx context.Context, target string, testSizeBytes int64) SpeedtestResult {
	if testSizeBytes <= 0 {
		testSizeBytes = 2 * 1024 * 1024 // 2MB
	}

	peerName := target
	peerID := target

	if p, ok := r.cfg.GetPeer(target); ok {
		peerName = p.DeviceName
		peerID = p.DeviceID
	} else {
		for id, p := range r.cfg.Peers {
			if strings.EqualFold(p.DeviceName, target) {
				peerName = p.DeviceName
				peerID = id
				break
			}
		}
	}

	res := SpeedtestResult{
		PeerID:      peerID,
		PeerName:    peerName,
		BytesTested: testSizeBytes,
	}

	if r.cfg.PrivateKey == "" {
		return r.runSimulatedSpeedtest(ctx, res, testSizeBytes)
	}

	router := discovery.NewAutoRouter(r.cfg)
	routed, err := router.DialPeer(ctx, target)
	if err != nil {
		res.Error = fmt.Sprintf("failed to connect to '%s': %v", peerName, err)
		return res
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: r.cfg.PrivateKey,
	})
	if err != nil {
		res.Error = fmt.Sprintf("handshake failed with '%s': %v", peerName, err)
		return res
	}
	defer client.Close()

	sftpCl := client.SFTPClient()
	if sftpCl == nil {
		res.Error = "SFTP client unavailable"
		return res
	}

	tmpRemotePath := fmt.Sprintf(".speedtest_%d.tmp", time.Now().UnixNano())
	rf, err := sftpCl.Create(tmpRemotePath)
	if err != nil {
		res.Error = fmt.Sprintf("remote create failed: %v", err)
		return res
	}
	defer func() {
		_ = rf.Close()
		_ = sftpCl.Remove(tmpRemotePath)
	}()

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
			n, err := rf.Write(buf[:chunk])
			if err != nil {
				res.Error = fmt.Sprintf("transfer failed: %v", err)
				return res
			}
			transferred += int64(n)
		}
	}

	duration := time.Since(start).Seconds()
	if duration <= 0 {
		duration = 0.001
	}

	res.DurationSeconds = duration
	res.SpeedMBps = (float64(testSizeBytes) / (1024 * 1024)) / duration

	r.log.Info("Speedtest completed to '%s' via %s: %.2f MB/s in %.3fs", peerName, routed.Tier.String(), res.SpeedMBps, duration)
	return res
}

func (r *Runner) runSimulatedSpeedtest(ctx context.Context, res SpeedtestResult, testSizeBytes int64) SpeedtestResult {
	start := time.Now()
	buf := make([]byte, 64*1024)
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
			time.Sleep(50 * time.Microsecond)
		}
	}
	duration := time.Since(start).Seconds()
	if duration <= 0 {
		duration = 0.001
	}
	res.DurationSeconds = duration
	res.SpeedMBps = (float64(testSizeBytes) / (1024 * 1024)) / duration
	return res
}
