package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// FileEvent represents a detected change to a session file.
type FileEvent struct {
	Path      string // Absolute path to the changed file
	Source    string // Detected source type (e.g. "claude", "kimi")
	EventType string // "created" or "modified"
}

// warmWindow is how far back warmActive looks for recently-modified files.
const warmWindow = 10 * time.Minute

// FileWatcher monitors directories for new or modified JSONL session files.
//
// It uses lazy watching: on startup only the root and first-level session
// directories are watched. Deeper directories are added on demand when
// fsnotify fires Create events for new subdirectories, or warmed for
// directories containing recently-modified session files.
type FileWatcher struct {
	dirs    []WatchDir
	watcher *fsnotify.Watcher
	done    chan struct{}
	mu      sync.Mutex

	// configs maps watched root paths to their WatchDir so that runtime
	// new-directory events can inherit the correct config.
	configs map[string]WatchDir

	// watched tracks all paths added to the fsnotify watcher to avoid
	// duplicate Add calls.
	watched map[string]bool
}

// NewFileWatcher creates a new FileWatcher for the given directories.
func NewFileWatcher(dirs []WatchDir) (*FileWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	configs := make(map[string]WatchDir, len(dirs))
	for _, d := range dirs {
		configs[d.Path] = d
	}

	return &FileWatcher{
		dirs:    dirs,
		watcher: fw,
		done:    make(chan struct{}),
		configs: configs,
		watched: make(map[string]bool),
	}, nil
}

// Start begins watching directories and returns a channel of file events.
// Only shallow directories are watched initially; deeper directories are
// added on demand as new subdirectories are created, plus any directories
// containing recently-modified session files.
func (w *FileWatcher) Start(ctx context.Context) (<-chan FileEvent, error) {
	for _, wd := range w.dirs {
		w.addShallow(wd)
		w.warmActive(wd)
	}

	tuilog.Log.Info("Watcher started", "roots", len(w.dirs), "watched_dirs", len(w.watched))

	events := make(chan FileEvent, 64)
	go w.watchLoop(ctx, events)
	return events, nil
}

// addShallow watches only the root directory and its first-level IncludeDirs
// children. No deeper recursion.
func (w *FileWatcher) addShallow(wd WatchDir) {
	w.watchDir(wd.Path)

	if len(wd.Config.IncludeDirs) == 0 {
		return
	}

	includeSet := makeSet(wd.Config.IncludeDirs)
	entries, err := os.ReadDir(wd.Path)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && includeSet[e.Name()] {
			w.watchDir(filepath.Join(wd.Path, e.Name()))
		}
	}
}

// warmActive walks the directory tree looking for .jsonl files modified within
// the warm window. For each found file, it ensures the parent directory chain
// is watched up to the root. This seeds the watcher for sessions that were
// already active before the exporter started.
func (w *FileWatcher) warmActive(wd WatchDir) {
	root := wd.Path
	cfg := wd.Config
	includeSet := makeSet(cfg.IncludeDirs)
	excludeSet := makeSet(cfg.ExcludeDirs)
	cutoff := time.Now().Add(-warmWindow)

	rootDepth := strings.Count(root, string(filepath.Separator))
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Apply the same directory filtering as expandDir
		if d.IsDir() && path != root {
			name := d.Name()
			depth := strings.Count(path, string(filepath.Separator)) - rootDepth
			if depth == 1 && len(includeSet) > 0 && !includeSet[name] {
				return filepath.SkipDir
			}
			if excludeSet[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if cfg.MaxDepth > 0 && depth > cfg.MaxDepth {
				return filepath.SkipDir
			}
			return nil
		}

		// Check .jsonl files for recent modification
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			info, err := d.Info()
			if err != nil || info.ModTime().Before(cutoff) {
				return nil
			}
			// Warm the parent directory chain up to root
			w.warmAncestors(filepath.Dir(path), root)
		}
		return nil
	})
}

// warmAncestors ensures dir and all its ancestors up to (and including) root
// are watched.
func (w *FileWatcher) warmAncestors(dir, root string) {
	for {
		if w.watched[dir] {
			return // already watched, ancestors must be too
		}
		w.watchDir(dir)
		if dir == root {
			return
		}
		dir = filepath.Dir(dir)
		if len(dir) < len(root) {
			return // safety: don't go above root
		}
	}
}

