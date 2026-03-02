package collect

import (
	"testing"
	"time"
)

func TestRegisterNewAgent(t *testing.T) {
	r := NewAgentRegistry()
	started := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	info := r.Register(AgentRegistration{
		InstanceID: "agent-1",
		Platform:   "darwin/arm64",
		Hostname:   "mac.local",
		StartedAt:  started,
	})

	if info.InstanceID != "agent-1" {
		t.Fatalf("got instance_id %q, want %q", info.InstanceID, "agent-1")
	}
	if !info.StartedAt.Equal(started) {
		t.Fatalf("got started_at %v, want %v", info.StartedAt, started)
	}
	if info.Status != "active" {
		t.Fatalf("got status %q, want %q", info.Status, "active")
	}
}

func TestRegisterUpdatesStartedAtOnHeartbeatCreatedAgent(t *testing.T) {
	r := NewAgentRegistry()

	// Heartbeat creates a minimal entry with zero-time StartedAt
	r.Heartbeat("agent-1")
	agents := r.List()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if !agents[0].StartedAt.IsZero() {
		t.Fatalf("heartbeat-created agent should have zero StartedAt, got %v", agents[0].StartedAt)
	}

	// Explicit registration should backfill StartedAt
	started := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	info := r.Register(AgentRegistration{
		InstanceID: "agent-1",
		Platform:   "darwin/arm64",
		Hostname:   "mac.local",
		StartedAt:  started,
	})

	if !info.StartedAt.Equal(started) {
		t.Fatalf("Register should update StartedAt on existing agent: got %v, want %v", info.StartedAt, started)
	}
	if info.Platform != "darwin/arm64" {
		t.Fatalf("got platform %q, want %q", info.Platform, "darwin/arm64")
	}
}

func TestHeartbeatCreatesMinimalEntry(t *testing.T) {
	r := NewAgentRegistry()

	existed := r.Heartbeat("agent-1")
	if existed {
		t.Fatal("first heartbeat should return false (new agent)")
	}

	existed = r.Heartbeat("agent-1")
	if !existed {
		t.Fatal("second heartbeat should return true (existing agent)")
	}

	total, active := r.Count()
	if total != 1 || active != 1 {
		t.Fatalf("expected 1 total, 1 active; got %d, %d", total, active)
	}
}

func TestIncrementTraceCount(t *testing.T) {
	r := NewAgentRegistry()

	r.IncrementTraceCount("agent-1", 50)
	agents := r.List()
	if len(agents) != 1 || agents[0].TraceCount != 50 {
		t.Fatalf("expected trace_count=50, got %d", agents[0].TraceCount)
	}

	r.IncrementTraceCount("agent-1", 25)
	agents = r.List()
	if agents[0].TraceCount != 75 {
		t.Fatalf("expected trace_count=75 after increment, got %d", agents[0].TraceCount)
	}
}

func TestCleanStale(t *testing.T) {
	r := NewAgentRegistry()

	r.Register(AgentRegistration{InstanceID: "agent-1"})

	// Force the heartbeat to be old
	r.mu.Lock()
	r.agents["agent-1"].LastHeartbeat = time.Now().Add(-10 * time.Minute)
	r.mu.Unlock()

	removed := r.CleanStale(5 * time.Minute)
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	total, _ := r.Count()
	if total != 0 {
		t.Fatalf("expected 0 agents after cleanup, got %d", total)
	}
}
