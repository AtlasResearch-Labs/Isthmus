package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"

	"isthmus/pkg/fileserver"
)

type Browser struct {
	client        *fileserver.Client
	peerName      string
	transportTier string
	currentPath   string
	entries       []os.FileInfo
	selectedIndex int
	scrollOffset  int
	statusMsg     string
	statusTime    time.Time
	width         int
	height        int
}

func NewBrowser(client *fileserver.Client, peerName, tier, initialPath string) *Browser {
	if initialPath == "" {
		initialPath = "."
	}
	return &Browser{
		client:        client,
		peerName:      peerName,
		transportTier: tier,
		currentPath:   initialPath,
		width:         80,
		height:        24,
	}
}

func (b *Browser) loadDirectory() error {
	entries, err := b.client.List(b.currentPath)
	if err != nil {
		return err
	}

	// Sort directories first, then files alphabetically
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	b.entries = entries
	if b.selectedIndex >= len(entries) {
		b.selectedIndex = len(entries) - 1
	}
	if b.selectedIndex < 0 {
		b.selectedIndex = 0
	}
	b.scrollOffset = 0
	return nil
}

func (b *Browser) Run(ctx context.Context) error {
	// Query terminal size
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		b.width = w
		b.height = h
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to enable raw terminal mode: %w", err)
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		fmt.Print("\033[?25h\033[0m\n") // Restore cursor and reset colors
	}()

	fmt.Print("\033[?25l") // Hide cursor

	if err := b.loadDirectory(); err != nil {
		b.setStatus(fmt.Sprintf("Error reading dir: %v", err))
	}

	b.render()

	buf := make([]byte, 16)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}

		// Handle key presses
		if n == 1 {
			switch buf[0] {
			case 'q', 'Q', 3, 27: // 'q', Ctrl+C, Esc
				return nil
			case 'k': // Up
				b.moveSelection(-1)
			case 'j': // Down
				b.moveSelection(1)
			case 13: // Enter
				b.openSelected()
			case 127, 8, 'h': // Backspace / 'h'
				b.navigateUp()
			case 'd', 'D': // Download
				b.downloadSelected(oldState)
			case 's', 'S': // Sync folder
				b.syncCurrentDir(oldState)
			case 'r', 'R': // Refresh
				_ = b.loadDirectory()
				b.setStatus("Refreshed.")
			}
		} else if n >= 3 && buf[0] == 27 && buf[1] == 91 {
			// Escape sequence (Arrow keys)
			switch buf[2] {
			case 65: // Up Arrow
				b.moveSelection(-1)
			case 66: // Down Arrow
				b.moveSelection(1)
			case 67: // Right Arrow
				b.openSelected()
			case 68: // Left Arrow
				b.navigateUp()
			case 53: // Page Up
				b.moveSelection(-10)
			case 54: // Page Down
				b.moveSelection(10)
			case 72: // Home
				b.selectedIndex = 0
			case 70: // End
				if len(b.entries) > 0 {
					b.selectedIndex = len(b.entries) - 1
				}
			}
		}

		b.render()
	}

	return nil
}

func (b *Browser) setStatus(msg string) {
	b.statusMsg = msg
	b.statusTime = time.Now()
}

func (b *Browser) moveSelection(delta int) {
	if len(b.entries) == 0 {
		return
	}
	b.selectedIndex += delta
	if b.selectedIndex < 0 {
		b.selectedIndex = 0
	}
	if b.selectedIndex >= len(b.entries) {
		b.selectedIndex = len(b.entries) - 1
	}

	maxVisible := b.height - 7
	if maxVisible < 5 {
		maxVisible = 5
	}

	if b.selectedIndex < b.scrollOffset {
		b.scrollOffset = b.selectedIndex
	}
	if b.selectedIndex >= b.scrollOffset+maxVisible {
		b.scrollOffset = b.selectedIndex - maxVisible + 1
	}
}

func (b *Browser) openSelected() {
	if len(b.entries) == 0 || b.selectedIndex >= len(b.entries) {
		return
	}
	selected := b.entries[b.selectedIndex]
	if selected.IsDir() {
		if b.currentPath == "." || b.currentPath == "" {
			b.currentPath = selected.Name()
		} else {
			b.currentPath = filepath.ToSlash(filepath.Join(b.currentPath, selected.Name()))
		}
		_ = b.loadDirectory()
		b.selectedIndex = 0
		b.scrollOffset = 0
		b.setStatus(fmt.Sprintf("Entered %s", b.currentPath))
	} else {
		b.setStatus(fmt.Sprintf("File: %s (%d bytes). Press 'd' to download.", selected.Name(), selected.Size()))
	}
}