// expandDir decides whether a newly-created directory should be watched,
// applying the same filtering rules as the initial walk. It watches only
// the single directory (no recursion).
func (w *FileWatcher) expandDir(path string) {
	wd, ok := w.rootConfigFor(path)
	if !ok {
		return
	}

	name := filepath.Base(path)
	cfg := wd.Config
	root := wd.Path
	depth := strings.Count(path, string(filepath.Separator)) - strings.Count(root, string(filepath.Separator))

	// Apply filtering
	if depth == 1 && len(cfg.IncludeDirs) > 0 {
		includeSet := makeSet(cfg.IncludeDirs)
		if !includeSet[name] {
			return
		}
	}
	excludeSet := makeSet(cfg.ExcludeDirs)
	if excludeSet[name] {
		return
	}
	if strings.HasPrefix(name, ".") {
		return
	}
	if cfg.MaxDepth > 0 && depth > cfg.MaxDepth {
		return
	}

	w.watchDir(path)
}

// watchDir adds a single directory to the fsnotify watcher if not already watched.
func (w *FileWatcher) watchDir(path string) {
	if w.watched[path] {
		return
	}
	if err := w.watcher.Add(path); err != nil {
		tuilog.Log.Warn("Failed to watch directory", "dir", path, "error", err)
		return
	}
	w.watched[path] = true
	tuilog.Log.Debug("Watching directory", "dir", path)
}

// Stop stops the file watcher and releases resources.
func (w *FileWatcher) Stop() error {
	close(w.done)
	return w.watcher.Close()
}

func (w *FileWatcher) watchLoop(ctx context.Context, events chan<- FileEvent) {
	defer close(events)

	// Debounce timers to avoid duplicate events on rapid writes
	timers := make(map[string]*time.Timer)
	const debounceDuration = 2 * time.Second

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// If a new directory was created, lazily expand the watch.
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					name := filepath.Base(event.Name)
					if !strings.HasSuffix(name, ".lock") {
						w.expandDir(event.Name)
					}
					continue
				}
			}

			// Only care about .jsonl files
			if !strings.HasSuffix(event.Name, ".jsonl") {
				continue
			}

			// Handle creates and writes
			var eventType string
			switch {
			case event.Op&fsnotify.Create == fsnotify.Create:
				eventType = "created"
			case event.Op&fsnotify.Write == fsnotify.Write:
				eventType = "modified"
			default:
				continue
			}

			// Debounce: reset timer for this file
			w.mu.Lock()
			if timer, ok := timers[event.Name]; ok {
				timer.Stop()
			}

			path := event.Name
			et := eventType
			source := w.sourceFor(path)
			timers[path] = time.AfterFunc(debounceDuration, func() {
				realPath, err := filepath.EvalSymlinks(path)
				if err != nil {
					realPath = path
				}

				fe := FileEvent{
					Path:      realPath,
					Source:    source,
					EventType: et,
				}

				select {
				case events <- fe:
					tuilog.Log.Debug("File event", "path", fe.Path, "source", fe.Source, "type", fe.EventType)
				case <-ctx.Done():
				case <-w.done:
				}
			})
			w.mu.Unlock()

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			tuilog.Log.Error("Watcher error", "error", err)

		case <-w.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

// rootConfigFor finds the registered root WatchDir that owns the given path.
func (w *FileWatcher) rootConfigFor(path string) (WatchDir, bool) {
	for root, wd := range w.configs {
		if strings.HasPrefix(path, root+string(filepath.Separator)) || path == root {
			return wd, true
		}
	}
	return WatchDir{}, false
}

// sourceFor returns the source name for a file path by matching it against
// registered watch roots. Falls back to path-based detection.
func (w *FileWatcher) sourceFor(path string) string {
	for root, wd := range w.configs {
		if strings.HasPrefix(path, root) {
			return wd.Source
		}
	}
	return detectSource(path)
}

// detectSource guesses the source type from the file path.
// Used as a fallback when no WatchDir config matches.
func detectSource(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, ".claude"):
		return "claude"
	case strings.Contains(lower, ".kimi"):
		return "kimi"
	case strings.Contains(lower, ".codex"):
		return "codex"
	case strings.Contains(lower, ".gemini"):
		return "gemini"
	case strings.Contains(lower, ".copilot"):
		return "copilot"
	default:
		return "unknown"
	}
}

// makeSet converts a string slice to a set for O(1) lookups.
func makeSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
