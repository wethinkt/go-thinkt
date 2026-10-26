package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// The background total used by Progress must count emitted entries, not raw
// lines, or the bar stalls at the bookkeeping ratio and jumps to 100% at EOF.
func TestLazySessionProgressTotalCountsEntries(t *testing.T) {
	data, err := os.ReadFile(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	const repeat = 200
	path := filepath.Join(t.TempDir(), "big.jsonl")
	var big []byte
	for i := 0; i < repeat; i++ {
		big = append(big, data...)
	}
	if err := os.WriteFile(path, big, 0o644); err != nil {
		t.Fatal(err)
	}
	ls, err := OpenLazySession(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ls.Close()
	if ls.fullyLoaded {
		t.Fatal("test file must exceed the preload window")
	}
	deadline := time.Now().Add(5 * time.Second)
	for ls.totalEntries.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := ls.totalEntries.Load(); got != int64(modernFixtureEntries*repeat) {
		t.Fatalf("background total = %d, want %d emitted entries (raw lines are %d)", got, modernFixtureEntries*repeat, 14*repeat)
	}
}

// A legacy sessions-index.json hit must still get title, counts, and state
// from the file; the index predates those fields and counts raw lines.
func TestGetSessionMetaLegacyIndexStillEnriches(t *testing.T) {
	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "-home-dev-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(modernFixture)
	if err != nil {
		t.Fatal(err)
	}
	const id = "11111111-1111-4111-8111-111111111111"
	sessionPath := filepath.Join(projectDir, id+".jsonl")
	if err := os.WriteFile(sessionPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	index := `{"version":1,"entries":[{"sessionId":"` + id + `","fullPath":"` + sessionPath + `","firstPrompt":"hello","messageCount":14,"created":"2026-10-20T10:00:00Z","modified":"2026-10-20T10:00:05Z"}]}`
	if err := os.WriteFile(filepath.Join(projectDir, "sessions-index.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStoreWithCacheDir(base, filepath.Join(base, "cache"))
	meta, err := store.GetSessionMeta(context.Background(), sessionPath)
	if err != nil || meta == nil {
		t.Fatalf("meta %v err %v", meta, err)
	}
	if meta.Title != "Final title" || meta.EntryCount != modernFixtureEntries || meta.Cost == nil {
		t.Fatalf("legacy index hit skipped enrichment: %+v", meta)
	}
	if meta.FirstPrompt != "hello" || meta.Source != thinkt.SourceClaude {
		t.Fatalf("index fields lost: %+v", meta)
	}
}
