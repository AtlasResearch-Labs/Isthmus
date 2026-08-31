package history

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type SnapshotInfo struct {
	ID        string    `json:"id"`
	FilePath  string    `json:"file_path"`
	Checksum  string    `json:"checksum"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
}

type TimeMachine struct {
	historyDir string
	log        *logger.Logger
	mu         sync.Mutex
}

func NewTimeMachine(baseDir string) *TimeMachine {
	hDir := filepath.Join(baseDir, ".isthmus", "history")
	_ = os.MkdirAll(hDir, 0755)

	return &TimeMachine{
		historyDir: hDir,
		log:        logger.WithPrefix("TimeMachine"),
	}
}

// RecordSnapshot saves a versioned delta copy of a modified file
func (tm *TimeMachine) RecordSnapshot(filePath string) (*SnapshotInfo, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	h := sha256.New()
	h.Write(data)
	checksum := hex.EncodeToString(h.Sum(nil))

	cleanPath := strings.ReplaceAll(filePath, ":", "")
	cleanPath = strings.ReplaceAll(cleanPath, "\\", "/")
	fileSnapshotDir := filepath.Join(tm.historyDir, cleanPath)
	if err := os.MkdirAll(fileSnapshotDir, 0755); err != nil {
		return nil, err
	}

	timestamp := time.Now()
	snapshotID := fmt.Sprintf("%s_%s", timestamp.Format("20060102150405"), checksum[:12])
	destFile := filepath.Join(fileSnapshotDir, snapshotID)

	if err := os.WriteFile(destFile, data, 0644); err != nil {
		return nil, err
	}

	snap := &SnapshotInfo{
		ID:        snapshotID,
		FilePath:  filePath,
		Checksum:  checksum,
		Size:      int64(len(data)),
		Timestamp: timestamp,
	}

	tm.log.Info("Snapshot recorded for '%s' (ID: %s)", filepath.Base(filePath), snapshotID)
	return snap, nil
}

// ListSnapshots returns all historic revisions for a specific file
func (tm *TimeMachine) ListSnapshots(filePath string) ([]SnapshotInfo, error) {
	cleanPath := strings.ReplaceAll(filePath, ":", "")
	cleanPath = strings.ReplaceAll(cleanPath, "\\", "/")
	fileSnapshotDir := filepath.Join(tm.historyDir, cleanPath)

	entries, err := os.ReadDir(fileSnapshotDir)
	if err != nil {
		return []SnapshotInfo{}, nil
	}

	res := make([]SnapshotInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			continue
		}

		parts := strings.Split(entry.Name(), "_")
		t := fi.ModTime()
		if len(parts) >= 2 {
			if parsed, err := time.Parse("20060102150405", parts[0]); err == nil {
				t = parsed
			}
		}

		res = append(res, SnapshotInfo{
			ID:        entry.Name(),
			FilePath:  filePath,
			Size:      fi.Size(),
			Timestamp: t,
		})
	}

	return res, nil
}

// RestoreSnapshot reverts a file to a historic snapshot
func (tm *TimeMachine) RestoreSnapshot(filePath string, snapshotID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cleanPath := strings.ReplaceAll(filePath, ":", "")
	cleanPath = strings.ReplaceAll(cleanPath, "\\", "/")
	snapshotFile := filepath.Join(tm.historyDir, cleanPath, snapshotID)

	src, err := os.Open(snapshotFile)
	if err != nil {
		return fmt.Errorf("snapshot not found: %w", err)
	}
	defer src.Close()

	dest, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open destination for rollback: %w", err)
	}
	defer dest.Close()

	_, err = io.Copy(dest, src)
	if err == nil {
		tm.log.Info("Successfully rolled back '%s' to snapshot %s", filepath.Base(filePath), snapshotID)
	}
	return err
}
