package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"isthmus/internal/logger"
)

type EventType string

const (
	EventCreate EventType = "CREATE"
	EventModify EventType = "MODIFY"
	EventDelete EventType = "DELETE"
	EventRename EventType = "RENAME"
)

type WatchEvent struct {
	Path      string    `json:"path"`
	RelPath   string    `json:"rel_path"`
	Type      EventType `json:"type"`
	IsDir     bool      `json:"is_dir"`
	Timestamp time.Time `json:"timestamp"`
}

type Options struct {
	DebounceDelay time.Duration
	IgnoredPaths  []string
}

type FolderWatcher struct {
	rootDir string
	options Options
	watcher *fsnotify.Watcher
	log     *logger.Logger

	subscribers []func(WatchEvent)
	subMu       sync.RWMutex

	pendingEvents map[string]WatchEvent
	pendingMu     sync.Mutex
	debounceTimer *time.Timer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewFolderWatcher(rootDir string, opts Options) (*FolderWatcher, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}

	if opts.DebounceDelay == 0 {
		opts.DebounceDelay = 400 * time.Millisecond
	}
	if len(opts.IgnoredPaths) == 0 {
		opts.IgnoredPaths = []string{".git", ".ssh", ".tmp", ".isthmus", "node_modules"}
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FolderWatcher{
		rootDir:       absRoot,
		options:       opts,
		watcher:       fsw,
		log:           logger.WithPrefix("FolderWatcher"),
		pendingEvents: make(map[string]WatchEvent),
		ctx:           ctx,
		cancel:        cancel,
	}

	return fw, nil
}

func (fw *FolderWatcher) OnChange(cb func(WatchEvent)) {
	fw.subMu.Lock()
	defer fw.subMu.Unlock()
	fw.subscribers = append(fw.subscribers, cb)
}

func (fw *FolderWatcher) Start() error {
	// Add root directory and all subdirectories recursively
	if err := fw.addRecursive(fw.rootDir); err != nil {
		return err
	}

	fw.wg.Add(1)
	go fw.eventLoop()

	fw.log.Info("Watching directory for live real-time changes: %s", fw.rootDir)
	return nil
}

func (fw *FolderWatcher) Stop() {
	fw.cancel()
	_ = fw.watcher.Close()
	fw.wg.Wait()
}

func (fw *FolderWatcher) eventLoop() {
	defer fw.wg.Done()

	for {
		select {
		case <-fw.ctx.Done():
			return
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.log.Error("Watcher error: %v", err)
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleFSEvent(event)
		}
	}
}

func (fw *FolderWatcher) handleFSEvent(event fsnotify.Event) {
	path := event.Name
	if fw.isIgnored(path) {
		return
	}

	rel, err := filepath.Rel(fw.rootDir, path)
	if err != nil {
		rel = filepath.Base(path)
	}

	var evType EventType
	switch {
	case event.Has(fsnotify.Create):
		evType = EventCreate
		// If a new directory was created, watch it recursively
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			_ = fw.addRecursive(path)
		}
	case event.Has(fsnotify.Write):
		evType = EventModify
	case event.Has(fsnotify.Remove):
		evType = EventDelete
	case event.Has(fsnotify.Rename):
		evType = EventRename
	default:
		return
	}

	isDir := false
	if fi, err := os.Stat(path); err == nil {
		isDir = fi.IsDir()
	}

	we := WatchEvent{
		Path:      path,
		RelPath:   rel,
		Type:      evType,
		IsDir:     isDir,
		Timestamp: time.Now(),
	}

	fw.pendingMu.Lock()
	fw.pendingEvents[rel] = we
	if fw.debounceTimer != nil {
		fw.debounceTimer.Stop()
	}
	fw.debounceTimer = time.AfterFunc(fw.options.DebounceDelay, fw.flushEvents)
	fw.pendingMu.Unlock()
}

func (fw *FolderWatcher) flushEvents() {
	fw.pendingMu.Lock()
	events := make([]WatchEvent, 0, len(fw.pendingEvents))
	for _, ev := range fw.pendingEvents {
		events = append(events, ev)
	}
	fw.pendingEvents = make(map[string]WatchEvent)
	fw.pendingMu.Unlock()

	fw.subMu.RLock()
	subs := make([]func(WatchEvent), len(fw.subscribers))
	copy(subs, fw.subscribers)
	fw.subMu.RUnlock()

	for _, ev := range events {
		for _, sub := range subs {
			go sub(ev)
		}
	}
}

func (fw *FolderWatcher) addRecursive(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if fw.isIgnored(path) && path != fw.rootDir {
				return filepath.SkipDir
			}
			return fw.watcher.Add(path)
		}
		return nil
	})
}

func (fw *FolderWatcher) isIgnored(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") && base != "." {
		return true
	}
	for _, ig := range fw.options.IgnoredPaths {
		if strings.Contains(path, string(filepath.Separator)+ig) || base == ig {
			return true
		}
	}
	return false
}
