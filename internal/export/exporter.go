package export

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wethinkt/go-thinkt/internal/fingerprint"
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Default timeout for considering a session inactive.
const sessionInactiveTimeout = 5 * time.Minute

// Exporter watches local session files and ships trace entries to a remote collector.
// It handles batching, buffering, and retry logic.
type Exporter struct {
	cfg     ExporterConfig
	shipper *Shipper
	buffer  *DiskBuffer
	watcher *FileWatcher

	// Stats tracked atomically
	tracesShipped  atomic.Int64
	tracesFailed   atomic.Int64
	tracesBuffered atomic.Int64
	lastShipTime   atomic.Int64 // unix nano

	// Tracks file read offsets so we only ship new entries
	offsets   map[string]int64
	offsetsMu sync.Mutex

	// Session activity tracking
	sessionActivity   map[string]time.Time // sessionPath -> lastWriteTime
	sessionActivityMu sync.Mutex

	machineID  string // fingerprint of this machine
	instanceID string // unique ID for this exporter instance
	startedAt  time.Time
}

// New creates a new Exporter with the given configuration.
func New(cfg ExporterConfig) (*Exporter, error) {
	cfg.Defaults()

	bufDir := cfg.BufferDir
	if len(bufDir) > 0 && bufDir[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		bufDir = filepath.Join(home, bufDir[1:])
	}

	buf, err := NewDiskBuffer(bufDir, cfg.MaxBufferMB)
	if err != nil {
		return nil, fmt.Errorf("create disk buffer: %w", err)
	}

	machineID, _ := fingerprint.GetFingerprint()
	instanceID := fmt.Sprintf("%s-%d", machineID, os.Getpid())

	return &Exporter{
		cfg:             cfg,
		buffer:          buf,
		offsets:         make(map[string]int64),
		sessionActivity: make(map[string]time.Time),
		machineID:       machineID,
		instanceID:      instanceID,
		startedAt:       time.Now(),
	}, nil
}

// Start runs the exporter loop until the context is canceled. This method blocks.
func (e *Exporter) Start(ctx context.Context) error {
	// Discover collector
	endpoint, err := e.resolveCollector()
	if err != nil {
		return fmt.Errorf("discover collector: %w", err)
	}

	if endpoint.URL != "" {
		e.shipper = NewShipper(endpoint.URL, e.cfg.APIKey)
		if !e.cfg.Quiet {
			tuilog.Log.Info("Exporter started", "collector", endpoint.URL, "origin", endpoint.Origin)
		}
		e.registerAgent(ctx)
	} else {
		tuilog.Log.Info("Exporter started in buffer-only mode (no collector)")
	}

	// Start file watcher
	watcher, err := NewFileWatcher(e.cfg.WatchDirs)
	if err != nil {
		return fmt.Errorf("create file watcher: %w", err)
	}
	e.watcher = watcher

	events, err := watcher.Start(ctx)
	if err != nil {
		return fmt.Errorf("start file watcher: %w", err)
	}
	watchedDirs.Set(float64(len(e.cfg.WatchDirs)))

	// Log watched directories
	for _, wd := range e.cfg.WatchDirs {
		tuilog.Log.Info("Watching", "source", wd.Source, "dir", wd.Path)
	}

	// Start session activity sweep goroutine
	go e.sweepInactiveSessions(ctx)

	// Periodic buffer drain
	drainTicker := time.NewTicker(e.cfg.FlushInterval)
	defer drainTicker.Stop()
	defer watcher.Stop()

	// Periodic agent heartbeat (re-registers to keep metadata alive)
	heartbeatTicker := time.NewTicker(2 * time.Minute)
	defer heartbeatTicker.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil
			}
			e.handleFileEvent(ctx, event)

		case <-drainTicker.C:
			e.FlushBuffer(ctx)

		case <-heartbeatTicker.C:
			e.registerAgent(ctx)

		case <-ctx.Done():
			tuilog.Log.Info("Exporter shutting down")
			return nil
		}
	}
}

// ExportOnce performs a one-shot export of all session files in the watch directories.
func (e *Exporter) ExportOnce(ctx context.Context) error {
	endpoint, err := e.resolveCollector()
	if err != nil {
		return fmt.Errorf("discover collector: %w", err)
	}

	if endpoint.URL != "" {
		e.shipper = NewShipper(endpoint.URL, e.cfg.APIKey)
	}

	for _, wd := range e.cfg.WatchDirs {
		entries, err := os.ReadDir(wd.Path)
		if err != nil {
			tuilog.Log.Warn("Failed to read watch dir", "dir", wd.Path, "error", err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}

			path := filepath.Join(wd.Path, entry.Name())
			e.processFile(ctx, path, wd.Source)
		}
	}

	return e.FlushBuffer(ctx)
}

