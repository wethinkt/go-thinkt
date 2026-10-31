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

	"github.com/wethinkt/go-thinkt/internal/config"
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
	state     *relayState
	syncMu    sync.Mutex
	dirty     bool      // state changed since the last save (syncMu)
	lastSaved time.Time // syncMu

	// offline is set when a ship fails and cleared once the buffer fully
	// drains. While set, syncs buffer without contacting the collector, so
	// an outage costs one retry cycle, not one per payload; the drain
	// ticker keeps trying.
	offline atomic.Bool

	// Session activity tracking
	sessionActivity   map[string]activity // session path → last write and identity
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
	if cfg.StatePath != "" {
		buf.quarantine = filepath.Join(filepath.Dir(cfg.StatePath), "quarantine")
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
		sessionActivity: make(map[string]activity),
		machineID:       machineID,
		instanceID:      instanceID,
		startedAt:       time.Now(),
	}, nil
}

// activity is a session the relay has seen written to recently.
type activity struct {
	last        time.Time
	source      string
	sessionID   string
	projectPath string
}

// claim registers this relay as the machine's relay instance. Two relays on
// one machine share a machine id and state file, so a second one refuses.
func (e *Relay) claim() (release func(), err error) {
	if inst := config.FindInstanceByType(config.InstanceRelay); inst != nil && inst.PID != os.Getpid() {
		return nil, fmt.Errorf("relay already running (pid %d)", inst.PID)
	}
	if err := config.RegisterInstance(config.Instance{Type: config.InstanceRelay, PID: os.Getpid(), StartedAt: e.startedAt}); err != nil {
		tuilog.Log.Warn("Relay: register instance failed", "error", err)
	}
	return func() { _ = config.UnregisterInstance(os.Getpid()) }, nil
}

// discardDisabled drops buffered payloads of sources not in the registry
// (turned off since they were buffered).
//
// "Turned off" is judged by the config, not by this run's registry: a
// --source filter narrows what ships now, and must not drop other enabled
// sources' payloads. A dropped payload's session loses its state too, so
// turning the source back on re-ships it.
func (e *Relay) discardDisabled() {
	cfg, _ := config.Load()
	allowed, constrained := sources.ConfiguredEnabled(cfg)
	enabled := map[string]bool{}
	for _, s := range allowed {
		enabled[s] = true
	}
	var dropped []string
	n, err := e.buffer.Discard(func(p tracewire.Payload) bool {
		if !constrained || enabled[p.Source] {
			return true
		}
		dropped = append(dropped, stateKey(thinkt.Source(p.Source), p.Session.ID))
		return false
	})
	if len(dropped) > 0 {
		e.syncMu.Lock()
		for _, k := range dropped {
			delete(e.state.Sessions, k)
		}
		e.dirty = true
		e.saveStateLocked()
		e.syncMu.Unlock()
	}
	if err != nil {
		tuilog.Log.Warn("Relay: discard buffered payloads failed", "error", err)
	} else if n > 0 {
		tuilog.Log.Info("Relay: discarded buffered payloads of disabled sources", "count", n)
	}
}

