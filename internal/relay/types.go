// Package relay provides a trace exporter that watches local AI session files
// and ships them to a remote collector endpoint via HTTP POST.
package relay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/urlutil"
)

// WatchDir pairs a directory path with its source identity and watch configuration.
type WatchDir struct {
	Path   string             // Absolute path to the source's base directory
	Source string             // Source name (e.g. "claude", "kimi")
	Config thinkt.WatchConfig // Per-source watch configuration
}

// RelayConfig holds configuration for the trace relay.
type RelayConfig struct {
	// CollectorURL is the endpoint to POST traces to (e.g. "https://collect.wethinkt.com/v1/traces").
	// If empty, discovery will be used.
	CollectorURL string

	// APIKey is the Bearer token for collector authentication.
	APIKey string

	// BufferDir is the local disk buffer directory for relay when the collector is unreachable.
	// Defaults to ~/.thinkt/relay-buffer/.
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

	// Version is the relay version string (set via ldflags).
	Version string

	// Registry holds the stores of the enabled sources; only they ship.
	Registry *thinkt.StoreRegistry

	// Backfill is "all" (ship every session already on disk when the relay
	// first sees it) or "none" (record what is there and ship only later
	// changes). Default "all"; THINKT_RELAY_BACKFILL.
	Backfill string

	// StatePath is the relay's per-session state file. Default
	// <thinkt home>/relay/state.json.
	StatePath string
}

// Backfill modes.
const (
	BackfillAll  = "all"
	BackfillNone = "none"
)

// ResolveBackfill returns the --backfill flag value, else
// THINKT_RELAY_BACKFILL, else all.
func ResolveBackfill(flag string) (string, error) {
	if flag == "" {
		flag = os.Getenv("THINKT_RELAY_BACKFILL")
	}
	switch flag {
	case "", BackfillAll:
		return BackfillAll, nil
	case BackfillNone:
		return BackfillNone, nil
	}
	return "", fmt.Errorf("invalid backfill %q (want all or none)", flag)
}

// Defaults applies default values to unset config fields.
func (c *RelayConfig) Defaults() {
	if c.Backfill == "" {
		c.Backfill = BackfillAll
	}
	if c.StatePath == "" {
		if dir, err := config.Dir(); err == nil {
			c.StatePath = filepath.Join(dir, "relay", "state.json")
		}
	}
	if c.BufferDir == "" {
		c.BufferDir = "~/.thinkt/relay-buffer/"
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
	c.CollectorURL = NormalizeCollectorURL(c.CollectorURL)
}

// ValidateCollectorURL normalizes and validates a collector endpoint URL.
func ValidateCollectorURL(u string) (string, error) {
	if u == "" {
		return "", nil
	}
	u = NormalizeCollectorURL(u)
	validated, err := urlutil.ValidateEndpointURLAllowHTTP(u)
	if err != nil {
		return "", fmt.Errorf("invalid collector url: %w", err)
	}
	return validated, nil
}

// NormalizeCollectorURL ensures a collector URL ends with the /v1/traces path.
// A bare base URL like "http://localhost:4318" becomes "http://localhost:4318/v1/traces".
// URLs that already contain the path are returned unchanged.
func NormalizeCollectorURL(u string) string {
	if u == "" {
		return u
	}
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/v1/traces") {
		return u
	}
	// Handle partial paths like /v1 without /traces
	if strings.HasSuffix(u, "/v1") {
		return u + "/traces"
	}
	return u + "/v1/traces"
}

// ShipResult tracks the result of shipping a batch.
type ShipResult struct {
	BatchID    string
	Entries    int
	StatusCode int
	Error      error
	Duration   time.Duration
}

// RelayStats reports current relay state.
type RelayStats struct {
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
	MachineID   string    `json:"machine_id,omitempty"`
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
