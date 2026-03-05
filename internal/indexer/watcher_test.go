package indexer

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestShouldHandleWatcherEvent(t *testing.T) {
	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{
			name:  "write jsonl",
			event: fsnotify.Event{Name: "/tmp/session.jsonl", Op: fsnotify.Write},
			want:  true,
		},
		{
			name:  "create jsonl",
			event: fsnotify.Event{Name: "/tmp/session.jsonl", Op: fsnotify.Create},
			want:  true,
		},
		{
			name:  "rename jsonl",
			event: fsnotify.Event{Name: "/tmp/session.jsonl", Op: fsnotify.Rename},
			want:  true,
		},
		{
			name:  "remove ignored",
			event: fsnotify.Event{Name: "/tmp/session.jsonl", Op: fsnotify.Remove},
			want:  false,
		},
		{
			name:  "non jsonl ignored",
			event: fsnotify.Event{Name: "/tmp/session.tmp", Op: fsnotify.Write},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldHandleWatcherEvent(tt.event); got != tt.want {
				t.Fatalf("shouldHandleWatcherEvent(%+v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}
