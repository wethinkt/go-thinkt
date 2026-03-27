package index

import "testing"

func TestStatusTracker(t *testing.T) {
	st := NewStatusTracker()
	snap := st.Snapshot()
	if snap.State != "idle" {
		t.Fatalf("expected idle, got %s", snap.State)
	}

	st.SetSyncing(true)
	if snap := st.Snapshot(); snap.State != "syncing" {
		t.Fatalf("expected syncing, got %s", snap.State)
	}

	st.SetEmbedding(true)
	if snap := st.Snapshot(); snap.State != "syncing+embedding" {
		t.Fatalf("expected syncing+embedding, got %s", snap.State)
	}

	st.SetSyncing(false)
	if snap := st.Snapshot(); snap.State != "embedding" {
		t.Fatalf("expected embedding, got %s", snap.State)
	}

	st.SetSummarizing(true)
	if snap := st.Snapshot(); snap.State != "embedding+summarizing" {
		t.Fatalf("expected embedding+summarizing, got %s", snap.State)
	}

	st.SetEmbedding(false)
	st.SetSummarizing(false)
	if snap := st.Snapshot(); snap.State != "idle" {
		t.Fatalf("expected idle, got %s", snap.State)
	}
}

func TestStatusTrackerProgress(t *testing.T) {
	st := NewStatusTracker()
	st.SetEmbedProgress(5, 10, "sess-1")
	snap := st.Snapshot()
	if snap.EmbedDone != 5 || snap.EmbedTotal != 10 {
		t.Fatalf("expected 5/10, got %d/%d", snap.EmbedDone, snap.EmbedTotal)
	}
	if snap.EmbedSessionID != "sess-1" {
		t.Fatalf("expected sess-1, got %s", snap.EmbedSessionID)
	}
}
