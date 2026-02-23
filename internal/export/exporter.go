package export

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
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

	return &Exporter{
		cfg:             cfg,
		buffer:          buf,
		offsets:         make(map[string]int64),
		sessionActivity: make(map[string]time.Time),
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

	// Start session activity sweep goroutine
	go e.sweepInactiveSessions(ctx)

	// Periodic buffer drain
	drainTicker := time.NewTicker(e.cfg.FlushInterval)
	defer drainTicker.Stop()
	defer watcher.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil
			}
			e.handleFileEvent(ctx, event)

		case <-drainTicker.C:
			e.FlushBuffer(ctx)

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

	for _, dir := range e.cfg.WatchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			tuilog.Log.Warn("Failed to read watch dir", "dir", dir, "error", err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			e.processFile(ctx, path, detectSource(path))
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
		tuilog.Log.Info("Drained buffer", "shipped", shipped)
	}
	return err
}

// Stats returns current exporter statistics.
func (e *Exporter) Stats() ExporterStats {
	bufSize, _ := e.buffer.Size()
	bufCount, _ := e.buffer.Count()

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
		projectPath = e.cfg.WatchDirs[0]
	}
	return DiscoverCollector(projectPath)
}

func (e *Exporter) handleFileEvent(ctx context.Context, event FileEvent) {
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
			Source:    source,
			SessionID: sessionID,
			Event:     "session_start",
			Timestamp: now,
		})
	} else {
		e.emitSessionEvent(ctx, SessionActivityEvent{
			Source:    source,
			SessionID: sessionID,
			Event:     "session_active",
			Timestamp: now,
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
			Source:    source,
			SessionID: sessionID,
			Event:     "session_end",
			Timestamp: now,
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
		var raw thinkt.Entry
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue // Skip malformed lines
		}

		text := raw.Text
		if text == "" && len(raw.ContentBlocks) > 0 {
			for _, b := range raw.ContentBlocks {
				if b.Text != "" {
					text = b.Text
					break
				}
			}
		}

		entries = append(entries, TraceEntry{
			UUID:      raw.UUID,
			Role:      string(raw.Role),
			Timestamp: raw.Timestamp,
			Text:      text,
			Model:     raw.Model,
			ToolName:  firstToolName(raw.ContentBlocks),
			AgentID:   raw.AgentID,
		})
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

	// Batch and ship
	for i := 0; i < len(entries); i += e.cfg.BatchSize {
		end := i + e.cfg.BatchSize
		if end > len(entries) {
			end = len(entries)
		}

		payload := TracePayload{
			Source:    source,
			SessionID: sessionID,
			Entries:   entries[i:end],
		}

		e.shipOrBuffer(ctx, payload)
	}
}

// shipOrBuffer tries to ship a payload; on failure, buffers it to disk.
func (e *Exporter) shipOrBuffer(ctx context.Context, payload TracePayload) {
	if e.shipper == nil {
		// No collector configured, buffer only
		if err := e.buffer.Write(payload); err != nil {
			tuilog.Log.Error("Failed to buffer payload", "error", err)
			e.tracesFailed.Add(int64(len(payload.Entries)))
		} else {
			e.tracesBuffered.Add(int64(len(payload.Entries)))
		}
		return
	}

	result, err := e.shipper.Ship(ctx, payload)
	if err != nil {
		tuilog.Log.Warn("Ship failed, buffering",
			"entries", len(payload.Entries),
			"error", err,
		)
		e.tracesFailed.Add(int64(len(payload.Entries)))
		if bufErr := e.buffer.Write(payload); bufErr != nil {
			tuilog.Log.Error("Failed to buffer payload", "error", bufErr)
		} else {
			e.tracesBuffered.Add(int64(len(payload.Entries)))
		}
		return
	}

	e.tracesShipped.Add(int64(result.Entries))
	e.lastShipTime.Store(time.Now().UnixNano())
}

// firstToolName extracts the first tool_use name from content blocks, if any.
func firstToolName(blocks []thinkt.ContentBlock) string {
	for _, b := range blocks {
		if b.ToolName != "" {
			return b.ToolName
		}
	}
	return ""
}