// FlushBuffer attempts to drain the disk buffer by shipping buffered payloads.
func (e *Exporter) FlushBuffer(ctx context.Context) error {
	if e.shipper == nil {
		return nil
	}

	count, err := e.buffer.Count()
	if err != nil || count == 0 {
		return err
	}

	shipped, err := e.buffer.Drain(ctx, func(payload TracePayload) error {
		_, shipErr := e.shipper.Ship(ctx, payload)
		return shipErr
	})

	if shipped > 0 {
		e.tracesBuffered.Add(-int64(shipped))
		bufferDrainedTotal.Add(float64(shipped))
		tuilog.Log.Info("Drained buffer", "shipped", shipped)
	}
	return err
}

// Stats returns current exporter statistics.
func (e *Exporter) Stats() ExporterStats {
	bufSize, _ := e.buffer.Size()
	bufCount, _ := e.buffer.Count()
	bufferSizeBytes.Set(float64(bufSize))

	var collectorURL string
	if e.shipper != nil {
		collectorURL = e.shipper.collectorURL
	}

	lastShip := e.lastShipTime.Load()
	var lastShipTime time.Time
	if lastShip > 0 {
		lastShipTime = time.Unix(0, lastShip)
	}

	return ExporterStats{
		TracesShipped:   e.tracesShipped.Load(),
		TracesFailed:    e.tracesFailed.Load(),
		TracesBuffered:  int64(bufCount),
		BufferSizeBytes: bufSize,
		LastShipTime:    lastShipTime,
		CollectorURL:    collectorURL,
		Watching:        e.cfg.WatchDirs,
	}
}

func (e *Exporter) resolveCollector() (*CollectorEndpoint, error) {
	if e.cfg.CollectorURL != "" {
		return &CollectorEndpoint{URL: e.cfg.CollectorURL, Origin: "config"}, nil
	}
	// Use first watch dir as project path hint for discovery
	projectPath := ""
	if len(e.cfg.WatchDirs) > 0 {
		projectPath = e.cfg.WatchDirs[0].Path
	}
	return DiscoverCollector(projectPath)
}

// registerAgent sends an agent registration to the collector.
func (e *Exporter) registerAgent(ctx context.Context) {
	hostname, _ := os.Hostname()
	reg := AgentRegistration{
		InstanceID: e.instanceID,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Hostname:   hostname,
		Version:    e.cfg.Version,
		MachineID:  e.machineID,
		StartedAt:  e.startedAt,
	}
	if err := e.shipper.RegisterAgent(ctx, reg); err != nil {
		tuilog.Log.Warn("Failed to register agent", "error", err)
	} else {
		tuilog.Log.Info("Registered with collector", "instance_id", e.instanceID)
	}
}

func (e *Exporter) handleFileEvent(ctx context.Context, event FileEvent) {
	src := event.Source
	if src == "" {
		src = "unknown"
	}
	fileEventsTotal.WithLabelValues(src).Inc()
	tuilog.Log.Debug("Processing file event", "path", event.Path, "type", event.EventType)

	// Track session activity
	e.recordSessionWrite(ctx, event.Path, event.Source)

	e.processFile(ctx, event.Path, event.Source)
}

// recordSessionWrite updates session activity tracking and emits lifecycle events.
func (e *Exporter) recordSessionWrite(ctx context.Context, path, source string) {
	now := time.Now()

	e.sessionActivityMu.Lock()
	_, existed := e.sessionActivity[path]
	e.sessionActivity[path] = now
	e.sessionActivityMu.Unlock()

	sessionID := sessionIDFromPath(path)

	if !existed {
		// New session detected
		e.emitSessionEvent(ctx, SessionActivityEvent{
			InstanceID: e.instanceID,
			Source:     source,
			SessionID:  sessionID,
			Event:      "session_start",
			Timestamp:  now,
		})
	} else {
		e.emitSessionEvent(ctx, SessionActivityEvent{
			InstanceID: e.instanceID,
			Source:     source,
			SessionID:  sessionID,
			Event:      "session_active",
			Timestamp:  now,
		})
	}
}