// Start runs the relay loop until the context is canceled. This method blocks.
func (e *Relay) Start(ctx context.Context) error {
	release, err := e.claim()
	if err != nil {
		return err
	}
	defer release()
	e.discardDisabled()

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

	// Start file watcher before the reconcile, so a session written while
	// the reconcile runs queues an event instead of being missed.
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

	// Ship what changed while the relay was off (and, on first run, the
	// history already on disk). Events queue in the watcher meanwhile.
	if err := e.reconcile(ctx); err != nil {
		tuilog.Log.Warn("Startup reconcile failed", "error", err)
	}

	// Start session activity sweep goroutine
	go e.sweepInactiveSessions(ctx)

	// Periodic buffer drain
	drainTicker := time.NewTicker(e.cfg.FlushInterval)
	defer drainTicker.Stop()
	defer watcher.Stop() //nolint:errcheck
	defer e.saveIfDirty()
	saveTicker := time.NewTicker(stateSaveInterval)
	defer saveTicker.Stop()

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

		case <-saveTicker.C:
			e.saveIfDirty()

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
	release, err := e.claim()
	if err != nil {
		return err
	}
	defer release()
	e.discardDisabled()

	endpoint, err := e.resolveCollector()
	if err != nil {
		return fmt.Errorf("discover collector: %w", err)
	}

	if endpoint.URL != "" && e.shipper == nil {
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
	if left, _ := e.buffer.Count(); left == 0 {
		e.offline.Store(false)
	} else {
		e.offline.Store(true)
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
	a, existed := e.sessionActivity[path]
	e.sessionActivityMu.Unlock()
	if !existed {
		a = e.identify(ctx, path, source)
	}
	a.last = now
	e.sessionActivityMu.Lock()
	e.sessionActivity[path] = a
	e.sessionActivityMu.Unlock()

	event := "session_active"
	if !existed {
		event = "session_start"
	}
	e.emitSessionEvent(ctx, SessionActivityEvent{
		InstanceID:  e.instanceID,
		MachineID:   e.machineID,
		Source:      a.source,
		ProjectPath: a.projectPath,
		SessionID:   a.sessionID,
		Event:       event,
		Timestamp:   now,
	})
}

// identify names the session a changed file belongs to, the way its store
// does, falling back to the file name when no enabled store claims it.
func (e *Relay) identify(ctx context.Context, path, source string) activity {
	a := activity{source: source, sessionID: sessionIDFromPath(path)}
	if sp, _, ok := sources.ResolveSessionPath(path); ok {
		a.source = string(sp.Source)
		if store, ok := e.cfg.Registry.Get(sp.Source); ok {
			if meta := lookup(ctx, store, path); meta != nil {
				a.sessionID, a.projectPath = meta.ID, meta.ProjectPath
			}
		}
	}
	return a
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

func (e *Relay) doSweep(ctx context.Context) { e.doSweepAt(ctx, time.Now()) }

// doSweepAt ends the sessions not written to within sessionInactiveTimeout
// of now.
func (e *Relay) doSweepAt(ctx context.Context, now time.Time) {
	cutoff := now.Add(-sessionInactiveTimeout)

	e.sessionActivityMu.Lock()
	var ended []activity
	for path, a := range e.sessionActivity {
		if a.last.Before(cutoff) {
			ended = append(ended, a)
			delete(e.sessionActivity, path)
		}
	}
	e.sessionActivityMu.Unlock()

	for _, a := range ended {
		e.emitSessionEvent(ctx, SessionActivityEvent{
			InstanceID:  e.instanceID,
			MachineID:   e.machineID,
			Source:      a.source,
			ProjectPath: a.projectPath,
			SessionID:   a.sessionID,
			Event:       "session_end",
			Timestamp:   now,
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

	if e.shipper == nil || e.offline.Load() {
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
	var rejected *RejectedError
	if errors.As(err, &rejected) {
		// Resending cannot succeed; keep it on disk and move on, so it
		// neither blocks the buffer nor loops on every sync.
		shipRequestsTotal.WithLabelValues("error").Inc()
		exportEntriesFailed.WithLabelValues(src).Add(n)
		e.buffer.Quarantine(payload, err)
		return true
	}
	if err != nil {
		e.offline.Store(true)
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
		return
	}
	e.syncSessionLocked(ctx, store, *meta, false)
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
	for _, p := range diffSession(meta, entries, st, base, e.cfg.BatchSize, nextGeneration(st, time.Now())) {
		if record {
			p.Session.Generation = 0
		} else if !e.shipOrBuffer(ctx, p) {
			return // keep the state as it was; the next sync retries
		}
		st.commit(p)
	}
	if info, err := os.Stat(meta.FullPath); err == nil {
		st.ModTime, st.Size = info.ModTime(), info.Size()
		st.ParserVersion = thinkt.ParserVersion
	}
	st.Path = meta.FullPath
	e.state.Sessions[key] = st
	e.dirty = true
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
					e.saveStateLocked() // keep the progress made so far
					return ctx.Err()
				}
				if time.Since(e.lastSaved) > stateSaveInterval {
					e.saveStateLocked()
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
				case !info.ModTime().Equal(st.ModTime) || info.Size() != st.Size,
					st.ParserVersion != thinkt.ParserVersion:
					e.syncSessionLocked(ctx, store, meta, false)
				}
			}
		}
	}
	e.pruneLocked(func(st *sessionState) bool {
		_, err := os.Stat(st.Path)
		return st.Path == "" || os.IsNotExist(err)
	})
	e.dirty = true // a reconcile always leaves the state on disk
	e.saveStateLocked()
	return nil
}

// stateSaveInterval bounds how often the state file is rewritten while
// the relay runs; it is also saved at the end of a reconcile and at stop.
const stateSaveInterval = 30 * time.Second

// pruneLocked drops state entries gone reports true for.
func (e *Relay) pruneLocked(gone func(*sessionState) bool) {
	for k, st := range e.state.Sessions {
		if gone(st) {
			delete(e.state.Sessions, k)
			e.dirty = true
		}
	}
}

// saveStateLocked writes the state if it changed. Callers hold syncMu.
func (e *Relay) saveStateLocked() {
	if e.cfg.StatePath == "" || !e.dirty {
		return
	}
	if err := e.state.save(e.cfg.StatePath); err != nil {
		tuilog.Log.Warn("Relay: save state failed", "error", err)
		return
	}
	e.dirty = false
	e.lastSaved = time.Now()
}

// quarantineDir holds payloads the collector permanently rejected.
func (e *Relay) quarantineDir() string { return e.buffer.quarantine }

// saveIfDirty saves the state if a sync changed it since the last save.
func (e *Relay) saveIfDirty() {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	e.saveStateLocked()
}
