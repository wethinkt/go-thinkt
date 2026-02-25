package indexer

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
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
	dbPath       string
	embDBPath    string
	registry     *thinkt.StoreRegistry
	embedder     *embedding.Embedder          // shared, owned by caller (e.g. server)
	debounce     time.Duration                // debounce delay for file changes
	watcher      *fsnotify.Watcher
	done         chan struct{}
	mu           sync.Mutex
	sessionIndex map[string]sessionIndexEntry // normalized path -> session info
	dbPool       *db.LazyPool                 // lazy-close connection pool (index DB)
	embDBPool    *db.LazyPool                 // lazy-close connection pool (embeddings DB)

	// inFlight tracks sessions currently being re-indexed.
	// If a session is in-flight and another change comes in, we mark it
	// dirty so it re-runs after the current ingestion finishes.
	inFlightMu sync.Mutex
	inFlight   map[string]bool // session path -> currently processing
	dirty      map[string]bool // session path -> needs re-run after current finishes
}

// NewWatcher creates a new Watcher instance.
// The embedder may be nil if embedding is unavailable.
// A zero debounce defaults to 2 seconds.
func NewWatcher(dbPath, embDBPath string, registry *thinkt.StoreRegistry, embedder *embedding.Embedder, debounce time.Duration) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if debounce <= 0 {
		debounce = 2 * time.Second
	}

	return &Watcher{
		dbPath:       dbPath,
		embDBPath:    embDBPath,
		registry:     registry,
		embedder:     embedder,
		debounce:     debounce,
		watcher:      fw,
		done:         make(chan struct{}),
		sessionIndex: make(map[string]sessionIndexEntry),
		dbPool:       db.NewLazyPool(dbPath, db.IndexSchema(), 5*time.Second),
		embDBPool:    db.NewLazyPool(embDBPath, db.EmbeddingsSchemaForDim(embDim(embedder)), 5*time.Second),
		inFlight:     make(map[string]bool),
		dirty:        make(map[string]bool),
	}, nil
}

// Start begins monitoring projects for changes.
func (w *Watcher) Start(ctx context.Context) error {
	// Initial discovery and watch setup
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
	return nil
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	close(w.done)
	err := w.watcher.Close()

	// Close DB pools
	if w.dbPool != nil {
		if poolErr := w.dbPool.Close(); poolErr != nil {
			err = errors.Join(err, poolErr)
		}
	}
	if w.embDBPool != nil {
		if poolErr := w.embDBPool.Close(); poolErr != nil {
			err = errors.Join(err, poolErr)
		}
	}

	return err
}

// SetEmbedder swaps the embedder used for future re-indexing.
// Pass nil to disable embedding without stopping the watcher.
func (w *Watcher) SetEmbedder(e *embedding.Embedder) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.embedder = e
}

// embDim returns the embedding dimension from an embedder, or 768 (nomic default) if nil.
func embDim(e *embedding.Embedder) int {
	if e != nil {
		return e.Dim()
	}
	spec, _ := embedding.LookupModel("")
	return spec.Dim
}

func (w *Watcher) watchProject(p thinkt.Project) error {
	// Get store and list sessions without lock (I/O operations)
	store, ok := w.registry.Get(p.Source)
	if !ok {
		return nil
	}

	sessions, err := store.ListSessions(context.Background(), p.ID)
	if err != nil {
		return err
	}

	// Prepare index entries and watch directories without holding lock
	type indexEntry struct {
		path  string
		entry sessionIndexEntry
	}
	var entries []indexEntry
	watchedDirs := make(map[string]bool)

	for _, s := range sessions {
		// Resolve symlinks to get the "real" physical path
		realPath, err := filepath.EvalSymlinks(s.FullPath)
		if err != nil {
			realPath = s.FullPath // Fallback to original if resolution fails
		}

		dir := filepath.Dir(realPath)

		// Exclude internal/hidden directories to avoid recursive loops
		if hasExcludedDirComponent(dir, []string{".thinkt", ".git"}) {
			continue
		}

		// Add to fsnotify watcher (outside lock - fsnotify has its own locking)
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
				projectID: ScopedProjectID(p.Source, p.ID),
				source:    p.Source,
			},
		})
	}

	// Only hold lock when updating the sessionIndex (fast memory operation)
	func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, e := range entries {
			w.sessionIndex[e.path] = e.entry
		}
	}()

	return nil
}