func (b *Browser) navigateUp() {
	if b.currentPath == "." || b.currentPath == "" || b.currentPath == "/" {
		b.setStatus("Already at root directory.")
		return
	}
	parent := filepath.ToSlash(filepath.Dir(b.currentPath))
	if parent == "/" || parent == "." {
		b.currentPath = "."
	} else {
		b.currentPath = parent
	}
	_ = b.loadDirectory()
	b.selectedIndex = 0
	b.scrollOffset = 0
	b.setStatus(fmt.Sprintf("Moved up to %s", b.currentPath))
}

func (b *Browser) downloadSelected(oldState *term.State) {
	if len(b.entries) == 0 || b.selectedIndex >= len(b.entries) {
		return
	}
	selected := b.entries[b.selectedIndex]
	if selected.IsDir() {
		b.setStatus("Cannot download directory as single file. Press 's' to delta-sync folder.")
		return
	}

	remoteFilePath := filepath.ToSlash(filepath.Join(b.currentPath, selected.Name()))
	localDest := selected.Name()

	// Restore normal terminal mode temporarily for download progress
	_ = term.Restore(int(os.Stdin.Fd()), oldState)
	fmt.Print("\033[2J\033[H\033[?25h") // Clear screen and show cursor

	fmt.Printf("%s\n", RetroTitleBar("ISTHMUS FILE DOWNLOAD", b.width))
	fmt.Printf("%sRemote: %s%s\n", ColorFgWhite, remoteFilePath, ColorReset)
	fmt.Printf("%sLocal:  %s%s\n\n", ColorFgWhite, localDest, ColorReset)

	lastReport := time.Now()
	_, err := b.client.PullFileResume(remoteFilePath, localDest, func(transferred, total int64, speed float64) {
		if time.Since(lastReport) >= 200*time.Millisecond || transferred == total {
			var percent float64
			if total > 0 {
				percent = float64(transferred) / float64(total) * 100.0
			}
			speedMB := speed / (1024 * 1024)
			barWidth := 30
			filled := int(float64(barWidth) * (percent / 100.0))
			if filled > barWidth {
				filled = barWidth
			}
			bar := strings.Repeat("=", filled)
			if filled < barWidth {
				bar += ">" + strings.Repeat(" ", barWidth-filled-1)
			}
			fmt.Printf("\r%s[%s]%s %5.1f%% [%6.2f MB/s] %d/%d bytes",
				ColorFgBlue, bar, ColorReset, percent, speedMB, transferred, total)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("\n%s%s Download failed: %v%s\n", ColorFgWhite, SymErr, err, ColorReset)
	} else {
		fmt.Printf("\n%s%s Download complete: %s%s\n", ColorFgWhite, SymOK, localDest, ColorReset)
	}

	fmt.Printf("\n%sPress any key to return to File Explorer...%s", ColorFgGray, ColorReset)
	_, _ = term.MakeRaw(int(os.Stdin.Fd()))
	var pause [1]byte
	_, _ = os.Stdin.Read(pause[:])

	b.setStatus(fmt.Sprintf("Downloaded %s successfully.", selected.Name()))
}

func (b *Browser) syncCurrentDir(oldState *term.State) {
	_ = term.Restore(int(os.Stdin.Fd()), oldState)
	fmt.Print("\033[2J\033[H\033[?25h")

	fmt.Printf("%s\n", RetroTitleBar("ISTHMUS DIRECTORY DELTA SYNC", b.width))
	fmt.Printf("%sSyncing remote '%s' to local './sync_output'...%s\n\n", ColorFgWhite, b.currentPath, ColorReset)

	syncEngine := fileserver.NewSyncEngine(b.client)
	lastReport := time.Now()
	stats, err := syncEngine.SyncDirectory(b.currentPath, "./sync_output", fileserver.SyncOptions{Resume: true}, func(relPath string, current, total int64, doneFiles, totalFiles int) {
		if time.Since(lastReport) >= 200*time.Millisecond || doneFiles == totalFiles {
			fmt.Printf("\rSyncing [%d/%d files] %-30s", doneFiles, totalFiles, relPath)
			lastReport = time.Now()
		}
	})
	fmt.Println()

	if err != nil {
		fmt.Printf("\n%s%s Sync failed: %v%s\n", ColorFgWhite, SymErr, err, ColorReset)
	} else {
		fmt.Printf("\n%s%s Sync complete. %d downloaded, %d skipped (%v)%s\n",
			ColorFgWhite, SymOK, stats.FilesDownloaded, stats.FilesSkipped, stats.Duration, ColorReset)
	}

	fmt.Printf("\n%sPress any key to return to File Explorer...%s", ColorFgGray, ColorReset)
	_, _ = term.MakeRaw(int(os.Stdin.Fd()))
	var pause [1]byte
	_, _ = os.Stdin.Read(pause[:])

	b.setStatus(fmt.Sprintf("Synced %d files.", stats.FilesDownloaded))
}

func (b *Browser) render() {
	var sb strings.Builder

	// Clear and cursor to 1,1
	sb.WriteString("\033[H\033[2J")

	// 1. Title Header Bar
	titleText := fmt.Sprintf("ISTHMUS FILE EXPLORER - PEER: %s | TRANSPORT: %s", strings.ToUpper(b.peerName), strings.ToUpper(b.transportTier))
	sb.WriteString(RetroTitleBar(titleText, b.width))
	sb.WriteString("\n")

	// 2. Current Path Bar
	sb.WriteString(fmt.Sprintf("%s%s PATH: /%s%s\n", ColorFgWhite, ColorBold, strings.TrimPrefix(b.currentPath, "./"), ColorReset))
	sb.WriteString(RetroHorizontalDivider(b.width))
	sb.WriteString("\n")

	// 3. Column Header
	sb.WriteString(fmt.Sprintf("%s %-4s %-6s %-36s %-12s %-20s%s\n",
		ColorBgGray, "", "TYPE", "NAME", "SIZE", "MODIFIED", ColorReset))
	sb.WriteString(RetroHorizontalDivider(b.width))
	sb.WriteString("\n")

	// 4. File Entries List
	maxVisible := b.height - 7
	if maxVisible < 5 {
		maxVisible = 5
	}

	if len(b.entries) == 0 {
		sb.WriteString(fmt.Sprintf("%s   (Empty directory)%s\n", ColorFgGray, ColorReset))
	} else {
		for i := b.scrollOffset; i < len(b.entries) && i < b.scrollOffset+maxVisible; i++ {
			entry := b.entries[i]
			isSel := (i == b.selectedIndex)

			cursor := "  "
			rowBg := ""
			rowFg := ColorFgWhite
			if isSel {
				cursor = "->"
				rowBg = ColorBgWinBlue
				rowFg = ColorFgWhite
			}

			typeStr := SymFile
			if entry.IsDir() {
				typeStr = SymDir
			}

			name := entry.Name()
			if len(name) > 34 {
				name = name[:31] + "..."
			}

			sizeStr := fmt.Sprintf("%d B", entry.Size())
			if entry.IsDir() {
				sizeStr = "<DIR>"
			} else if entry.Size() >= 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(entry.Size())/(1024*1024))
			} else if entry.Size() >= 1024 {
				sizeStr = fmt.Sprintf("%.1f KB", float64(entry.Size())/1024)
			}

			modTime := entry.ModTime().Format("2006-01-02 15:04:05")

			sb.WriteString(fmt.Sprintf("%s%s %-2s %-6s %-36s %-12s %-20s%s\n",
				rowBg, rowFg, cursor, typeStr, name, sizeStr, modTime, ColorReset))
		}
	}

	// 5. Fill blank rows
	rowsRendered := len(b.entries) - b.scrollOffset
	if rowsRendered < 0 {
		rowsRendered = 0
	}
	if rowsRendered > maxVisible {
		rowsRendered = maxVisible
	}
	for i := rowsRendered; i < maxVisible; i++ {
		sb.WriteString("\n")
	}

	// 6. Status Message Area
	sb.WriteString(RetroHorizontalDivider(b.width))
	sb.WriteString("\n")
	if b.statusMsg != "" && time.Since(b.statusTime) < 8*time.Second {
		sb.WriteString(fmt.Sprintf("%s%s %s%s\n", ColorFgWhite, SymNode, b.statusMsg, ColorReset))
	} else {
		sb.WriteString(fmt.Sprintf("%s Total items: %d%s\n", ColorFgGray, len(b.entries), ColorReset))
	}

	// 7. Retro Status Shortcut Bar
	shortcuts := "[ENTER] Open   [BACKSPACE] Up   [D] Download   [S] Sync   [R] Refresh   [Q] Quit"
	sb.WriteString(RetroStatusBar(shortcuts, b.width))

	fmt.Print(sb.String())
}
