package index

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// sessionIndexEntry holds the metadata needed to re-index a session.
type sessionIndexEntry struct {
	session   thinkt.SessionMeta
	projectID string
	source    thinkt.Source
}

// Watcher monitors session directories for changes and triggers ingestion.
type Watcher struct {
	database       *db.DB
	registry       *thinkt.StoreRegistry
	debounce       time.Duration
	rescanInterval time.Duration
	watcher        *fsnotify.Watcher
	done           chan struct{}
	mu             sync.Mutex
	sessionIndex   map[string]sessionIndexEntry // normalized path -> session info

	inFlightMu sync.Mutex
	inFlight   map[string]bool
	dirty      map[string]bool

	// OnSessionIndexed is called after a session is successfully re-indexed.
	OnSessionIndexed func(sessionID string)
}

// NewWatcher creates a new Watcher. A zero debounce defaults to 2 seconds.
func NewWatcher(database *db.DB, registry *thinkt.StoreRegistry, debounce time.Duration) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		database:     database,
		registry:     registry,
		watcher:      fw,
		done:         make(chan struct{}),
		sessionIndex: make(map[string]sessionIndexEntry),
		inFlight:     make(map[string]bool),
		dirty:        make(map[string]bool),
	}
	w.setDebounce(debounce)

	return w, nil
}

func (w *Watcher) setDebounce(d time.Duration) {
	if d <= 0 {
		d = 2 * time.Second
	}
	w.debounce = d
}

// SetRescanInterval configures periodic project rescans. A value ≤0 disables rescanning.
func (w *Watcher) SetRescanInterval(d time.Duration) {
	w.rescanInterval = d
}

// Start begins monitoring projects for changes.
func (w *Watcher) Start(ctx context.Context) error {
	projects, err := w.registry.ListAllProjects(ctx)
	if err != nil {
		return err
	}

	for _, p := range projects {
		if err := w.watchProject(p); err != nil {
			tuilog.Log.Warn("watcher: failed to watch project", "project", p.Name, "error", err)
		}
	}

	go w.watchLoop(ctx)
	if w.rescanInterval > 0 {
		go w.rescanLoop(ctx)
	}
	return nil
}

// rescanLoop periodically re-lists projects so brand-new project directories are picked up.
// watchProject is idempotent, so re-calling it for already-watched projects is safe.
func (w *Watcher) rescanLoop(ctx context.Context) {
	t := time.NewTicker(w.rescanInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-t.C:
			projects, err := w.registry.ListAllProjects(ctx)
			if err != nil {
				tuilog.Log.Warn("watcher: rescan list projects failed", "error", err)
				continue
			}
			for _, p := range projects {
				if err := w.watchProject(p); err != nil {
					tuilog.Log.Warn("watcher: rescan failed to watch project", "project", p.Name, "error", err)
				}
			}
		}
	}
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	close(w.done)
	return w.watcher.Close()
}

