package app

import (
	"encoding/json"
	"sync"
)

type EventHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newHub() *EventHub { return &EventHub{clients: map[chan []byte]struct{}{}} }
func (h *EventHub) Emit(kind string, v any) {
	b, _ := json.Marshal(map[string]any{"type": kind, "data": v})
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- b:
		default:
		}
	}
}
func (h *EventHub) subscribe() chan []byte {
	c := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}
func (h *EventHub) unsubscribe(c chan []byte) {
	h.mu.Lock()
	delete(h.clients, c)
	close(c)
	h.mu.Unlock()
}