// sweepInactiveSessions periodically checks for sessions that haven't been
// written to recently and emits session_end events.
func (e *Exporter) sweepInactiveSessions(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.doSweep(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (e *Exporter) doSweep(ctx context.Context) {
	now := time.Now()
	cutoff := now.Add(-sessionInactiveTimeout)

	e.sessionActivityMu.Lock()
	var ended []string
	for path, lastWrite := range e.sessionActivity {
		if lastWrite.Before(cutoff) {
			ended = append(ended, path)
		}
	}
	for _, path := range ended {
		delete(e.sessionActivity, path)
	}
	e.sessionActivityMu.Unlock()

	for _, path := range ended {
		sessionID := sessionIDFromPath(path)
		source := detectSource(path)
		e.emitSessionEvent(ctx, SessionActivityEvent{
			InstanceID: e.instanceID,
			Source:     source,
			SessionID:  sessionID,
			Event:      "session_end",
			Timestamp:  now,
		})
	}
}

// emitSessionEvent sends a session activity event to the collector.
func (e *Exporter) emitSessionEvent(ctx context.Context, event SessionActivityEvent) {
	if e.shipper == nil {
		return
	}
	if err := e.shipper.ShipSessionActivity(ctx, event); err != nil {
		tuilog.Log.Debug("Failed to ship session activity", "event", event.Event, "error", err)
	}
}

// sessionIDFromPath extracts the session ID from a file path.
func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		return base[:len(base)-len(ext)]
	}
	return base
}

// processFile reads new entries from a JSONL file and ships them in batches.
func (e *Exporter) processFile(ctx context.Context, path, source string) {
	e.offsetsMu.Lock()
	offset := e.offsets[path]
	e.offsetsMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		tuilog.Log.Warn("Failed to open session file", "path", path, "error", err)
		return
	}
	defer f.Close()

	// Seek to last known position
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			tuilog.Log.Warn("Failed to seek in session file", "path", path, "error", err)
			return
		}
	}

	// Read new entries
	var entries []TraceEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		entry, ok := parseRawEntry(scanner.Bytes())
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	// Update offset
	newOffset, _ := f.Seek(0, 1) // current position
	e.offsetsMu.Lock()
	e.offsets[path] = newOffset
	e.offsetsMu.Unlock()

	if len(entries) == 0 {
		return
	}

	// Derive session ID from filename
	sessionID := filepath.Base(path)
	ext := filepath.Ext(sessionID)
	if ext != "" {
		sessionID = sessionID[:len(sessionID)-len(ext)]
	}

	// Extract project path from directory structure.
	// Claude stores sessions in ~/.claude/projects/<encoded-path>/<session>.jsonl
	projectPath := extractProjectPath(path)

	// Batch and ship
	for i := 0; i < len(entries); i += e.cfg.BatchSize {
		end := i + e.cfg.BatchSize
		if end > len(entries) {
			end = len(entries)
		}

		payload := TracePayload{
			InstanceID:  e.instanceID,
			Source:      source,
			SessionID:   sessionID,
			ProjectPath: projectPath,
			Entries:     entries[i:end],
			MachineID:   e.machineID,
		}

		e.shipOrBuffer(ctx, payload)
	}
}

// extractProjectPath derives a project path from a session file path.
// For Claude, sessions are stored in ~/.claude/projects/<encoded-path>/<session>.jsonl
// where <encoded-path> uses dash-encoded paths (e.g., -Users-evan-my-project).
func extractProjectPath(filePath string) string {
	dir := filepath.Dir(filePath)
	parent := filepath.Base(filepath.Dir(dir)) // e.g., "projects"
	if parent == "projects" {
		encoded := filepath.Base(dir)
		_, fullPath := claude.DecodeDirName(encoded)
		return fullPath
	}
	return ""
}

