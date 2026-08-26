package fileserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"isthmus/internal/logger"
)

type SyncOptions struct {
	Force  bool
	Resume bool
}

type SyncStats struct {
	FilesChecked     int
	FilesDownloaded  int
	FilesSkipped     int
	BytesTransferred int64
	TotalBytes       int64
	Duration         time.Duration
}

type SyncProgressCallback func(relPath string, fileBytesTransferred, fileTotalBytes int64, overallFilesCompleted, totalFiles int)

type SyncEngine struct {
	client *Client
	log    *logger.Logger
}

func NewSyncEngine(client *Client) *SyncEngine {
	return &SyncEngine{
		client: client,
		log:    logger.WithPrefix("SyncEngine"),
	}
}

type fileSyncItem struct {
	remotePath string
	relPath    string
	size       int64
	modTime    time.Time
	isDir      bool
}

func (s *SyncEngine) SyncDirectory(remoteDir, localDir string, opts SyncOptions, progressCb SyncProgressCallback) (*SyncStats, error) {
	startTime := time.Now()
	stats := &SyncStats{}

	if remoteDir == "" {
		remoteDir = "."
	}

	s.log.Info("Scanning remote directory tree '%s'...", remoteDir)

	var items []fileSyncItem
	var totalBytes int64

	err := s.client.Walk(remoteDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}

		rel, err := filepath.Rel(remoteDir, path)
		if err != nil {
			rel = strings.TrimPrefix(path, remoteDir)
		}
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		rel = strings.TrimPrefix(rel, "/")

		if rel == "" || rel == "." {
			return nil
		}

		item := fileSyncItem{
			remotePath: path,
			relPath:    rel,
			size:       info.Size(),
			modTime:    info.ModTime(),
			isDir:      info.IsDir(),
		}

		items = append(items, item)
		if !item.isDir {
			totalBytes += item.size
		}
		return nil
	})

	if err != nil {
		return stats, fmt.Errorf("remote walk failed: %w", err)
	}

	stats.TotalBytes = totalBytes
	totalFiles := 0
	for _, it := range items {
		if !it.isDir {
			totalFiles++
		}
	}

	s.log.Info("Found %d remote files/folders (%d bytes) to evaluate.", len(items), totalBytes)

	filesDone := 0

	for _, item := range items {
		localTarget := filepath.Join(localDir, filepath.FromSlash(item.relPath))

		if item.isDir {
			if err := os.MkdirAll(localTarget, 0755); err != nil {
				return stats, fmt.Errorf("failed to create local dir %s: %w", localTarget, err)
			}
			continue
		}

		stats.FilesChecked++

		// Check if local file exists and matches size and modtime
		if !opts.Force {
			if localInfo, err := os.Stat(localTarget); err == nil {
				if localInfo.Size() == item.size && !localInfo.IsDir() {
					// File matches size, skip
					stats.FilesSkipped++
					filesDone++
					if progressCb != nil {
						progressCb(item.relPath, item.size, item.size, filesDone, totalFiles)
					}
					continue
				}
			}
		}

		// Pull file
		s.log.Debug("Syncing: %s -> %s", item.remotePath, localTarget)

		var pullErr error
		if opts.Resume {
			_, pullErr = s.client.PullFileResume(item.remotePath, localTarget, func(transferred, total int64, speed float64) {
				if progressCb != nil {
					progressCb(item.relPath, transferred, total, filesDone, totalFiles)
				}
			})
		} else {
			_, pullErr = s.client.PullFile(item.remotePath, localTarget, func(transferred, total int64, speed float64) {
				if progressCb != nil {
					progressCb(item.relPath, transferred, total, filesDone, totalFiles)
				}
			})
		}

		if pullErr != nil {
			return stats, fmt.Errorf("failed syncing file %s: %w", item.relPath, pullErr)
		}

		stats.FilesDownloaded++
		stats.BytesTransferred += item.size
		filesDone++

		if progressCb != nil {
			progressCb(item.relPath, item.size, item.size, filesDone, totalFiles)
		}
	}

	stats.Duration = time.Since(startTime)
	s.log.Info("Sync completed in %v. Downloaded: %d, Skipped: %d, Transferred: %d bytes",
		stats.Duration, stats.FilesDownloaded, stats.FilesSkipped, stats.BytesTransferred)

	return stats, nil
}
