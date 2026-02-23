package collect

import (
	"sync"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// SessionPubSub provides in-memory fan-out of ingested entries to WebSocket
// subscribers watching specific sessions.
type SessionPubSub struct {
	mu   sync.RWMutex
	subs map[string][]*subscriber
}

type subscriber struct {
	ch     chan []IngestEntry
	closed bool
}

// NewSessionPubSub creates a new pub/sub instance.
func NewSessionPubSub() *SessionPubSub {
	return &SessionPubSub{
		subs: make(map[string][]*subscriber),
	}
}

// Subscribe returns a channel that receives entry batches for the given session.
// Call the returned function to unsubscribe and close the channel.
func (ps *SessionPubSub) Subscribe(sessionID string) (<-chan []IngestEntry, func()) {
	ch := make(chan []IngestEntry, 64)
	sub := &subscriber{ch: ch}

	ps.mu.Lock()
	ps.subs[sessionID] = append(ps.subs[sessionID], sub)
	ps.mu.Unlock()

	unsub := func() {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		subs := ps.subs[sessionID]
		for i, s := range subs {
			if s == sub {
				ps.subs[sessionID] = append(subs[:i], subs[i+1:]...)
				if !s.closed {
					s.closed = true
					close(s.ch)
				}
				break
			}
		}
		if len(ps.subs[sessionID]) == 0 {
			delete(ps.subs, sessionID)
		}
	}

	return ch, unsub
}

// Publish sends entries to all subscribers watching the given session.
// Slow consumers whose buffers are full will have entries dropped.
func (ps *SessionPubSub) Publish(sessionID string, entries []IngestEntry) {
	ps.mu.RLock()
	subs := ps.subs[sessionID]
	ps.mu.RUnlock()

	for _, sub := range subs {
		if sub.closed {
			continue
		}
		select {
		case sub.ch <- entries:
		default:
			tuilog.Log.Warn("Dropping entries for slow WebSocket subscriber",
				"session_id", sessionID)
		}
	}
}
