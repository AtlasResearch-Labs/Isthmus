package runner

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"isthmus/internal/logger"
	"isthmus/pkg/config"
	"isthmus/pkg/discovery"
	"isthmus/pkg/fileserver"
)

type JobTargetResult struct {
	TargetID   string        `json:"target_id"`
	TargetName string        `json:"target_name"`
	Stdout     string        `json:"stdout"`
	Stderr     string        `json:"stderr"`
	ExitCode   int           `json:"exit_code"`
	Duration   time.Duration `json:"duration_ns"`
	DurationMs float64       `json:"duration_ms"`
	Error      string        `json:"error,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

type JobBatchResult struct {
	JobID       string            `json:"job_id"`
	Command     string            `json:"command"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at"`
	Results     []JobTargetResult `json:"results"`
}

type Dispatcher struct {
	cfg *config.Config
	log *logger.Logger
	mu  sync.RWMutex
}

func NewDispatcher(cfg *config.Config) *Dispatcher {
	return &Dispatcher{
		cfg: cfg,
		log: logger.WithPrefix("JobRunner"),
	}
}

// ExecuteLocal runs a command locally with timeout protection
func (d *Dispatcher) ExecuteLocal(ctx context.Context, command string) JobTargetResult {
	start := time.Now()
	res := JobTargetResult{
		TargetID:   "local",
		TargetName: d.cfg.DeviceName,
		Timestamp:  start,
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	out, err := cmd.CombinedOutput()
	duration := time.Since(start)
	res.Duration = duration
	res.DurationMs = float64(duration.Microseconds()) / 1000.0
	res.Stdout = string(out)

	if err != nil {
		res.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	} else {
		res.ExitCode = 0
	}

	return res
}

// DispatchJob dispatches a command across specified targets ("local", peer IDs, or "all")
func (d *Dispatcher) DispatchJob(ctx context.Context, command string, targets []string) JobBatchResult {
	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
	batch := JobBatchResult{
		JobID:     jobID,
		Command:   command,
		StartedAt: time.Now(),
		Results:   make([]JobTargetResult, 0),
	}

	d.log.Info("Dispatching job %s: '%s' to %d targets", jobID, command, len(targets))

	runAll := false
	for _, t := range targets {
		if t == "all" || t == "*" {
			runAll = true
			break
		}
	}

	var targetList []string
	if runAll {
		targetList = append(targetList, "local")
		for peerID := range d.cfg.Peers {
			targetList = append(targetList, peerID)
		}
	} else {
		targetList = targets
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, target := range targetList {
		wg.Add(1)
		go func(tgt string) {
			defer wg.Done()
			var r JobTargetResult
			if tgt == "local" || tgt == "" || tgt == d.cfg.DeviceID {
				r = d.ExecuteLocal(ctx, command)
			} else {
				r = d.ExecuteRemote(ctx, tgt, command)
			}
			mu.Lock()
			batch.Results = append(batch.Results, r)
			mu.Unlock()
		}(target)
	}

	wg.Wait()
	batch.CompletedAt = time.Now()
	return batch
}

// ExecuteRemote runs a command on a remote mesh peer over the secure SSH session channel.
func (d *Dispatcher) ExecuteRemote(ctx context.Context, target string, command string) JobTargetResult {
	start := time.Now()
	peerName := target
	peerID := target

	if p, ok := d.cfg.GetPeer(target); ok {
		peerName = p.DeviceName
		peerID = p.DeviceID
		if !p.Allowed {
			return JobTargetResult{
				TargetID:   peerID,
				TargetName: peerName,
				Timestamp:  start,
				Error:      fmt.Sprintf("peer '%s' is not allowed in ACL", peerName),
				ExitCode:   -1,
			}
		}
	} else {
		for id, p := range d.cfg.Peers {
			if strings.EqualFold(p.DeviceName, target) {
				peerName = p.DeviceName
				peerID = id
				if !p.Allowed {
					return JobTargetResult{
						TargetID:   peerID,
						TargetName: peerName,
						Timestamp:  start,
						Error:      fmt.Sprintf("peer '%s' is not allowed in ACL", peerName),
						ExitCode:   -1,
					}
				}
				break
			}
		}
	}

	res := JobTargetResult{
		TargetID:   peerID,
		TargetName: peerName,
		Timestamp:  start,
	}

	router := discovery.NewAutoRouter(d.cfg)
	routed, err := router.DialPeer(ctx, target)
	if err != nil {
		duration := time.Since(start)
		res.Duration = duration
		res.DurationMs = float64(duration.Microseconds()) / 1000.0
		res.Error = fmt.Sprintf("failed to connect to peer '%s': %v", peerName, err)
		res.ExitCode = -1
		return res
	}
	defer routed.Conn.Close()

	client, err := fileserver.NewClientFromConn(routed.Conn, fileserver.ClientConfig{
		Endpoint:   routed.Addr,
		PrivateKey: d.cfg.PrivateKey,
	})
	if err != nil {
		duration := time.Since(start)
		res.Duration = duration
		res.DurationMs = float64(duration.Microseconds()) / 1000.0
		res.Error = fmt.Sprintf("SSH session handshake failed: %v", err)
		res.ExitCode = -1
		return res
	}
	defer client.Close()

	out, err := client.Exec(command)
	duration := time.Since(start)
	res.Duration = duration
	res.DurationMs = float64(duration.Microseconds()) / 1000.0
	res.Stdout = out

	if err != nil {
		res.Error = err.Error()
		res.ExitCode = 1
	} else {
		res.ExitCode = 0
	}

	return res
}

// QuickCommandTemplates provides standard administrative templates
func QuickCommandTemplates() map[string]string {
	if runtime.GOOS == "windows" {
		return map[string]string{
			"System Uptime":     "Get-CimInstance Win32_OperatingSystem | Select-Object LastBootUpTime",
			"Disk Space":        "Get-PSDrive -PSProvider FileSystem | Select-Object Name, Used, Free",
			"Active Processes":  "Get-Process | Sort-Object CPU -Descending | Select-Object -First 5",
			"Network Status":    "Get-NetIPAddress -AddressFamily IPv4 | Select-Object IPAddress, InterfaceAlias",
		}
	}
	return map[string]string{
		"System Uptime":     "uptime",
		"Disk Space":        "df -h /",
		"Active Processes":  "ps aux --sort=-%cpu | head -n 6",
		"Network Status":    "ip addr || ifconfig",
	}
}
