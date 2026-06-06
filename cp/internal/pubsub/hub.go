// Package pubsub provides a per-key fan-out message hub used for SSE live tailing.
package pubsub

import (
	"encoding/json"
	"sync"
)

// Message is one published event.
type Message struct {
	ID   string          // SSE event id (created_at RFC3339)
	Data json.RawMessage // JSON payload
}

// Hub is a thread-safe per-key fan-out hub.
type Hub struct {
	mu   sync.RWMutex
	subs map[string][]chan Message
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string][]chan Message)}
}

// Subscribe returns a buffered channel for key and a cancel func.
// The cancel func must be called to avoid goroutine leaks.
func (h *Hub) Subscribe(key string, bufSize int) (<-chan Message, func()) {
	c := make(chan Message, bufSize)
	h.mu.Lock()
	h.subs[key] = append(h.subs[key], c)
	h.mu.Unlock()
	return c, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		list := h.subs[key]
		for i, s := range list {
			if s == c {
				h.subs[key] = append(list[:i], list[i+1:]...)
				close(c)
				break
			}
		}
		if len(h.subs[key]) == 0 {
			delete(h.subs, key)
		}
	}
}

// Publish sends msg to all current subscribers for key. Slow subscribers are dropped.
func (h *Hub) Publish(key string, msg Message) {
	h.mu.RLock()
	subs := make([]chan Message, len(h.subs[key]))
	copy(subs, h.subs[key])
	h.mu.RUnlock()
	for _, c := range subs {
		select {
		case c <- msg:
		default:
		}
	}
}