// shipOrBuffer tries to ship a payload; on failure, buffers it to disk.
func (e *Exporter) shipOrBuffer(ctx context.Context, payload TracePayload) {
	src := payload.Source
	if src == "" {
		src = "unknown"
	}
	n := float64(len(payload.Entries))

	if e.shipper == nil {
		// No collector configured, buffer only
		if err := e.buffer.Write(payload); err != nil {
			tuilog.Log.Error("Failed to buffer payload", "error", err)
			e.tracesFailed.Add(int64(len(payload.Entries)))
			exportEntriesFailed.WithLabelValues(src).Add(n)
		} else {
			e.tracesBuffered.Add(int64(len(payload.Entries)))
			exportEntriesBuffered.Add(n)
		}
		return
	}

	start := time.Now()
	result, err := e.shipper.Ship(ctx, payload)
	shipDurationSeconds.Observe(time.Since(start).Seconds())

	if err != nil {
		shipRequestsTotal.WithLabelValues("error").Inc()
		exportEntriesFailed.WithLabelValues(src).Add(n)
		tuilog.Log.Warn("Ship failed, buffering",
			"entries", len(payload.Entries),
			"error", err,
		)
		e.tracesFailed.Add(int64(len(payload.Entries)))
		if bufErr := e.buffer.Write(payload); bufErr != nil {
			tuilog.Log.Error("Failed to buffer payload", "error", bufErr)
		} else {
			e.tracesBuffered.Add(int64(len(payload.Entries)))
			exportEntriesBuffered.Add(n)
		}
		return
	}

	shipRequestsTotal.WithLabelValues("ok").Inc()
	exportEntriesShipped.WithLabelValues(src).Add(float64(result.Entries))
	e.tracesShipped.Add(int64(result.Entries))
	e.lastShipTime.Store(time.Now().UnixNano())
}

// rawEntry is a minimal struct for parsing JSONL lines from any source.
// Different sources use different field names: Claude uses "type" for role,
// Kimi uses "role", etc. This struct captures both so we can normalize.
type rawEntry struct {
	UUID      string          `json:"uuid"`
	Type      string          `json:"type"`                // Claude: role is in "type"
	Role      string          `json:"role"`                // Kimi/others: role is in "role"
	Timestamp time.Time       `json:"timestamp"`
	Model     string          `json:"model,omitempty"`
	Text      string          `json:"text,omitempty"`
	AgentID   string          `json:"agentId,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
}

// validRoles are the roles accepted by the collector.
var validRoles = map[string]bool{
	"user": true, "assistant": true, "tool_use": true, "tool_result": true, "system": true,
}

// parseRawEntry parses a JSONL line into a TraceEntry, handling source-specific
// field naming differences.
func parseRawEntry(line []byte) (TraceEntry, bool) {
	var raw rawEntry
	if err := json.Unmarshal(line, &raw); err != nil {
		return TraceEntry{}, false
	}

	if raw.UUID == "" {
		return TraceEntry{}, false
	}

	// Resolve role: prefer "role" field, fall back to "type" field
	role := raw.Role
	if role == "" {
		role = raw.Type
	}
	// Map source-specific types to collector roles
	switch role {
	case "human":
		role = "user"
	}
	if !validRoles[role] {
		return TraceEntry{}, false
	}

	text := raw.Text

	// Try to extract text, model, tokens from assistant message content
	var info messageInfo
	if raw.Message != nil {
		info = extractFromMessage(raw.Message, text)
		text = info.Text
	}
	model := info.Model
	if raw.Model != "" {
		model = raw.Model
	}

	return TraceEntry{
		UUID:         raw.UUID,
		Role:         role,
		Timestamp:    raw.Timestamp,
		Text:         text,
		Model:        model,
		ToolName:     info.ToolName,
		AgentID:      raw.AgentID,
		InputTokens:  info.InputTokens,
		OutputTokens: info.OutputTokens,
	}, true
}

// messageInfo holds extracted fields from a message JSON blob.
type messageInfo struct {
	Text         string
	Model        string
	ToolName     string
	InputTokens  int
	OutputTokens int
}

// extractFromMessage extracts text, model, tool name, and token usage from a message JSON blob.
func extractFromMessage(msg json.RawMessage, fallbackText string) messageInfo {
	info := messageInfo{Text: fallbackText}

	var parsed struct {
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(msg, &parsed); err != nil {
		return info
	}
	info.Model = parsed.Model
	if parsed.Usage != nil {
		info.InputTokens = parsed.Usage.InputTokens
		info.OutputTokens = parsed.Usage.OutputTokens
	}

	if parsed.Content == nil {
		return info
	}

	// Content can be a string or an array of blocks
	var contentStr string
	if err := json.Unmarshal(parsed.Content, &contentStr); err == nil {
		if info.Text == "" {
			info.Text = contentStr
		}
		return info
	}

	// Parse as array of content blocks
	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Name     string `json:"name"`
		Thinking string `json:"thinking"`
	}
	if err := json.Unmarshal(parsed.Content, &blocks); err != nil {
		return info
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if info.Text == "" {
				info.Text = b.Text
			}
		case "tool_use":
			if info.ToolName == "" {
				info.ToolName = b.Name
			}
		}
	}
	return info
}

