package indexer

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Watcher monitors session directories for changes and triggers ingestion.
type Watcher struct {
	dbPath   string
	registry *thinkt.StoreRegistry
	watcher  *fsnotify.Watcher
	done     chan struct{}
	mu       sync.Mutex
}

// NewWatcher creates a new Watcher instance.
func NewWatcher(dbPath string, registry *thinkt.StoreRegistry) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		dbPath:   dbPath,
		registry: registry,
		watcher:  fw,
		done:     make(chan struct{}),
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
			log.Printf("Error watching project %s: %v", p.Name, err)
		}
	}

	go w.watchLoop(ctx)
	return nil
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

	// Track which directories we are watching to avoid duplicates
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
		if !watchedDirs[dir] {
			if err := w.watcher.Add(dir); err != nil {
				log.Printf("Failed to watch directory %s: %v", dir, err)
			} else {
				watchedDirs[dir] = true
			}
		}
	}

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
	const debounceDuration = 2 * time.Second

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

				timers[event.Name] = time.AfterFunc(debounceDuration, func() {
					w.handleFileChange(event.Name)
				})
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)

		case <-w.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (w *Watcher) handleFileChange(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Normalize incoming path
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		realPath = path
	}

	log.Printf("File changed, re-indexing: %s", realPath)

	ctx := context.Background()
	projects, _ := w.registry.ListAllProjects(ctx)

	for _, p := range projects {
		store, _ := w.registry.Get(p.Source)
		sessions, _ := store.ListSessions(ctx, p.ID)

		for _, s := range sessions {
			// Compare normalized paths
			sRealPath, _ := filepath.EvalSymlinks(s.FullPath)
			if sRealPath == "" {
				sRealPath = s.FullPath
			}

			if sRealPath == realPath {
				database, err := db.Open(w.dbPath)
				if err != nil {
					log.Printf("Failed to open database: %v", err)
					return
				}
				ingester := NewIngester(database, w.registry)
				if err := ingester.IngestSession(ctx, ScopedProjectID(p.Source, p.ID), s); err != nil {
					log.Printf("Failed to re-index session %s: %v", s.ID, err)
				}
				database.Close()
				return
			}
		}
	}
}
