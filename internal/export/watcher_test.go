package export

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestFileWatcher_PruneStale(t *testing.T) {
	dir := t.TempDir()

	// Create subdirectories to watch
	shallow := filepath.Join(dir, "projects")
	deep1 := filepath.Join(shallow, "session-a")
	deep2 := filepath.Join(shallow, "session-b")
	for _, d := range []string{shallow, deep1, deep2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	wd := WatchDir{
		Path:   dir,
		Source: "test",
		Config: thinkt.WatchConfig{
			IncludeDirs: []string{"projects"},
			MaxDepth:    4,
		},
	}

	fw, err := NewFileWatcher([]WatchDir{wd})
	if err != nil {
		t.Fatal(err)
	}
	defer fw.Stop() //nolint:errcheck

	// addShallow marks root + include dirs as permanent
	fw.addShallow(wd)

	if !fw.shallowDirs[dir] {
		t.Error("root should be marked as shallow")
	}
	if !fw.shallowDirs[shallow] {
		t.Error("include dir should be marked as shallow")
	}

	// Manually add deeper dirs (simulating warmActive/expandDir)
	fw.watchDir(deep1)
	fw.watchDir(deep2)

	if len(fw.watched) != 4 {
		t.Fatalf("expected 4 watched dirs, got %d", len(fw.watched))
	}

	// Make deep dirs look stale
	past := time.Now().Add(-20 * time.Minute)
	fw.lastActivity[deep1] = past
	fw.lastActivity[deep2] = past

	// Prune with 10-minute threshold
	pruned := fw.pruneStale(10 * time.Minute)
	if pruned != 2 {
		t.Errorf("expected 2 pruned, got %d", pruned)
	}

	// Shallow dirs should survive
	if !fw.watched[dir] {
		t.Error("root should still be watched")
	}
	if !fw.watched[shallow] {
		t.Error("include dir should still be watched")
	}

	// Deep dirs should be gone
	if fw.watched[deep1] {
		t.Error("stale deep1 should have been unwatched")
	}
	if fw.watched[deep2] {
		t.Error("stale deep2 should have been unwatched")
	}
}

func TestFileWatcher_PruneKeepsActive(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "active-session")
	stale := filepath.Join(dir, "stale-session")
	for _, d := range []string{active, stale} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	wd := WatchDir{Path: dir, Source: "test"}
	fw, err := NewFileWatcher([]WatchDir{wd})
	if err != nil {
		t.Fatal(err)
	}
	defer fw.Stop() //nolint:errcheck

	fw.addShallow(wd)
	fw.watchDir(active)
	fw.watchDir(stale)

	// active dir has recent activity, stale does not
	fw.lastActivity[active] = time.Now()
	fw.lastActivity[stale] = time.Now().Add(-20 * time.Minute)

	pruned := fw.pruneStale(10 * time.Minute)
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}

	if !fw.watched[active] {
		t.Error("active dir should still be watched")
	}
	if fw.watched[stale] {
		t.Error("stale dir should have been unwatched")
	}
}

func TestFileWatcher_TouchDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	wd := WatchDir{Path: dir, Source: "test"}
	fw, err := NewFileWatcher([]WatchDir{wd})
	if err != nil {
		t.Fatal(err)
	}
	defer fw.Stop() //nolint:errcheck

	fw.watchDir(sub)

	// Set to old timestamp
	old := time.Now().Add(-1 * time.Hour)
	fw.lastActivity[sub] = old

	// Touch via a file path under sub
	fw.touchDir(filepath.Join(sub, "session.jsonl"))

	if !fw.lastActivity[sub].After(old) {
		t.Error("touchDir should have updated lastActivity to a newer time")
	}
}

func TestFileWatcher_TouchDirIgnoresUnwatched(t *testing.T) {
	dir := t.TempDir()
	wd := WatchDir{Path: dir, Source: "test"}
	fw, err := NewFileWatcher([]WatchDir{wd})
	if err != nil {
		t.Fatal(err)
	}
	defer fw.Stop() //nolint:errcheck

	// Touch a dir that isn't watched — should not create an entry
	fw.touchDir(filepath.Join(dir, "nonexistent", "file.jsonl"))

	if _, exists := fw.lastActivity[filepath.Join(dir, "nonexistent")]; exists {
		t.Error("touchDir should not create entries for unwatched dirs")
	}
}

func TestFileWatcher_TimerCleanup(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a JSONL file to trigger events
	testFile := filepath.Join(sub, "test.jsonl")
	if err := os.WriteFile(testFile, []byte(`{"uuid":"1","type":"user","timestamp":"2025-01-01T00:00:00Z","text":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	wd := WatchDir{Path: dir, Source: "test"}
	fw, err := NewFileWatcher([]WatchDir{wd})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := fw.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Write to the file to trigger a debounced event
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"uuid":"2","type":"user","timestamp":"2025-01-01T00:00:01Z","text":"hello"}` + "\n")
	f.Close()

	// Wait for the debounced event (2s debounce + margin)
	select {
	case <-events:
		// Got the event, now wait a bit for timer cleanup
		time.Sleep(100 * time.Millisecond)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for file event")
	}

	cancel()
	fw.Stop() //nolint:errcheck
}
