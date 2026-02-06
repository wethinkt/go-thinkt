package indexer

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Watcher monitors session directories for changes and triggers ingestion.
type Watcher struct {
	ingester *Ingester
	registry *thinkt.StoreRegistry
	watcher  *fsnotify.Watcher
	done     chan struct{}
}

// NewWatcher creates a new Watcher instance.
func NewWatcher(ingester *Ingester, registry *thinkt.StoreRegistry) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		ingester: ingester,
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
		dir := filepath.Dir(s.FullPath)
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
	log.Printf("File changed, re-indexing: %s", path)

	ctx := context.Background()
	projects, _ := w.registry.ListAllProjects(ctx)

	for _, p := range projects {
		store, _ := w.registry.Get(p.Source)
		sessions, _ := store.ListSessions(ctx, p.ID)

		for _, s := range sessions {
			if s.FullPath == path {
				if err := w.ingester.IngestSession(ctx, p.ID, s); err != nil {
					log.Printf("Failed to re-index session %s: %v", s.ID, err)
				}
				return
			}
		}
	}
}
