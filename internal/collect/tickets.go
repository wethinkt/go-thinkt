package collect

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const defaultTicketTTL = 30 * time.Second

// TicketStore manages short-lived, single-use tickets for WebSocket auth.
// Browser clients exchange a bearer token for a ticket, then connect with
// the ticket as a query parameter.
type TicketStore struct {
	mu      sync.Mutex
	tickets map[string]ticketEntry
	ttl     time.Duration
}

type ticketEntry struct {
	ExpiresAt time.Time
	SessionID string
}

// NewTicketStore creates a new ticket store.
func NewTicketStore() *TicketStore {
	return &TicketStore{
		tickets: make(map[string]ticketEntry),
		ttl:     defaultTicketTTL,
	}
}

// Issue creates a new single-use ticket scoped to the given session.
func (ts *TicketStore) Issue(sessionID string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	ticket := hex.EncodeToString(b)

	ts.mu.Lock()
	ts.tickets[ticket] = ticketEntry{
		ExpiresAt: time.Now().Add(ts.ttl),
		SessionID: sessionID,
	}
	ts.mu.Unlock()

	return ticket
}

// Redeem validates and burns a ticket. Returns true if valid.
func (ts *TicketStore) Redeem(ticket, sessionID string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	entry, ok := ts.tickets[ticket]
	if !ok {
		return false
	}
	delete(ts.tickets, ticket)

	if time.Now().After(entry.ExpiresAt) {
		return false
	}
	return entry.SessionID == sessionID
}

// Cleanup removes expired tickets.
func (ts *TicketStore) Cleanup() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	for k, v := range ts.tickets {
		if now.After(v.ExpiresAt) {
			delete(ts.tickets, k)
		}
	}
}
