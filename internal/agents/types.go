package agents

import (
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// UnifiedAgent represents an active agent regardless of origin (local or remote).
// Local vs. remote is derived: an agent is local when MachineID matches the
// current machine's fingerprint.
type UnifiedAgent struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`       // "claude", "kimi", "gemini", etc.
	ProjectPath string    `json:"project_path"`
	SessionID   string    `json:"session_id"`
	Hostname    string    `json:"hostname"`
	Status      string    `json:"status"` // "active", "stale", "ended"
	DetectedAt  time.Time `json:"detected_at"`
	LastSeen    time.Time `json:"last_seen"`
	MachineID   string    `json:"machine_id"`   // fingerprint
	MachineName string    `json:"machine_name"` // human-friendly hostname

	// Local-only fields
	Method      string `json:"method,omitempty"`       // "process", "ide_lock", "mtime"
	IDE         string `json:"ide,omitempty"`
	PID         int    `json:"pid,omitempty"`
	SessionPath string `json:"session_path,omitempty"` // filesystem path for direct tailing

	// Remote-only fields
	InstanceID   string `json:"instance_id,omitempty"`
	Region       string `json:"region,omitempty"`
	Version      string `json:"version,omitempty"`
	TraceCount   int64  `json:"trace_count,omitempty"`
	CollectorURL string `json:"collector_url,omitempty"`
}

// IsLocal returns true if this agent runs on the same machine as the caller.
func (a UnifiedAgent) IsLocal(localFingerprint string) bool {
	return a.MachineID == localFingerprint
}

// AgentFilter holds criteria for filtering the agent list.
type AgentFilter struct {
	Source     string // filter by source type
	Status     string // filter by status
	MachineID  string // filter by specific machine
	LocalOnly  bool   // only local agents
	RemoteOnly bool   // only remote agents
}

// Matches returns true if the agent satisfies all filter criteria.
// localFP is the current machine's fingerprint (used for LocalOnly/RemoteOnly).
func (f AgentFilter) Matches(a UnifiedAgent, localFP string) bool {
	if f.Source != "" && a.Source != f.Source {
		return false
	}
	if f.Status != "" && a.Status != f.Status {
		return false
	}
	if f.MachineID != "" && a.MachineID != f.MachineID {
		return false
	}
	if f.LocalOnly && !a.IsLocal(localFP) {
		return false
	}
	if f.RemoteOnly && a.IsLocal(localFP) {
		return false
	}
	return true
}

// AgentEvent signals a change in the agent list.
type AgentEvent struct {
	Type  string       `json:"type"` // "added", "removed", "updated"
	Agent UnifiedAgent `json:"agent"`
}

// StreamEntry is a single conversation entry received from a live stream.
type StreamEntry struct {
	Timestamp     time.Time              `json:"timestamp"`
	Role          string                 `json:"role"`          // "user", "assistant", "tool_use", "tool_result"
	Text          string                 `json:"text,omitempty"`
	Model         string                 `json:"model,omitempty"`
	ToolName      string                 `json:"tool_name,omitempty"`
	IsError       bool                   `json:"is_error,omitempty"`
	InputTokens   int                    `json:"input_tokens,omitempty"`
	OutputTokens  int                    `json:"output_tokens,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"` // for context in streams
	Synthetic     bool                   `json:"synthetic,omitempty"`  // true for connection status messages
	ContentBlocks []thinkt.ContentBlock  `json:"content_blocks,omitempty"`
}

// ToThinktEntry converts a StreamEntry to a thinkt.Entry for rendering.
func (e StreamEntry) ToThinktEntry() thinkt.Entry {
	entry := thinkt.Entry{
		Role:      thinkt.Role(e.Role),
		Timestamp: e.Timestamp,
		Model:     e.Model,
		Text:      e.Text,
	}
	if len(e.ContentBlocks) > 0 {
		entry.ContentBlocks = e.ContentBlocks
		entry.Text = "" // prefer blocks over flat text
	}
	return entry
}
