// Package export provides a trace exporter that watches local AI session files
// and ships them to a remote collector endpoint via HTTP POST.
package export

import (
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// WatchDir pairs a directory path with its source identity and watch configuration.
type WatchDir struct {
	Path   string            // Absolute path to the source's base directory
	Source string            // Source name (e.g. "claude", "kimi")
	Config thinkt.WatchConfig // Per-source watch configuration
}

// ExporterConfig holds configuration for the trace exporter.
type ExporterConfig struct {
	// CollectorURL is the endpoint to POST traces to (e.g. "https://collect.wethinkt.com/v1/traces").
	// If empty, discovery will be used.
	CollectorURL string

	// APIKey is the Bearer token for collector authentication.
	APIKey string

	// BufferDir is the local disk buffer directory for when the collector is unreachable.
	// Defaults to ~/.thinkt/export-buffer/.
	BufferDir string

	// WatchDirs are directories to watch for new/modified JSONL session files.
	WatchDirs []WatchDir

	// MaxBufferMB is the maximum buffer size in megabytes. Default: 100.
	MaxBufferMB int

	// BatchSize is the number of entries per batch POST. Default: 100.
	BatchSize int

	// FlushInterval controls how often buffered entries are flushed. Default: 5s.
	FlushInterval time.Duration

	// Quiet suppresses non-error output when true.
	Quiet bool

	// Version is the exporter version string (set via ldflags).
	Version string
}

// Defaults applies default values to unset config fields.
func (c *ExporterConfig) Defaults() {
	if c.BufferDir == "" {
		c.BufferDir = "~/.thinkt/export-buffer/"
	}
	if c.MaxBufferMB == 0 {
		c.MaxBufferMB = 100
	}
	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = 5 * time.Second
	}
}

// TracePayload is the HTTP POST body for /v1/traces.
type TracePayload struct {
	InstanceID  string         `json:"instance_id"`
	Source      string         `json:"source"`       // "claude", "kimi", etc.
	ProjectPath string        `json:"project_path"`
	SessionID   string         `json:"session_id"`
	Entries     []TraceEntry   `json:"entries"`
	MachineID   string         `json:"machine_id,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TraceEntry is a single trace entry in the payload.
type TraceEntry struct {
	UUID         string    `json:"uuid"`
	Role         string    `json:"role"`
	Timestamp    time.Time `json:"timestamp"`
	Text         string    `json:"text,omitempty"`
	Model        string    `json:"model,omitempty"`
	ToolName     string    `json:"tool_name,omitempty"`
	AgentID      string    `json:"agent_id,omitempty"`
	IsError      bool      `json:"is_error,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	ThinkingLen  int       `json:"thinking_len,omitempty"`
	HasThinking  bool      `json:"has_thinking,omitempty"`
	HasToolUse   bool      `json:"has_tool_use,omitempty"`
}

// ShipResult tracks the result of shipping a batch.
type ShipResult struct {
	BatchID    string
	Entries    int
	StatusCode int
	Error      error
	Duration   time.Duration
}

// ExporterStats reports current exporter state.
type ExporterStats struct {
	TracesShipped   int64
	TracesFailed    int64
	TracesBuffered  int64
	BufferSizeBytes int64
	LastShipTime    time.Time
	CollectorURL    string
	Watching        []WatchDir
}

// SessionActivityEvent is sent to the collector to report session lifecycle changes.
type SessionActivityEvent struct {
	InstanceID  string    `json:"instance_id"`
	Source      string    `json:"source"`
	ProjectPath string    `json:"project_path"`
	SessionID   string    `json:"session_id"`
	Event       string    `json:"event"` // "session_start", "session_active", "session_end"
	Timestamp   time.Time `json:"timestamp"`
}

// AgentRegistration is the POST body for /v1/agents/register.
type AgentRegistration struct {
	InstanceID string         `json:"instance_id"`
	Platform   string         `json:"platform"`
	Hostname   string         `json:"hostname,omitempty"`
	Version    string         `json:"version,omitempty"`
	MachineID  string         `json:"machine_id,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// CollectorEndpoint holds a discovered collector URL and its origin.
type CollectorEndpoint struct {
	URL    string `json:"url"`
	Origin string `json:"origin"` // "env", "project", "well-known", "local"
}
