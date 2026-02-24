package agents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHub_LocalDetectionAndStream(t *testing.T) {
	// Create a fake session file
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "test-session.jsonl")

	entry := map[string]any{
		"type":    "human",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": "hello"}}},
	}
	data, _ := json.Marshal(entry)
	os.WriteFile(sessionPath, append(data, '\n'), 0644)

	// Create hub with no detector (we'll test stream directly)
	hub := NewHub(HubConfig{})

	// Manually add a local agent for testing
	hub.mu.Lock()
	hub.agents = append(hub.agents, UnifiedAgent{
		ID:          "test-session",
		SessionID:   "test-session",
		SessionPath: sessionPath,
		MachineID:   hub.localFP,
		Status:      "active",
		Source:      "claude",
	})
	hub.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := hub.Stream(ctx, "test-session")
	if err != nil {
		t.Fatal(err)
	}

	// Give watcher time to start
	time.Sleep(200 * time.Millisecond)

	// Append a new entry
	f, _ := os.OpenFile(sessionPath, os.O_APPEND|os.O_WRONLY, 0644)
	newEntry := map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []map[string]any{{"type": "text", "text": "world"}}},
	}
	data, _ = json.Marshal(newEntry)
	f.Write(data)
	f.Write([]byte("\n"))
	f.Close()

	select {
	case e := <-ch:
		if e.Role != "assistant" {
			t.Errorf("expected assistant, got %s", e.Role)
		}
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}
