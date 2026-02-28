// Package collect implements the trace collector server that receives,
// normalizes, and stores AI reasoning traces from exporters.
package collect

import (
	"context"
	"time"
)

// Default configuration values for the collector.
const (
	DefaultPort          = 8785
	DefaultHost          = "localhost"
	DefaultBatchSize     = 100
	DefaultFlushInterval = 2 * time.Second
)

// CollectorConfig holds configuration for the trace collector.
type CollectorConfig struct {
	Port          int
	Host          string
	DBPath        string
	Token         string // bearer token for auth
	Quiet         bool
	BatchSize     int
	FlushInterval time.Duration
}

// DefaultCollectorConfig returns a CollectorConfig with default values.
func DefaultCollectorConfig() CollectorConfig {
	return CollectorConfig{
		Port:          DefaultPort,
		Host:          DefaultHost,
		BatchSize:     DefaultBatchSize,
		FlushInterval: DefaultFlushInterval,
	}
}

// IngestRequest is the POST /v1/traces request body sent by exporters.
type IngestRequest struct {
	InstanceID  string         `json:"instance_id"`
	Source      string         `json:"source"`
	ProjectPath string         `json:"project_path"`
	SessionID   string         `json:"session_id"`
	Entries     []IngestEntry  `json:"entries"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// IngestEntry is a single trace entry within an ingest payload.
type IngestEntry struct {
	UUID         string    `json:"uuid"`
	Role         string    `json:"role"`
	Timestamp    time.Time `json:"timestamp"`
	Model        string    `json:"model,omitempty"`
	Text         string    `json:"text,omitempty"`
	ToolName     string    `json:"tool_name,omitempty"`
	IsError      bool      `json:"is_error,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	ThinkingLen  int       `json:"thinking_len,omitempty"`
	HasThinking  bool      `json:"has_thinking,omitempty"`
	HasToolUse   bool      `json:"has_tool_use,omitempty"`
}

// IngestResponse is returned by POST /v1/traces.
type IngestResponse struct {
	Accepted int    `json:"accepted"`
	Message  string `json:"message,omitempty"`
}

// AgentRegistration is the POST /v1/agents/register request body.
type AgentRegistration struct {
	InstanceID string         `json:"instance_id"`
	Platform   string         `json:"platform"`
	Region     string         `json:"region,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	Project    string         `json:"project,omitempty"`
	Hostname   string         `json:"hostname,omitempty"`
	Version    string         `json:"version,omitempty"`
	MachineID  string         `json:"machine_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// AgentInfo represents a registered exporter agent and its status.
type AgentInfo struct {
	InstanceID    string         `json:"instance_id"`
	Platform      string         `json:"platform"`
	Region        string         `json:"region,omitempty"`
	Hostname      string         `json:"hostname,omitempty"`
	Version       string         `json:"version,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	LastHeartbeat time.Time      `json:"last_heartbeat"`
	TraceCount    int64          `json:"trace_count"`
	Project       string         `json:"project,omitempty"`
	Status        string         `json:"status"` // "active" or "stale"
	MachineID     string         `json:"machine_id,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// CollectorStats contains aggregate collector statistics.
type CollectorStats struct {
	TotalTraces   int64     `json:"total_traces"`
	TotalSessions int64     `json:"total_sessions"`
	TotalAgents   int       `json:"total_agents"`
	ActiveAgents  int       `json:"active_agents"`
	DBSizeBytes   int64     `json:"db_size_bytes"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	StartedAt     time.Time `json:"started_at"`
}

// TraceStore is the storage interface for the collector.
type TraceStore interface {
	// IngestBatch writes a batch of trace entries from an ingest request.
	IngestBatch(ctx context.Context, req IngestRequest) error
	// QuerySessions returns session summaries matching the filter.
	QuerySessions(ctx context.Context, filter SessionFilter) ([]SessionSummary, error)
	// QueryEntries returns trace entries for a session with pagination.
	QueryEntries(ctx context.Context, sessionID string, limit, offset int) ([]IngestEntry, error)
	// SearchTraces searches collected traces by text query.
	SearchTraces(ctx context.Context, query string, limit int) ([]SessionSummary, error)
	// RecordSessionActivity records a session lifecycle event.
	RecordSessionActivity(ctx context.Context, event SessionActivityEvent) error
	// QueryActiveSessions returns sessions that are currently active.
	QueryActiveSessions(ctx context.Context) ([]SessionSummary, error)
	// GetUsageStats returns aggregate collector usage statistics.
	GetUsageStats(ctx context.Context) (*CollectorStats, error)
	// Close shuts down the store and releases resources.
	Close() error
}

// SessionFilter holds query parameters for filtering sessions.
type SessionFilter struct {
	Source      string `json:"source,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	InstanceID  string `json:"instance_id,omitempty"`
	ActiveOnly  bool   `json:"active_only,omitempty"` // Only return sessions with status "active"
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

// SessionSummary is a summary of a collected session.
type SessionSummary struct {
	ID           string    `json:"id"`
	ProjectPath  string    `json:"project_path"`
	Source       string    `json:"source"`
	InstanceID   string    `json:"instance_id"`
	Model        string    `json:"model"`
	EntryCount   int       `json:"entry_count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastUpdated  time.Time `json:"last_updated"`
	Status       string    `json:"status,omitempty"`        // "active", "ended", "unknown"
	LastActivity time.Time `json:"last_activity,omitempty"` // Most recent activity event time
}

// SessionActivityEvent is sent by exporters to report session lifecycle changes.
type SessionActivityEvent struct {
	InstanceID  string    `json:"instance_id"`
	Source      string    `json:"source"`
	ProjectPath string    `json:"project_path"`
	SessionID   string    `json:"session_id"`
	Event       string    `json:"event"` // "session_start", "session_active", "session_end"
	Timestamp   time.Time `json:"timestamp"`
}
