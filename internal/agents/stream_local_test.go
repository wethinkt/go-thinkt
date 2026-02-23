package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalStream_NewEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	// Write initial content
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	// Write an initial entry
	entry := map[string]any{
		"type":    "human",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": "initial"}}},
	}
	data, _ := json.Marshal(entry)
	f.Write(data)
	f.Write([]byte("\n"))
	f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start stream — it should seek to end, only delivering new entries
	ch, err := StreamLocal(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	// Give watcher time to start
	time.Sleep(200 * time.Millisecond)

	// Append a new entry
	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	newEntry := map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": "hello world"}}},
	}
	data, _ = json.Marshal(newEntry)
	f.Write(data)
	f.Write([]byte("\n"))
	f.Close()

	select {
	case e := <-ch:
		if e.Role != "assistant" {
			t.Errorf("expected assistant role, got %s", e.Role)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for stream entry")
	}
}
