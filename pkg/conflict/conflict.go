package conflict

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type DiffLineType string

const (
	DiffUnchanged DiffLineType = "unchanged"
	DiffAdded     DiffLineType = "added"
	DiffDeleted   DiffLineType = "deleted"
)

type DiffLine struct {
	Type    DiffLineType `json:"type"`
	Content string       `json:"content"`
	OldNum  int          `json:"old_num,omitempty"`
	NewNum  int          `json:"new_num,omitempty"`
}

type ConflictReport struct {
	FilePath     string     `json:"file_path"`
	HasConflict  bool       `json:"has_conflict"`
	LocalModTime time.Time  `json:"local_mod_time"`
	RemoteNode   string     `json:"remote_node"`
	Lines        []DiffLine `json:"diff_lines"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
}

type Resolver struct {
	baseDir string
	log     *logger.Logger
	mu      sync.Mutex
}

func NewResolver(baseDir string) *Resolver {
	return &Resolver{
		baseDir: baseDir,
		log:     logger.WithPrefix("ConflictResolver"),
	}
}

// ComputeDiff generates a line-by-line diff between two text versions
func ComputeDiff(localText, remoteText string) []DiffLine {
	localLines := strings.Split(localText, "\n")
	remoteLines := strings.Split(remoteText, "\n")

	diff := make([]DiffLine, 0)
	i, j := 0, 0
	oldNum, newNum := 1, 1

	for i < len(localLines) && j < len(remoteLines) {
		if localLines[i] == remoteLines[j] {
			diff = append(diff, DiffLine{
				Type:    DiffUnchanged,
				Content: localLines[i],
				OldNum:  oldNum,
				NewNum:  newNum,
			})
			i++
			j++
			oldNum++
			newNum++
		} else {
			// Marked as local deletion, remote addition
			diff = append(diff, DiffLine{
				Type:    DiffDeleted,
				Content: localLines[i],
				OldNum:  oldNum,
			})
			diff = append(diff, DiffLine{
				Type:    DiffAdded,
				Content: remoteLines[j],
				NewNum:  newNum,
			})
			i++
			j++
			oldNum++
			newNum++
		}
	}

	for ; i < len(localLines); i++ {
		diff = append(diff, DiffLine{
			Type:    DiffDeleted,
			Content: localLines[i],
			OldNum:  oldNum,
		})
		oldNum++
	}

	for ; j < len(remoteLines); j++ {
		diff = append(diff, DiffLine{
			Type:    DiffAdded,
			Content: remoteLines[j],
			NewNum:  newNum,
		})
		newNum++
	}

	return diff
}

// GenerateConflictFile copies remote data to a .conflicted.<timestamp> file
func (r *Resolver) GenerateConflictFile(relPath string, remoteContent []byte, remoteNode string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullPath := filepath.Join(r.baseDir, relPath)
	ext := filepath.Ext(fullPath)
	base := strings.TrimSuffix(fullPath, ext)
	timestamp := time.Now().Format("20060102_150405")
	conflictedName := fmt.Sprintf("%s.conflicted_%s_%s%s", base, remoteNode, timestamp, ext)

	if err := os.WriteFile(conflictedName, remoteContent, 0644); err != nil {
		return "", err
	}

	r.log.Info("Conflict detected for '%s' — created backup snapshot '%s'", relPath, filepath.Base(conflictedName))
	return conflictedName, nil
}

// Resolve applies a resolution strategy ("keep_local", "keep_remote", "merge_both")
func (r *Resolver) Resolve(relPath, strategy string, remoteContent []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullPath := filepath.Join(r.baseDir, relPath)
	switch strategy {
	case "keep_local":
		// Keep existing local file as-is
		r.log.Info("Conflict on '%s' resolved: kept local version", relPath)
		return nil

	case "keep_remote":
		if err := os.WriteFile(fullPath, remoteContent, 0644); err != nil {
			return err
		}
		r.log.Info("Conflict on '%s' resolved: replaced with remote version", relPath)
		return nil

	case "merge_both":
		localData, _ := os.ReadFile(fullPath)
		merged := fmt.Sprintf("<<<<<<< LOCAL (%s)\n%s\n=======\n%s\n>>>>>>> REMOTE\n", time.Now().Format(time.RFC3339), string(localData), string(remoteContent))
		if err := os.WriteFile(fullPath, []byte(merged), 0644); err != nil {
			return err
		}
		r.log.Info("Conflict on '%s' resolved: merged both with diff markers", relPath)
		return nil

	default:
		return fmt.Errorf("unknown conflict resolution strategy '%s'", strategy)
	}
}
