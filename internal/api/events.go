package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// eventHub fans out daemon RPC events to connected SSE clients.
type eventHub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[chan []byte]struct{})}
}

func (h *eventHub) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *eventHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *eventHub) broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

func (s *Server) startEventForwarder() {
	if s.events == nil || s.client == nil {
		return
	}
	go func() {
		for ev := range s.client.Events() {
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			s.events.broadcast(payload)
		}
	}()
}

// handleEvents streams daemon push notifications (crawl.progress, crawl.done, db.changed).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		badRequest(w, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.events.subscribe()
	defer s.events.unsubscribe(ch)

	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: daemon\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
