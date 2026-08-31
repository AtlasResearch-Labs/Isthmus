package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"isthmus/internal/logger"
)

type EventType string

const (
	EventPeerConnected    EventType = "peer_connected"
	EventPeerDisconnected EventType = "peer_disconnected"
	EventFileSynced       EventType = "file_synced"
	EventTransferFinished EventType = "transfer_finished"
	EventBatteryLow       EventType = "battery_low"
	EventDiskLow          EventType = "disk_low"
	EventVaultLocked      EventType = "vault_locked"
	EventVaultUnlocked    EventType = "vault_unlocked"
	EventJobCompleted     EventType = "job_completed"
	EventSystemNotice     EventType = "system_notice"
)

type Event struct {
	ID        string      `json:"id"`
	Type      EventType   `json:"type"`
	Title     string      `json:"title"`
	Message   string      `json:"message"`
	Level     string      `json:"level"` // "info", "success", "warning", "danger"
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

type Broker struct {
	subscribers map[chan Event]struct{}
	log         *logger.Logger
	history     []Event
	mu          sync.RWMutex
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan Event]struct{}),
		log:         logger.WithPrefix("EventBroker"),
		history:     make([]Event, 0, 50),
	}
}

// Publish broadcasts an event to all connected SSE clients
func (b *Broker) Publish(evType EventType, title, message, level string, data interface{}) Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ev := Event{
		ID:        fmt.Sprintf("ev_%d", time.Now().UnixNano()),
		Type:      evType,
		Title:     title,
		Message:   message,
		Level:     level,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Keep last 50 events in history
	if len(b.history) >= 50 {
		b.history = b.history[1:]
	}
	b.history = append(b.history, ev)

	// Broadcast to active channels non-blocking
	for ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}

	b.log.Info("[%s] %s: %s", ev.Type, ev.Title, ev.Message)
	return ev
}

// GetHistory returns recent events
func (b *Broker) GetHistory() []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	res := make([]Event, len(b.history))
	copy(res, b.history)
	return res
}

// ServeHTTP implements Server-Sent Events (SSE) streaming endpoint
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan Event, 20)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		close(ch)
		b.mu.Unlock()
	}()

	// Send initial ping event
	initEvent := Event{
		ID:        "init",
		Type:      EventSystemNotice,
		Title:     "Connected",
		Message:   "Subscribed to Isthmus Real-Time Event Stream",
		Level:     "info",
		Timestamp: time.Now(),
	}
	initJSON, _ := json.Marshal(initEvent)
	fmt.Fprintf(w, "data: %s\n\n", string(initJSON))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			data, err := json.Marshal(ev)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	}
}
