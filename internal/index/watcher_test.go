package index

import (
	"path/filepath"
	"testing"
	"time"
)

func TestShouldHandleWatcherEvent(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"jsonl file", "/tmp/session.jsonl", true},
		{"json file", "/tmp/data.json", false},
		{"txt file", "/tmp/notes.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWatchableFile(tt.path); got != tt.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHasExcludedDirComponent(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/home/user/.git/config", true},
		{"/home/user/.thinkt/data", true},
		{"/home/user/my.thinkt.project/file", false},
		{"/home/user/projects/go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := hasExcludedDirComponent(tt.path); got != tt.want {
				t.Errorf("hasExcludedDirComponent(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestWatcherDebounceDefault(t *testing.T) {
	w := &Watcher{}
	w.setDebounce(0)
	if w.debounce != 2*time.Second {
		t.Fatalf("debounce = %v, want 2s", w.debounce)
	}
	w.setDebounce(5 * time.Second)
	if w.debounce != 5*time.Second {
		t.Fatalf("debounce = %v, want 5s", w.debounce)
	}
}

func TestSessionIndexEntry(t *testing.T) {
	// Just verify the struct is usable.
	_ = sessionIndexEntry{
		projectID: "claude::test",
		source:    "claude",
	}
	_ = filepath.Join("a", "b") // suppress unused import
}
