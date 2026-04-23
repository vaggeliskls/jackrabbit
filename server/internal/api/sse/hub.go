package sse

import (
	"encoding/json"
	"sync"

	"github.com/rs/zerolog/log"
)

type Event struct {
	Type string
	Data interface{}
}

type Hub struct {
	mu          sync.RWMutex
	connections map[string][]chan Event
}

func NewHub() *Hub {
	return &Hub{
		connections: make(map[string][]chan Event),
	}
}

func (h *Hub) Register(slug string) <-chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 10)
	h.connections[slug] = append(h.connections[slug], ch)

	log.Info().Str("slug", slug).Int("connections", len(h.connections[slug])).Msg("SSE connection registered")
	return ch
}

func (h *Hub) Unregister(slug string, ch <-chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.connections[slug]
	for i, c := range conns {
		if c == ch {
			close(c)
			h.connections[slug] = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	if len(h.connections[slug]) == 0 {
		delete(h.connections, slug)
	}

	log.Info().Str("slug", slug).Int("remaining", len(h.connections[slug])).Msg("SSE connection unregistered")
}

// Send delivers an event to all SSE connections for the given slug.
// Returns true if at least one connection received the event.
func (h *Hub) Send(slug string, event Event) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conns := h.connections[slug]
	if len(conns) == 0 {
		log.Debug().Str("slug", slug).Msg("no active SSE connections for runner")
		return false
	}

	for _, ch := range conns {
		select {
		case ch <- event:
		default:
			log.Warn().Str("slug", slug).Msg("SSE channel full, dropping event")
		}
	}

	log.Debug().Str("slug", slug).Str("event", event.Type).Int("sent_to", len(conns)).Msg("SSE event sent")
	return true
}

func FormatSSE(event Event) ([]byte, error) {
	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}

	return []byte("event: " + event.Type + "\ndata: " + string(dataJSON) + "\n\n"), nil
}
