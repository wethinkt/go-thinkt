package agents

import (
	"testing"
	"time"
)

func TestAgentHub_ListEmpty(t *testing.T) {
	hub := NewHub(HubConfig{})
	agents := hub.List(AgentFilter{})
	if len(agents) != 0 {
		t.Errorf("expected empty list, got %d", len(agents))
	}
}

func TestAgentHub_Subscribe(t *testing.T) {
	hub := NewHub(HubConfig{})

	ch, unsub := hub.Subscribe()
	defer unsub()

	// Simulate an agent being added
	hub.mu.Lock()
	agent := UnifiedAgent{ID: "test-1", Source: "claude", Status: "active"}
	hub.agents = append(hub.agents, agent)
	hub.notify(AgentEvent{Type: "added", Agent: agent})
	hub.mu.Unlock()

	select {
	case e := <-ch:
		if e.Type != "added" || e.Agent.ID != "test-1" {
			t.Errorf("unexpected event: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestAgentHub_ListWithFilter(t *testing.T) {
	hub := NewHub(HubConfig{})
	hub.localFP = "local-fp"
	hub.agents = []UnifiedAgent{
		{ID: "1", Source: "claude", MachineID: "local-fp", Status: "active"},
		{ID: "2", Source: "kimi", MachineID: "remote-fp", Status: "active"},
		{ID: "3", Source: "claude", MachineID: "remote-fp", Status: "stale"},
	}

	tests := []struct {
		name   string
		filter AgentFilter
		want   int
	}{
		{"all", AgentFilter{}, 3},
		{"local only", AgentFilter{LocalOnly: true}, 1},
		{"remote only", AgentFilter{RemoteOnly: true}, 2},
		{"source claude", AgentFilter{Source: "claude"}, 2},
		{"active only", AgentFilter{Status: "active"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hub.List(tt.filter)
			if len(got) != tt.want {
				t.Errorf("List(%+v) returned %d agents, want %d", tt.filter, len(got), tt.want)
			}
		})
	}
}
