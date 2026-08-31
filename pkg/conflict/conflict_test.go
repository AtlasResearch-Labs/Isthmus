package conflict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConflictResolver(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "isthmus_conflict_test")
	if err != nil {
		t.Fatalf("temp dir err: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cr := NewResolver(tempDir)

	localFile := filepath.Join(tempDir, "doc.txt")
	localContent := "Line 1: Local header\nLine 2: Common body\nLine 3: Local footer"
	remoteContent := "Line 1: Remote header\nLine 2: Common body\nLine 3: Remote footer\nLine 4: Extra remote"

	if err := os.WriteFile(localFile, []byte(localContent), 0644); err != nil {
		t.Fatalf("write local file err: %v", err)
	}

	// 1. Test Diff Computation
	diff := ComputeDiff(localContent, remoteContent)
	if len(diff) == 0 {
		t.Fatalf("expected non-empty diff")
	}

	hasAdd, hasDel, hasUnchanged := false, false, false
	for _, l := range diff {
		if l.Type == DiffAdded {
			hasAdd = true
		}
		if l.Type == DiffDeleted {
			hasDel = true
		}
		if l.Type == DiffUnchanged {
			hasUnchanged = true
		}
	}
	if !hasAdd || !hasDel || !hasUnchanged {
		t.Fatalf("expected added, deleted, and unchanged diff lines, got: %+v", diff)
	}

	// 2. Test Conflict File Backup Generation
	conflictedPath, err := cr.GenerateConflictFile("doc.txt", []byte(remoteContent), "jack-vm")
	if err != nil {
		t.Fatalf("generate conflict file err: %v", err)
	}
	if !strings.Contains(conflictedPath, ".conflicted_jack-vm_") {
		t.Fatalf("expected filename to contain conflict tag: %s", conflictedPath)
	}

	// 3. Test Merge Both
	if err := cr.Resolve("doc.txt", "merge_both", []byte(remoteContent)); err != nil {
		t.Fatalf("resolve merge_both err: %v", err)
	}

	mergedData, _ := os.ReadFile(localFile)
	if !strings.Contains(string(mergedData), "<<<<<<< LOCAL") || !strings.Contains(string(mergedData), ">>>>>>> REMOTE") {
		t.Fatalf("expected conflict markers in merged file: %s", string(mergedData))
	}
}
