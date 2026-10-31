package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wethinkt/go-thinkt/internal/fingerprint"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tracewire"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Default timeout for considering a session inactive.
const sessionInactiveTimeout = 5 * time.Minute

// Relay watches local session files and ships trace entries to a remote collector.
// It handles batching, buffering, and retry logic.
type Relay struct {
	cfg     RelayConfig
	shipper *Shipper
	buffer  *DiskBuffer
	watcher *FileWatcher

	// Stats tracked atomically
	tracesShipped  atomic.Int64
	tracesFailed   atomic.Int64
	tracesBuffered atomic.Int64
	lastShipTime   atomic.Int64 // unix nano

	// What was last shipped per session, persisted at cfg.StatePath.
	// syncMu serialises syncs, so the state and the buffer order stay
	// consistent.
	state  *relayState
	syncMu sync.Mutex

	// Session activity tracking
	sessionActivity   map[string]time.Time // sessionPath -> lastWriteTime
	sessionActivityMu sync.Mutex

	machineID  string // fingerprint of this machine
	instanceID string // unique ID for this relay instance
	startedAt  time.Time
}

// New creates a new Relay with the given configuration.
func New(cfg RelayConfig) (*Relay, error) {
	cfg.Defaults()
	if cfg.CollectorURL != "" {
		validated, err := ValidateCollectorURL(cfg.CollectorURL)
		if err != nil {
			return nil, err
		}
		cfg.CollectorURL = validated
	}

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

	if cfg.Registry == nil {
		cfg.Registry = thinkt.NewRegistry()
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return nil, err
	}

	machineID, _ := fingerprint.GetFingerprint()
	instanceID := fmt.Sprintf("%s-%d", machineID, os.Getpid())

	return &Relay{
		cfg:             cfg,
		buffer:          buf,
		state:           state,
		sessionActivity: make(map[string]time.Time),
		machineID:       machineID,
		instanceID:      instanceID,
		startedAt:       time.Now(),
	}, nil
}

// Start runs the relay loop until the context is canceled. This method blocks.
func (e *Relay) Start(ctx context.Context) error {
	// Discover collector
	endpoint, err := e.resolveCollector()
	if err != nil {
		return fmt.Errorf("discover collector: %w", err)
	}

	if endpoint.URL != "" {
		e.shipper = NewShipper(endpoint.URL, e.cfg.APIKey)
		if !e.cfg.Quiet {
			tuilog.Log.Info("Relay started", "collector", endpoint.URL, "origin", endpoint.Origin)
		}
		e.registerAgent(ctx)
	} else {
		tuilog.Log.Info("Relay started in buffer-only mode (no collector)")
	}

	// Ship what changed while the relay was off (and, on first run, the
	// history already on disk) before watching.
	if err := e.reconcile(ctx); err != nil {
		tuilog.Log.Warn("Startup reconcile failed", "error", err)
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
	defer watcher.Stop() //nolint:errcheck

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
			_ = e.FlushBuffer(ctx)

		case <-heartbeatTicker.C:
			e.registerAgent(ctx)

		case <-ctx.Done():
			tuilog.Log.Info("Relay shutting down")
			return nil
		}
	}
}

// ExportOnce ships every enabled source's sessions that are new or changed
// since the last run, then drains the buffer.
func (e *Relay) ExportOnce(ctx context.Context) error {
	endpoint, err := e.resolveCollector()
	if err != nil {
		return fmt.Errorf("discover collector: %w", err)
	}

	if endpoint.URL != "" {
		e.shipper = NewShipper(endpoint.URL, e.cfg.APIKey)
	}
	if err := e.reconcile(ctx); err != nil {
		return err
	}
	return e.FlushBuffer(ctx)
}

