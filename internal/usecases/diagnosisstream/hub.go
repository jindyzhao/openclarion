// Package diagnosisstream relays transient diagnosis previews inside one
// OpenClarion process. Durable recovery always comes from Temporal Query and
// persisted ChatTurns, never from this best-effort hub.
package diagnosisstream

import (
	"strings"
	"sync"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type streamKey struct {
	sessionID string
	messageID string
}

// Hub is a concurrency-safe, process-local preview broker. Each subscriber
// keeps only the newest full snapshot so a slow browser cannot backpressure a
// Temporal Activity or accumulate unbounded token events.
type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[streamKey]map[uint64]chan ports.DiagnosisTurnStreamEvent
}

var (
	_ ports.DiagnosisTurnStreamSink   = (*Hub)(nil)
	_ ports.DiagnosisTurnStreamSource = (*Hub)(nil)
)

// NewHub constructs an empty process-local stream broker.
func NewHub() *Hub {
	return &Hub{subscribers: make(map[streamKey]map[uint64]chan ports.DiagnosisTurnStreamEvent)}
}

// SubscribeDiagnosisTurnStream registers one bounded latest-snapshot channel.
func (h *Hub) SubscribeDiagnosisTurnStream(sessionID, messageID string) (<-chan ports.DiagnosisTurnStreamEvent, func()) {
	if h == nil {
		closed := make(chan ports.DiagnosisTurnStreamEvent)
		close(closed)
		return closed, func() {}
	}
	key := streamKey{sessionID: strings.TrimSpace(sessionID), messageID: strings.TrimSpace(messageID)}
	ch := make(chan ports.DiagnosisTurnStreamEvent, 1)
	if key.sessionID == "" || key.messageID == "" {
		close(ch)
		return ch, func() {}
	}

	h.mu.Lock()
	h.nextID++
	id := h.nextID
	if h.subscribers[key] == nil {
		h.subscribers[key] = make(map[uint64]chan ports.DiagnosisTurnStreamEvent)
	}
	h.subscribers[key][id] = ch
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			subscribers := h.subscribers[key]
			if subscriber, ok := subscribers[id]; ok {
				delete(subscribers, id)
				close(subscriber)
			}
			if len(subscribers) == 0 {
				delete(h.subscribers, key)
			}
		})
	}
	return ch, cancel
}

// PublishDiagnosisTurnStream replaces each subscriber's stale snapshot with
// the newest one. Events without a routing key are ignored defensively.
func (h *Hub) PublishDiagnosisTurnStream(event ports.DiagnosisTurnStreamEvent) {
	if h == nil || strings.TrimSpace(event.SessionID) == "" || strings.TrimSpace(event.MessageID) == "" {
		return
	}
	key := streamKey{sessionID: strings.TrimSpace(event.SessionID), messageID: strings.TrimSpace(event.MessageID)}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subscribers[key] {
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- event:
		default:
		}
	}
}
