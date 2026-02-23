// Package export provides a trace exporter that watches local AI session files
// and ships them to a remote collector endpoint via HTTP POST.
package export

import "time"

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
	WatchDirs []string

	// MaxBufferMB is the maximum buffer size in megabytes. Default: 100.
	MaxBufferMB int

	// BatchSize is the number of entries per batch POST. Default: 100.
	BatchSize int

	// FlushInterval controls how often buffered entries are flushed. Default: 5s.
	FlushInterval time.Duration

	// Quiet suppresses non-error output when true.
	Quiet bool
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
	UUID      string    `json:"uuid"`
	Role      string    `json:"role"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text,omitempty"`
	Model     string    `json:"model,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
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
	Watching        []string
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

// CollectorEndpoint holds a discovered collector URL and its origin.
type CollectorEndpoint struct {
	URL    string `json:"url"`
	Origin string `json:"origin"` // "env", "project", "well-known", "local"
}