// hasExcludedDirComponent checks if any path component matches an excluded directory name.
// This ensures we exclude hidden directories like .thinkt or .git without matching
// legitimate paths like "my.thinkt.project".
func hasExcludedDirComponent(path string, excluded []string) bool {
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

func (w *Watcher) watchLoop(ctx context.Context) {
	// Debounce timer to avoid spamming ingestion on rapid writes
	timers := make(map[string]*time.Timer)

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// We only care about writes to .jsonl files
			if !strings.HasSuffix(event.Name, ".jsonl") {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				// Debounce ingestion
				if timer, ok := timers[event.Name]; ok {
					timer.Stop()
				}

				timers[event.Name] = time.AfterFunc(w.debounce, func() {
					w.handleFileChange(event.Name)
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
	// Normalize incoming path without lock (syscalls are slow)
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		realPath = path
	}

	// Look up session under lock using closure pattern — defer ensures
	// the lock is always released regardless of return paths.
	entry, ok := func() (sessionIndexEntry, bool) {
		w.mu.Lock()
		defer w.mu.Unlock()

		if entry, ok := w.sessionIndex[realPath]; ok {
			return entry, true
		}

		// Session not in index — might be a new session. Try to discover it
		// by scanning the project that owns this file's directory.
		tuilog.Log.Info("watcher: session not in index, attempting discovery", "path", realPath)
		entry, ok := w.discoverSession(realPath)
		if ok {
			// Add to index for future lookups
			w.sessionIndex[realPath] = entry
		}
		return entry, ok
	}()

	if !ok {
		tuilog.Log.Warn("watcher: file changed but no session found", "path", realPath)
		return
	}

	// Serialize re-indexing per session to avoid DuckDB transaction conflicts.
	// If already in-flight, mark dirty so it re-runs after current finishes.
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

// reindexSession runs ingestion for the session, then re-runs if the file
// was modified again during processing.
func (w *Watcher) reindexSession(realPath string, entry sessionIndexEntry) {
	defer func() {
		w.inFlightMu.Lock()
		delete(w.inFlight, realPath)
		w.inFlightMu.Unlock()
	}()

	for {
		tuilog.Log.Info("watcher: re-indexing changed file", "path", realPath)

		ctx := context.Background()
		database, err := w.dbPool.Acquire()
		if err != nil {
			tuilog.Log.Error("watcher: failed to open database", "error", err)
			return
		}

		embDB, err := w.embDBPool.Acquire()
		if err != nil {
			tuilog.Log.Error("watcher: failed to open embeddings database", "error", err)
			w.dbPool.Release()
			return
		}

		ingester := NewIngester(database, embDB, w.registry, w.embedder)
		if err := ingester.IngestAndEmbedSession(ctx, entry.projectID, entry.session); err != nil {
			tuilog.Log.Error("watcher: failed to re-index session", "session_id", entry.session.ID, "error", err)
		}

		w.embDBPool.Release()
		w.dbPool.Release()

		// Check if the file was modified again while we were processing.
		w.inFlightMu.Lock()
		if !w.dirty[realPath] {
			w.inFlightMu.Unlock()
			return
		}
		delete(w.dirty, realPath)
		w.inFlightMu.Unlock()
	}
}

// discoverSession attempts to find a session for the given path by scanning
// projects. This is called when a file change is detected for a path not in
// the session index (e.g., a newly created session).
func (w *Watcher) discoverSession(path string) (sessionIndexEntry, bool) {
	ctx := context.Background()
	projects, err := w.registry.ListAllProjects(ctx)
	if err != nil {
		return sessionIndexEntry{}, false
	}

	// Optimization: check projects whose path is a prefix of the changed file first
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
					projectID: ScopedProjectID(p.Source, p.ID),
					source:    p.Source,
				}, true
			}
		}
	}

	return sessionIndexEntry{}, false
}
