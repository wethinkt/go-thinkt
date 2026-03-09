package claude

import (
	"context"
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
	mockStatsigStableID(t, baseDir, "ws-merge")

	// Create a project with one session file.
	projectDir := filepath.Join(baseDir, "projects", "-Users-evan-merge-test")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	sessPath := filepath.Join(projectDir, "sess-abc.jsonl")
	createTestSessionFile(t, sessPath, []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-06-01T10:00:00Z", "message": map[string]any{
			"content": "Hello from test",
		}},
	})

	// Stat the file to get its mtime and size (needed for cache entry to match).
	info, err := os.Stat(sessPath)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}

	// Pre-populate the metadata cache file with enriched data.
	mc := &thinkt.MetadataCache{
		Version: 1,
		Source:  thinkt.SourceClaude,
		Sessions: map[string]thinkt.CachedSession{
			sessPath: {
				FirstPrompt: "Cached prompt text",
				Model:       "claude-sonnet-4-20250514",
				EntryCount:  42,
				GitBranch:   "feature-branch",
				ModifiedAt:  info.ModTime(),
				FileSize:    info.Size(),
			},
		},
	}

	// Save the cache to disk so LoadMetadataCache can find it.
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("creating cache dir: %v", err)
	}
	// We need to set the dir field via LoadMetadataCache's path, so write manually.
	mcWithDir, _ := thinkt.LoadMetadataCache(thinkt.SourceClaude, cacheDir)
	for k, v := range mc.Sessions {
		mcWithDir.Set(k, v)
	}
	if err := mcWithDir.Save(); err != nil {
		t.Fatalf("saving metadata cache: %v", err)
	}

	// Create store with explicit cache dir.
	store := NewStoreWithCacheDir(baseDir, cacheDir)

	sessions, err := store.ListSessions(context.Background(), projectDir)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	sess := sessions[0]
	if sess.FirstPrompt != "Cached prompt text" {
		t.Errorf("expected FirstPrompt='Cached prompt text', got %q", sess.FirstPrompt)
	}
	if sess.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected Model='claude-sonnet-4-20250514', got %q", sess.Model)
	}
	if sess.EntryCount != 42 {
		t.Errorf("expected EntryCount=42, got %d", sess.EntryCount)
	}
	if sess.GitBranch != "feature-branch" {
		t.Errorf("expected GitBranch='feature-branch', got %q", sess.GitBranch)
	}
}

func TestListSessions_WithEnrich_CallsCallback(t *testing.T) {
	baseDir := t.TempDir()
	cacheDir := t.TempDir()
	mockStatsigStableID(t, baseDir, "ws-enrich")

	// Create a project with a session containing user + assistant entries.
	projectDir := filepath.Join(baseDir, "projects", "-Users-evan-enrich-test")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	sessPath := filepath.Join(projectDir, "sess-enrich.jsonl")
	createTestSessionFile(t, sessPath, []map[string]any{
		{"type": "user", "uuid": "u1", "timestamp": "2024-06-01T10:00:00Z", "message": map[string]any{
			"content": "Enrich me please",
		}},
		{"type": "assistant", "uuid": "a1", "timestamp": "2024-06-01T10:01:00Z", "message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Sure thing!"}},
			"model":   "claude-sonnet-4-20250514",
		}},
		{"type": "user", "uuid": "u2", "timestamp": "2024-06-01T10:02:00Z", "message": map[string]any{
			"content": "Thanks",
		}},
	})

	store := NewStoreWithCacheDir(baseDir, cacheDir)

	var mu sync.Mutex
	var callbackSessions []thinkt.SessionMeta
	done := make(chan struct{}, 1)

	sessions, err := store.ListSessions(context.Background(), projectDir,
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
	if enriched.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected Model='claude-sonnet-4-20250514', got %q", enriched.Model)
	}
	if enriched.EntryCount == 0 {
		t.Error("expected EntryCount > 0 after enrichment")
	}

	// Verify the cache file was written to disk.
	mcOnDisk, _ := thinkt.LoadMetadataCache(thinkt.SourceClaude, cacheDir)
	cached, ok := mcOnDisk.Lookup(enriched.FullPath, enriched.ModifiedAt, enriched.FileSize)
	if !ok {
		t.Fatal("expected metadata cache entry on disk after enrichment")
	}
	if cached.FirstPrompt != "Enrich me please" {
		t.Errorf("expected cached FirstPrompt='Enrich me please', got %q", cached.FirstPrompt)
	}
}
