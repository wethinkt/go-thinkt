package collect

import (
	"testing"
	"time"
)

func TestTicketStore_IssueAndRedeem(t *testing.T) {
	ts := NewTicketStore()

	ticket := ts.Issue("session-1")
	if ticket == "" {
		t.Fatal("expected non-empty ticket")
	}

	// Redeem should succeed
	if !ts.Redeem(ticket, "session-1") {
		t.Error("expected redeem to succeed")
	}

	// Second redeem should fail (burned)
	if ts.Redeem(ticket, "session-1") {
		t.Error("expected second redeem to fail")
	}
}

func TestTicketStore_WrongSession(t *testing.T) {
	ts := NewTicketStore()
	ticket := ts.Issue("session-1")

	if ts.Redeem(ticket, "session-2") {
		t.Error("expected redeem to fail for wrong session")
	}
}

func TestTicketStore_Expired(t *testing.T) {
	ts := NewTicketStore()
	ts.ttl = 1 * time.Millisecond // override for test

	ticket := ts.Issue("session-1")
	time.Sleep(5 * time.Millisecond)

	if ts.Redeem(ticket, "session-1") {
		t.Error("expected expired ticket to fail")
	}
}

func TestTicketStore_Cleanup(t *testing.T) {
	ts := NewTicketStore()
	ts.ttl = 1 * time.Millisecond

	ts.Issue("s1")
	ts.Issue("s2")
	time.Sleep(5 * time.Millisecond)

	ts.Cleanup()

	ts.mu.Lock()
	count := len(ts.tickets)
	ts.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 tickets after cleanup, got %d", count)
	}
}