func (w *Watcher) watchProject(p thinkt.Project) error {
	store, ok := w.registry.Get(p.Source)
	if !ok {
		return nil
	}

	sessions, err := store.ListSessions(context.Background(), p.ID)
	if err != nil {
		return err
	}

	type indexEntry struct {
		path  string
		entry sessionIndexEntry
	}
	var entries []indexEntry
	watchedDirs := make(map[string]bool)

	for _, s := range sessions {
		realPath, err := filepath.EvalSymlinks(s.FullPath)
		if err != nil {
			realPath = s.FullPath
		}

		dir := filepath.Dir(realPath)
		if hasExcludedDirComponent(dir) {
			continue
		}

		if !watchedDirs[dir] {
			if err := w.watcher.Add(dir); err != nil {
				tuilog.Log.Warn("watcher: failed to watch directory", "dir", dir, "error", err)
			} else {
				watchedDirs[dir] = true
			}
		}

		entries = append(entries, indexEntry{
			path: realPath,
			entry: sessionIndexEntry{
				session:   s,
				projectID: db.ScopedProjectID(p.Source, p.ID),
				source:    p.Source,
			},
		})
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range entries {
		w.sessionIndex[e.path] = e.entry
	}

	return nil
}

func (w *Watcher) watchLoop(ctx context.Context) {
	timers := make(map[string]*time.Timer)

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if isWatchableFile(event.Name) &&
				event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				path := event.Name
				if timer, ok := timers[path]; ok {
					timer.Stop()
				}
				timers[path] = time.AfterFunc(w.debounce, func() {
					w.handleFileChange(path)
				})
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			tuilog.Log.Warn("watcher: fsnotify error", "error", err)

		case <-w.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) handleFileChange(path string) {
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		realPath = path
	}

	entry, ok := func() (sessionIndexEntry, bool) {
		w.mu.Lock()
		defer w.mu.Unlock()

		if entry, ok := w.sessionIndex[realPath]; ok {
			return entry, true
		}

		tuilog.Log.Info("watcher: session not in index, attempting discovery", "path", realPath)
		entry, ok := w.discoverSession(realPath)
		if ok {
			w.sessionIndex[realPath] = entry
		}
		return entry, ok
	}()

	if !ok {
		tuilog.Log.Warn("watcher: file changed but no session found", "path", realPath)
		return
	}

	w.inFlightMu.Lock()
	if w.inFlight[realPath] {
		w.dirty[realPath] = true
		w.inFlightMu.Unlock()
		return
	}
	w.inFlight[realPath] = true
	w.inFlightMu.Unlock()

	w.reindexSession(realPath, entry)
}

func (w *Watcher) reindexSession(realPath string, entry sessionIndexEntry) {
	defer func() {
		w.inFlightMu.Lock()
		delete(w.inFlight, realPath)
		w.inFlightMu.Unlock()
	}()

	for {
		tuilog.Log.Info("watcher: re-indexing changed file", "path", realPath)

		ctx := context.Background()
		ingester := NewIngester(w.database, w.registry)
		if err := ingester.IngestSession(ctx, entry.projectID, entry.session); err != nil {
			tuilog.Log.Error("watcher: failed to re-index session",
				"session_id", entry.session.ID, "error", err)
		} else if w.OnSessionIndexed != nil {
			w.OnSessionIndexed(entry.session.ID)
		}

		w.inFlightMu.Lock()
		if !w.dirty[realPath] {
			w.inFlightMu.Unlock()
			return
		}
		delete(w.dirty, realPath)
		w.inFlightMu.Unlock()
	}
}

func (w *Watcher) discoverSession(path string) (sessionIndexEntry, bool) {
	ctx := context.Background()
	projects, err := w.registry.ListAllProjects(ctx)
	if err != nil {
		return sessionIndexEntry{}, false
	}

	for _, p := range projects {
		if !strings.HasPrefix(path, p.Path) {
			continue
		}
		store, ok := w.registry.Get(p.Source)
		if !ok {
			continue
		}

		sessions, err := store.ListSessions(ctx, p.ID)
		if err != nil {
			continue
		}

		for _, s := range sessions {
			sRealPath, err := filepath.EvalSymlinks(s.FullPath)
			if err != nil {
				sRealPath = s.FullPath
			}
			if sRealPath == path {
				return sessionIndexEntry{
					session:   s,
					projectID: db.ScopedProjectID(p.Source, p.ID),
					source:    p.Source,
				}, true
			}
		}
	}

	return sessionIndexEntry{}, false
}

// isWatchableFile returns true if the path is a JSONL session file.
func isWatchableFile(path string) bool {
	return strings.HasSuffix(path, ".jsonl")
}

// hasExcludedDirComponent checks if any path component matches an excluded directory.
func hasExcludedDirComponent(path string) bool {
	excluded := []string{".thinkt", ".git"}
	clean := filepath.Clean(path)
	for {
		base := filepath.Base(clean)
		for _, ex := range excluded {
			if base == ex {
				return true
			}
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			break
		}
		clean = parent
	}
	return false
}
