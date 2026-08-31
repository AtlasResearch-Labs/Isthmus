package share

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type ShareToken struct {
	Token         string    `json:"token"`
	PeerID        string    `json:"peer_id"`
	FilePath      string    `json:"file_path"`
	Filename      string    `json:"filename"`
	ExpiresAt     time.Time `json:"expires_at"`
	MaxDownloads  int       `json:"max_downloads"`
	DownloadCount int       `json:"download_count"`
}

type Manager struct {
	tokens map[string]*ShareToken
	log    *logger.Logger
	mu     sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		tokens: make(map[string]*ShareToken),
		log:    logger.WithPrefix("ShareLinks"),
	}
}

func (m *Manager) CreateLink(peerID, filePath, filename string, ttl time.Duration, maxDownloads int) *ShareToken {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	if maxDownloads <= 0 {
		maxDownloads = 5
	}

	randBytes := make([]byte, 12)
	_, _ = rand.Read(randBytes)
	tokenStr := hex.EncodeToString(randBytes)

	st := &ShareToken{
		Token:         tokenStr,
		PeerID:        peerID,
		FilePath:      filePath,
		Filename:      filename,
		ExpiresAt:     time.Now().Add(ttl),
		MaxDownloads:  maxDownloads,
		DownloadCount: 0,
	}

	m.tokens[tokenStr] = st
	m.log.Info("Created share token %s for '%s' (TTL: %v)", tokenStr, filename, ttl)
	return st
}

func (m *Manager) ValidateAndConsume(tokenStr string) (*ShareToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	st, exists := m.tokens[tokenStr]
	if !exists {
		return nil, fmt.Errorf("invalid or expired share token")
	}

	if time.Now().After(st.ExpiresAt) {
		delete(m.tokens, tokenStr)
		return nil, fmt.Errorf("share link has expired")
	}

	if st.MaxDownloads > 0 && st.DownloadCount >= st.MaxDownloads {
		delete(m.tokens, tokenStr)
		return nil, fmt.Errorf("download limit reached for this share link")
	}

	st.DownloadCount++
	return st, nil
}

func (m *Manager) ListActive() []*ShareToken {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	res := make([]*ShareToken, 0, len(m.tokens))
	for _, t := range m.tokens {
		if now.Before(t.ExpiresAt) && (t.MaxDownloads == 0 || t.DownloadCount < t.MaxDownloads) {
			res = append(res, t)
		}
	}
	return res
}
