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

// FileWatcher monitors directories for new or modified JSONL session files.
type FileWatcher struct {
	dirs    []string
	watcher *fsnotify.Watcher
	done    chan struct{}
	mu      sync.Mutex
}

// NewFileWatcher creates a new FileWatcher for the given directories.
func NewFileWatcher(dirs []string) (*FileWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &FileWatcher{
		dirs:    dirs,
		watcher: fw,
		done:    make(chan struct{}),
	}, nil
}

// Start begins watching directories (recursively) and returns a channel of file events.
// The returned channel is closed when the context is canceled or Stop is called.
func (w *FileWatcher) Start(ctx context.Context) (<-chan FileEvent, error) {
	for _, dir := range w.dirs {
		w.addRecursive(dir)
	}

	events := make(chan FileEvent, 64)
	go w.watchLoop(ctx, events)
	return events, nil
}

// addRecursive walks a directory tree and adds all directories to the watcher.
func (w *FileWatcher) addRecursive(root string) {
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden subdirectories (except the root itself)
		if path != root {
			name := d.Name()
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
		}
		if err := w.watcher.Add(path); err != nil {
			tuilog.Log.Warn("Failed to watch directory", "dir", path, "error", err)
		} else {
			tuilog.Log.Debug("Watching directory", "dir", path)
		}
		return nil
	})
	tuilog.Log.Info("Watching directory tree", "root", root)
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

			// If a new directory was created, watch it recursively
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.addRecursive(event.Name)
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
			timers[path] = time.AfterFunc(debounceDuration, func() {
				realPath, err := filepath.EvalSymlinks(path)
				if err != nil {
					realPath = path
				}

				fe := FileEvent{
					Path:      realPath,
					Source:    detectSource(realPath),
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

// detectSource guesses the source type from the file path.
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