func (e *Relay) FlushBuffer(ctx context.Context) error {
	if e.shipper == nil {
		return nil
	}

	count, err := e.buffer.Count()
	if err != nil || count == 0 {
		return err
	}

	shipped, err := e.buffer.Drain(ctx, func(payload tracewire.Payload) error {
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

// Stats returns current relay statistics.
func (e *Relay) Stats() RelayStats {
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

	return RelayStats{
		TracesShipped:   e.tracesShipped.Load(),
		TracesFailed:    e.tracesFailed.Load(),
		TracesBuffered:  int64(bufCount),
		BufferSizeBytes: bufSize,
		LastShipTime:    lastShipTime,
		CollectorURL:    collectorURL,
		Watching:        e.cfg.WatchDirs,
	}
}

func (e *Relay) resolveCollector() (*CollectorEndpoint, error) {
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
func (e *Relay) registerAgent(ctx context.Context) {
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

func (e *Relay) handleFileEvent(ctx context.Context, event FileEvent) {
	src := event.Source
	if src == "" {
		src = "unknown"
	}
	fileEventsTotal.WithLabelValues(src).Inc()
	tuilog.Log.Debug("Processing file event", "path", event.Path, "type", event.EventType)

	// Track session activity
	e.recordSessionWrite(ctx, event.Path, event.Source)

	e.syncPath(ctx, event.Path)
}

// recordSessionWrite updates session activity tracking and emits lifecycle events.
func (e *Relay) recordSessionWrite(ctx context.Context, path, source string) {
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
func (e *Relay) sweepInactiveSessions(ctx context.Context) {
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

func (e *Relay) doSweep(ctx context.Context) {
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
func (e *Relay) emitSessionEvent(ctx context.Context, event SessionActivityEvent) {
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

// shipOrBuffer sends a payload, or writes it to the disk buffer when the
// collector is not configured, a ship fails, or older payloads are already
// buffered (so a newer payload never overtakes a buffered older one). It
// reports whether the payload was shipped or buffered.
func (e *Relay) shipOrBuffer(ctx context.Context, payload tracewire.Payload) bool {
	src := payload.Source
	if src == "" {
		src = "unknown"
	}
	n := float64(len(payload.Entries))

	toBuffer := func() bool {
		if err := e.buffer.Write(payload); err != nil {
			tuilog.Log.Error("Failed to buffer payload", "error", err)
			e.tracesFailed.Add(int64(len(payload.Entries)))
			exportEntriesFailed.WithLabelValues(src).Add(n)
			return false
		}
		e.tracesBuffered.Add(int64(len(payload.Entries)))
		exportEntriesBuffered.Add(n)
		return true
	}

	if e.shipper == nil {
		return toBuffer()
	}
	if pending, _ := e.buffer.Count(); pending > 0 {
		ok := toBuffer()
		_ = e.FlushBuffer(ctx)
		return ok
	}

	start := time.Now()
	result, err := e.shipper.Ship(ctx, payload)
	shipDurationSeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		shipRequestsTotal.WithLabelValues("error").Inc()
		exportEntriesFailed.WithLabelValues(src).Add(n)
		tuilog.Log.Warn("Ship failed, buffering", "session_id", payload.Session.ID, "entries", len(payload.Entries), "error", err)
		e.tracesFailed.Add(int64(len(payload.Entries)))
		return toBuffer()
	}

	shipRequestsTotal.WithLabelValues("ok").Inc()
	exportEntriesShipped.WithLabelValues(src).Add(float64(result.Entries))
	e.tracesShipped.Add(int64(result.Entries))
	e.lastShipTime.Store(time.Now().UnixNano())
	return true
}

// lookup finds the session a path belongs to. Some stores cache their
// session list for the life of the process (Kimi, Antigravity), so a nil
// result for an existing file, or a size that disagrees with the file,
// clears that store's cache and looks once more.
func lookup(ctx context.Context, store thinkt.Store, path string) *thinkt.SessionMeta {
	info, statErr := os.Stat(path)
	meta, _ := store.GetSessionMeta(ctx, path)
	stale := statErr == nil && (meta == nil || (meta.FileSize != 0 && meta.FileSize != info.Size()))
	if stale {
		if r, ok := store.(interface{ ResetCache() }); ok {
			r.ResetCache()
			meta, _ = store.GetSessionMeta(ctx, path)
		}
	}
	return meta
}

// syncPath ships the changes of the session file at path. A path no
// enabled source owns is ignored; a path that is no longer a session
// (deleted, or no longer listed by its store) drops its state and ships
// nothing: the archive keeps what it has.
func (e *Relay) syncPath(ctx context.Context, path string) {
	sp, _, ok := sources.ResolveSessionPath(path)
	if !ok {
		return
	}
	store, ok := e.cfg.Registry.Get(sp.Source)
	if !ok {
		return
	}
	meta := lookup(ctx, store, path)

	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	if meta == nil {
		e.pruneLocked(func(st *sessionState) bool { return st.Path == path })
		e.saveStateLocked()
		return
	}
	e.syncSessionLocked(ctx, store, *meta, false)
	e.saveStateLocked()
}

// syncSessionLocked reads one session through its store, diffs it against
// the state, and ships or buffers the payloads. With record set it only
// records what is there (backfill none). Callers hold syncMu.
func (e *Relay) syncSessionLocked(ctx context.Context, store thinkt.Store, meta thinkt.SessionMeta, record bool) {
	id := meta.FullPath
	if id == "" {
		id = meta.ID
	}
	reader, err := store.OpenSession(ctx, id)
	if err != nil {
		tuilog.Log.Debug("Relay: open session failed", "session_id", meta.ID, "error", err)
		return
	}
	var entries []thinkt.Entry
	for {
		entry, err := reader.ReadNext()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			tuilog.Log.Debug("Relay: read session failed", "session_id", meta.ID, "error", err)
			break
		}
		entries = append(entries, *entry)
	}
	reader.Close()
	assignIDs(entries)

	key := stateKey(store.Source(), meta.ID)
	st := e.state.Sessions[key]
	if st == nil {
		st = &sessionState{}
	}
	base := tracewire.Payload{
		Version:    tracewire.Version,
		InstanceID: e.instanceID,
		MachineID:  e.machineID,
		Source:     string(store.Source()),
	}
	for _, p := range diffSession(meta, entries, st, base, e.cfg.BatchSize) {
		if record {
			p.Session.Generation = 0
		} else if !e.shipOrBuffer(ctx, p) {
			return // keep the state as it was; the next sync retries
		}
		st.commit(p)
	}
	if info, err := os.Stat(meta.FullPath); err == nil {
		st.ModTime, st.Size = info.ModTime(), info.Size()
	}
	st.Path = meta.FullPath
	e.state.Sessions[key] = st
}

// reconcile walks every enabled source's sessions, subagents included, and
// syncs those that are new (per the backfill setting) or whose file changed
// since the state recorded it. State for sessions whose file is gone is
// dropped.
func (e *Relay) reconcile(ctx context.Context) error {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	for _, store := range e.cfg.Registry.All() {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			tuilog.Log.Warn("Relay: list projects failed", "source", store.Source(), "error", err)
			continue
		}
		for _, p := range projects {
			sessions, err := store.ListSessions(ctx, p.ID, thinkt.WithSubagents())
			if err != nil {
				continue
			}
			for _, meta := range sessions {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if meta.FullPath == "" {
					continue
				}
				info, err := os.Stat(meta.FullPath)
				if err != nil {
					continue
				}
				st := e.state.Sessions[stateKey(store.Source(), meta.ID)]
				switch {
				case st == nil:
					e.syncSessionLocked(ctx, store, meta, e.cfg.Backfill == BackfillNone)
				case !info.ModTime().Equal(st.ModTime) || info.Size() != st.Size:
					e.syncSessionLocked(ctx, store, meta, false)
				}
			}
		}
	}
	e.pruneLocked(func(st *sessionState) bool {
		_, err := os.Stat(st.Path)
		return st.Path == "" || os.IsNotExist(err)
	})
	e.saveStateLocked()
	return nil
}

// pruneLocked drops state entries gone reports true for.
func (e *Relay) pruneLocked(gone func(*sessionState) bool) {
	for k, st := range e.state.Sessions {
		if gone(st) {
			delete(e.state.Sessions, k)
		}
	}
}

func (e *Relay) saveStateLocked() {
	if e.cfg.StatePath == "" {
		return
	}
	if err := e.state.save(e.cfg.StatePath); err != nil {
		tuilog.Log.Warn("Relay: save state failed", "error", err)
	}
}
