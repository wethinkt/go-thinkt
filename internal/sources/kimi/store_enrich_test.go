package kimi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestListSessions_MergesFromCache(t *testing.T) {
	baseDir := t.TempDir()
	cacheDir := t.TempDir()

	// Create a session.
	projectPath := "/Users/test/cache-merge"
	hash := workDirHash(projectPath)
	sessionDir := filepath.Join(baseDir, "sessions", hash, "session-merge")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}

	contextPath := filepath.Join(sessionDir, "context.jsonl")
	writeJSONL(t, contextPath, []map[string]any{
		{"role": "user", "content": "Hello from test"},
	})

	// Stat the file to get mtime and size for the cache entry.
	info, err := os.Stat(contextPath)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}

	// Pre-populate the metadata cache with enriched data.
	mcWithDir, _ := thinkt.LoadMetadataCache(thinkt.SourceKimi, cacheDir)
	mcWithDir.Set(contextPath, thinkt.CachedSession{
		FirstPrompt: "Cached prompt text",
		Model:       "moonshot-v1-128k",
		EntryCount:  42,
		ModifiedAt:  info.ModTime(),
		FileSize:    info.Size(),
	})
	if err := mcWithDir.Save(); err != nil {
		t.Fatalf("saving metadata cache: %v", err)
	}

	store := NewStoreWithCacheDir(baseDir, cacheDir)

	sessions, err := store.ListSessions(context.Background(), projectPath)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	sess := sessions[0]
	// FirstPrompt comes from listing (firstPromptLimited), but cache should
	// fill it if listing returned empty. Here listing finds it, so we check
	// the cached Model and EntryCount.
	if sess.Model != "moonshot-v1-128k" {
		t.Errorf("expected Model='moonshot-v1-128k', got %q", sess.Model)
	}
	if sess.EntryCount != 42 {
		t.Errorf("expected EntryCount=42, got %d", sess.EntryCount)
	}
}

func TestListSessions_WithEnrich_CallsCallback(t *testing.T) {
	baseDir := t.TempDir()
	cacheDir := t.TempDir()

	// Create a session with user + assistant entries (assistant has model field).
	projectPath := "/Users/test/enrich-test"
	hash := workDirHash(projectPath)
	sessionDir := filepath.Join(baseDir, "sessions", hash, "session-enrich")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("creating session dir: %v", err)
	}

	contextPath := filepath.Join(sessionDir, "context.jsonl")
	writeJSONL(t, contextPath, []map[string]any{
		{"role": "user", "content": "Enrich me please"},
		{"role": "assistant", "model": "moonshot-v1-128k", "content": []map[string]any{
			{"type": "text", "text": "Sure thing!"},
		}},
		{"role": "user", "content": "Thanks"},
	})

	store := NewStoreWithCacheDir(baseDir, cacheDir)

	var mu sync.Mutex
	var callbackSessions []thinkt.SessionMeta
	done := make(chan struct{}, 1)

	sessions, err := store.ListSessions(context.Background(), projectPath,
		thinkt.WithEnrich(func(pid string, enriched []thinkt.SessionMeta) {
			mu.Lock()
			callbackSessions = enriched
			mu.Unlock()
			select {
			case done <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Wait for the background goroutine to call back.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for enrich callback")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(callbackSessions) != 1 {
		t.Fatalf("expected 1 enriched session in callback, got %d", len(callbackSessions))
	}

	enriched := callbackSessions[0]
	if enriched.FirstPrompt != "Enrich me please" {
		t.Errorf("expected FirstPrompt='Enrich me please', got %q", enriched.FirstPrompt)
	}
	if enriched.Model != "moonshot-v1-128k" {
		t.Errorf("expected Model='moonshot-v1-128k', got %q", enriched.Model)
	}
	if enriched.EntryCount == 0 {
		t.Error("expected EntryCount > 0 after enrichment")
	}

	// Verify the cache file was written to disk.
	mcOnDisk, _ := thinkt.LoadMetadataCache(thinkt.SourceKimi, cacheDir)
	cached, ok := mcOnDisk.Lookup(enriched.FullPath, enriched.ModifiedAt, enriched.FileSize)
	if !ok {
		t.Fatal("expected metadata cache entry on disk after enrichment")
	}
	if cached.FirstPrompt != "Enrich me please" {
		t.Errorf("expected cached FirstPrompt='Enrich me please', got %q", cached.FirstPrompt)
	}
	if cached.Model != "moonshot-v1-128k" {
		t.Errorf("expected cached Model='moonshot-v1-128k', got %q", cached.Model)
	}
}

func TestExtractKimiModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "context.jsonl")

	// Write a file with assistant entry that has a model field.
	lines := []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "model": "moonshot-v1-32k", "content": []map[string]any{
			{"type": "text", "text": "Hi"},
		}},
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	for _, line := range lines {
		data, _ := json.Marshal(line)
		_, _ = f.Write(data)
		_, _ = f.WriteString("\n")
	}
	f.Close()

	model := extractKimiModel(path)
	if model != "moonshot-v1-32k" {
		t.Errorf("expected model 'moonshot-v1-32k', got %q", model)
	}
}

func TestExtractKimiModel_NoModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "context.jsonl")

	lines := []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": []map[string]any{
			{"type": "text", "text": "Hi"},
		}},
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	for _, line := range lines {
		data, _ := json.Marshal(line)
		_, _ = f.Write(data)
		_, _ = f.WriteString("\n")
	}
	f.Close()

	model := extractKimiModel(path)
	if model != "" {
		t.Errorf("expected empty model, got %q", model)
	}
}

func TestCountKimiLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "context.jsonl")

	lines := []map[string]any{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "Hi"}}},
		{"role": "user", "content": "Bye"},
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	for _, line := range lines {
		data, _ := json.Marshal(line)
		_, _ = f.Write(data)
		_, _ = f.WriteString("\n")
	}
	f.Close()

	count := countKimiLines(path)
	if count != 3 {
		t.Errorf("expected 3 lines, got %d", count)
	}
}
