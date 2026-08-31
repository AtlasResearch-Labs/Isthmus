package clipboard

import (
	"sync"
	"time"

	"isthmus/internal/logger"
)

type ClipboardItem struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
}

type Manager struct {
	history []ClipboardItem
	maxSize int
	log     *logger.Logger
	mu      sync.RWMutex
}

func NewManager(maxSize int) *Manager {
	if maxSize <= 0 {
		maxSize = 50
	}
	return &Manager{
		history: make([]ClipboardItem, 0, maxSize),
		maxSize: maxSize,
		log:     logger.WithPrefix("Clipboard"),
	}
}

func (m *Manager) Set(content string, source string) ClipboardItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := ClipboardItem{
		ID:        time.Now().Format("20060102150405.000"),
		Content:   content,
		Source:    source,
		Timestamp: time.Now(),
	}

	// Prepend to history
	m.history = append([]ClipboardItem{item}, m.history...)
	if len(m.history) > m.maxSize {
		m.history = m.history[:m.maxSize]
	}

	m.log.Info("Clipboard synced from '%s' (%d bytes)", source, len(content))
	return item
}

func (m *Manager) GetLatest() *ClipboardItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.history) == 0 {
		return nil
	}
	item := m.history[0]
	return &item
}

func (m *Manager) GetHistory() []ClipboardItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]ClipboardItem, len(m.history))
	copy(res, m.history)
	return res
}

func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = make([]ClipboardItem, 0, m.maxSize)
}
