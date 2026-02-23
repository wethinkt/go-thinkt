package collect

import (
	"testing"
	"time"
)

func TestSessionPubSub_SubscribeAndPublish(t *testing.T) {
	ps := NewSessionPubSub()

	ch, unsub := ps.Subscribe("session-1")
	defer unsub()

	entries := []IngestEntry{
		{UUID: "e1", Role: "assistant", Text: "hello"},
	}
	ps.Publish("session-1", entries)

	select {
	case got := <-ch:
		if len(got) != 1 || got[0].UUID != "e1" {
			t.Errorf("unexpected entries: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published entries")
	}
}

func TestSessionPubSub_DifferentSession(t *testing.T) {
	ps := NewSessionPubSub()

	ch, unsub := ps.Subscribe("session-1")
	defer unsub()

	// Publish to a different session
	ps.Publish("session-2", []IngestEntry{{UUID: "e1"}})

	select {
	case <-ch:
		t.Fatal("should not receive entries for different session")
	case <-time.After(100 * time.Millisecond):
		// OK — no message received
	}
}

func TestSessionPubSub_Unsubscribe(t *testing.T) {
	ps := NewSessionPubSub()

	ch, unsub := ps.Subscribe("session-1")
	unsub()

	ps.Publish("session-1", []IngestEntry{{UUID: "e1"}})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		// OK — channel closed or no message
	}
}

func TestSessionPubSub_MultipleSubscribers(t *testing.T) {
	ps := NewSessionPubSub()

	ch1, unsub1 := ps.Subscribe("session-1")
	defer unsub1()
	ch2, unsub2 := ps.Subscribe("session-1")
	defer unsub2()

	ps.Publish("session-1", []IngestEntry{{UUID: "e1"}})

	for i, ch := range []<-chan []IngestEntry{ch1, ch2} {
		select {
		case got := <-ch:
			if len(got) != 1 {
				t.Errorf("subscriber %d: unexpected entries: %+v", i, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}
