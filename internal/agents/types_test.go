package agents

import (
	"testing"
	"time"
)

func TestUnifiedAgent_IsLocal(t *testing.T) {
	localFP := "abc-123"

	agent := UnifiedAgent{
		ID:        "sess-1",
		MachineID: "abc-123",
	}
	if !agent.IsLocal(localFP) {
		t.Error("expected agent to be local")
	}

	remote := UnifiedAgent{
		ID:        "sess-2",
		MachineID: "def-456",
	}
	if remote.IsLocal(localFP) {
		t.Error("expected agent to be remote")
	}
}

func TestAgentFilter_Matches(t *testing.T) {
	agent := UnifiedAgent{
		ID:        "sess-1",
		Source:    "claude",
		Status:    "active",
		MachineID: "abc-123",
	}
	localFP := "abc-123"

	tests := []struct {
		name   string
		filter AgentFilter
		want   bool
	}{
		{"empty filter matches all", AgentFilter{}, true},
		{"source match", AgentFilter{Source: "claude"}, true},
		{"source mismatch", AgentFilter{Source: "kimi"}, false},
		{"status match", AgentFilter{Status: "active"}, true},
		{"status mismatch", AgentFilter{Status: "stale"}, false},
		{"local only match", AgentFilter{LocalOnly: true}, true},
		{"remote only mismatch", AgentFilter{RemoteOnly: true}, false},
		{"machine match", AgentFilter{MachineID: "abc-123"}, true},
		{"machine mismatch", AgentFilter{MachineID: "def-456"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Matches(agent, localFP)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStreamEntry_Fields(t *testing.T) {
	e := StreamEntry{
		Timestamp: time.Now(),
		Role:      "assistant",
		Text:      "hello",
		Model:     "claude-sonnet-4-5-20250929",
	}
	if e.Role != "assistant" {
		t.Error("unexpected role")
	}
}
