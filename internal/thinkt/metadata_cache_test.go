package thinkt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetadataCache_LookupMiss(t *testing.T) {
	mc := &MetadataCache{
		Sessions: make(map[string]CachedSession),
	}
	_, ok := mc.Lookup("/some/unknown/path", time.Now(), 1234)
	if ok {
		t.Fatal("expected lookup miss for unknown path")
	}
}

func TestMetadataCache_SetAndLookupHit(t *testing.T) {
	mc := &MetadataCache{
		Sessions: make(map[string]CachedSession),
	}
	now := time.Now().Truncate(time.Second)
	entry := CachedSession{
		FirstPrompt: "hello world",
		Model:       "claude-4",
		EntryCount:  42,
		GitBranch:   "main",
		ModifiedAt:  now,
		FileSize:    9876,
	}
	mc.Set("/sessions/abc.jsonl", entry)

	got, ok := mc.Lookup("/sessions/abc.jsonl", now, 9876)
	if !ok {
		t.Fatal("expected lookup hit")
	}
	if got.FirstPrompt != "hello world" {
		t.Errorf("FirstPrompt = %q, want %q", got.FirstPrompt, "hello world")
	}
	if got.Model != "claude-4" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-4")
	}
	if got.EntryCount != 42 {
		t.Errorf("EntryCount = %d, want %d", got.EntryCount, 42)
	}
	if got.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", got.GitBranch, "main")
	}
}

func TestMetadataCache_LookupStaleModifiedAt(t *testing.T) {
	mc := &MetadataCache{
		Sessions: make(map[string]CachedSession),
	}
	now := time.Now().Truncate(time.Second)
	mc.Set("/sessions/abc.jsonl", CachedSession{
		FirstPrompt: "hello",
		ModifiedAt:  now,
		FileSize:    100,
	})

	_, ok := mc.Lookup("/sessions/abc.jsonl", now.Add(time.Second), 100)
	if ok {
		t.Fatal("expected lookup miss when mtime differs")
	}
}

func TestMetadataCache_LookupStaleFileSize(t *testing.T) {
	mc := &MetadataCache{
		Sessions: make(map[string]CachedSession),
	}
	now := time.Now().Truncate(time.Second)
	mc.Set("/sessions/abc.jsonl", CachedSession{
		FirstPrompt: "hello",
		ModifiedAt:  now,
		FileSize:    100,
	})

	_, ok := mc.Lookup("/sessions/abc.jsonl", now, 200)
	if ok {
		t.Fatal("expected lookup miss when file size differs")
	}
}

func TestMetadataCache_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)

	mc := &MetadataCache{
		Version:  1,
		Source:   SourceClaude,
		Sessions: make(map[string]CachedSession),
		dir:      dir,
	}
	mc.Set("/sessions/abc.jsonl", CachedSession{
		FirstPrompt: "what is Go?",
		Model:       "claude-4",
		EntryCount:  10,
		GitBranch:   "feat-x",
		ModifiedAt:  now,
		FileSize:    5000,
	})

	if err := mc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadMetadataCache(SourceClaude, dir)
	if err != nil {
		t.Fatalf("LoadMetadataCache: %v", err)
	}

	got, ok := loaded.Lookup("/sessions/abc.jsonl", now, 5000)
	if !ok {
		t.Fatal("expected lookup hit after round-trip")
	}
	if got.FirstPrompt != "what is Go?" {
		t.Errorf("FirstPrompt = %q, want %q", got.FirstPrompt, "what is Go?")
	}
	if got.Model != "claude-4" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-4")
	}
	if got.EntryCount != 10 {
		t.Errorf("EntryCount = %d, want %d", got.EntryCount, 10)
	}
	if got.GitBranch != "feat-x" {
		t.Errorf("GitBranch = %q, want %q", got.GitBranch, "feat-x")
	}
	if loaded.Source != SourceClaude {
		t.Errorf("Source = %q, want %q", loaded.Source, SourceClaude)
	}
}

func TestLoadMetadataCache_MissingFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	mc, err := LoadMetadataCache(SourceKimi, dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(mc.Sessions) != 0 {
		t.Errorf("expected empty sessions map, got %d entries", len(mc.Sessions))
	}
	if mc.Source != SourceKimi {
		t.Errorf("Source = %q, want %q", mc.Source, SourceKimi)
	}
}

func TestLoadMetadataCache_CorruptFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, cacheFileName(SourceClaude))
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	mc, err := LoadMetadataCache(SourceClaude, dir)
	if err != nil {
		t.Fatalf("expected no error on corrupt file, got %v", err)
	}
	if len(mc.Sessions) != 0 {
		t.Errorf("expected empty sessions map, got %d entries", len(mc.Sessions))
	}
}

func TestMetadataCache_SaveMergesFromDisk(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)

	// Process A writes entry 1.
	cacheA := &MetadataCache{
		Version:  1,
		Source:   SourceClaude,
		Sessions: make(map[string]CachedSession),
		dir:      dir,
	}
	cacheA.Set("/sessions/one.jsonl", CachedSession{
		FirstPrompt: "prompt one",
		ModifiedAt:  now,
		FileSize:    100,
	})
	if err := cacheA.Save(); err != nil {
		t.Fatalf("cacheA.Save: %v", err)
	}

	// Process B loads fresh, writes entry 2. Its Save should merge entry 1 from disk.
	cacheB := &MetadataCache{
		Version:  1,
		Source:   SourceClaude,
		Sessions: make(map[string]CachedSession),
		dir:      dir,
	}
	cacheB.Set("/sessions/two.jsonl", CachedSession{
		FirstPrompt: "prompt two",
		ModifiedAt:  now,
		FileSize:    200,
	})
	if err := cacheB.Save(); err != nil {
		t.Fatalf("cacheB.Save: %v", err)
	}

	// Reload and verify both entries exist.
	merged, err := LoadMetadataCache(SourceClaude, dir)
	if err != nil {
		t.Fatalf("LoadMetadataCache: %v", err)
	}
	if _, ok := merged.Sessions["/sessions/one.jsonl"]; !ok {
		t.Error("missing entry from process A after merge")
	}
	if _, ok := merged.Sessions["/sessions/two.jsonl"]; !ok {
		t.Error("missing entry from process B after merge")
	}

	// Verify process B's entry wins if both wrote the same key.
	cacheC := &MetadataCache{
		Version:  1,
		Source:   SourceClaude,
		Sessions: make(map[string]CachedSession),
		dir:      dir,
	}
	cacheC.Set("/sessions/one.jsonl", CachedSession{
		FirstPrompt: "updated prompt one",
		ModifiedAt:  now.Add(time.Second),
		FileSize:    150,
	})
	if err := cacheC.Save(); err != nil {
		t.Fatalf("cacheC.Save: %v", err)
	}

	final, err := LoadMetadataCache(SourceClaude, dir)
	if err != nil {
		t.Fatalf("LoadMetadataCache: %v", err)
	}
	if got := final.Sessions["/sessions/one.jsonl"].FirstPrompt; got != "updated prompt one" {
		t.Errorf("expected updated prompt, got %q", got)
	}
}

func TestMetadataCache_MergeInto(t *testing.T) {
	mc := &MetadataCache{
		Sessions: make(map[string]CachedSession),
	}
	now := time.Now().Truncate(time.Second)
	mc.Set("/sessions/abc.jsonl", CachedSession{
		FirstPrompt: "cached prompt",
		Model:       "claude-4",
		EntryCount:  15,
		GitBranch:   "dev",
		ModifiedAt:  now,
		FileSize:    500,
	})

	meta := &SessionMeta{
		FullPath:   "/sessions/abc.jsonl",
		ModifiedAt: now,
		FileSize:   500,
		// Fields intentionally left empty to be filled by MergeInto.
	}

	ok := mc.MergeInto(meta)
	if !ok {
		t.Fatal("expected MergeInto to return true")
	}
	if meta.FirstPrompt != "cached prompt" {
		t.Errorf("FirstPrompt = %q, want %q", meta.FirstPrompt, "cached prompt")
	}
	if meta.Model != "claude-4" {
		t.Errorf("Model = %q, want %q", meta.Model, "claude-4")
	}
	if meta.EntryCount != 15 {
		t.Errorf("EntryCount = %d, want %d", meta.EntryCount, 15)
	}
	if meta.GitBranch != "dev" {
		t.Errorf("GitBranch = %q, want %q", meta.GitBranch, "dev")
	}

	// Verify it does NOT overwrite already-set fields.
	meta2 := &SessionMeta{
		FullPath:    "/sessions/abc.jsonl",
		ModifiedAt:  now,
		FileSize:    500,
		FirstPrompt: "already set",
		Model:       "existing-model",
	}
	mc.MergeInto(meta2)
	if meta2.FirstPrompt != "already set" {
		t.Errorf("FirstPrompt should not be overwritten, got %q", meta2.FirstPrompt)
	}
	if meta2.Model != "existing-model" {
		t.Errorf("Model should not be overwritten, got %q", meta2.Model)
	}
	// But empty fields should still be filled.
	if meta2.EntryCount != 15 {
		t.Errorf("EntryCount = %d, want %d", meta2.EntryCount, 15)
	}
	if meta2.GitBranch != "dev" {
		t.Errorf("GitBranch = %q, want %q", meta2.GitBranch, "dev")
	}
}

func TestMetadataCache_MergeIntoStale(t *testing.T) {
	mc := &MetadataCache{
		Sessions: make(map[string]CachedSession),
	}
	now := time.Now().Truncate(time.Second)
	mc.Set("/sessions/abc.jsonl", CachedSession{
		FirstPrompt: "cached",
		ModifiedAt:  now,
		FileSize:    500,
	})

	// Stale mtime.
	meta := &SessionMeta{
		FullPath:   "/sessions/abc.jsonl",
		ModifiedAt: now.Add(time.Second),
		FileSize:   500,
	}
	ok := mc.MergeInto(meta)
	if ok {
		t.Fatal("expected MergeInto to return false for stale mtime")
	}
	if meta.FirstPrompt != "" {
		t.Errorf("FirstPrompt should remain empty, got %q", meta.FirstPrompt)
	}

	// Stale size.
	meta2 := &SessionMeta{
		FullPath:   "/sessions/abc.jsonl",
		ModifiedAt: now,
		FileSize:   999,
	}
	ok = mc.MergeInto(meta2)
	if ok {
		t.Fatal("expected MergeInto to return false for stale size")
	}
}

func TestCacheFileName(t *testing.T) {
	got := cacheFileName(SourceClaude)
	want := "sessions-claude.json"
	if got != want {
		t.Errorf("cacheFileName(SourceClaude) = %q, want %q", got, want)
	}
}

func TestMetadataCache_SaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	mc := &MetadataCache{
		Version:  1,
		Source:   SourceClaude,
		Sessions: make(map[string]CachedSession),
		dir:      dir,
	}
	mc.Set("/sessions/x.jsonl", CachedSession{
		ModifiedAt: time.Now().Truncate(time.Second),
		FileSize:   10,
	})
	if err := mc.Save(); err != nil {
		t.Fatalf("Save should create dir: %v", err)
	}
	// Verify file exists.
	path := filepath.Join(dir, cacheFileName(SourceClaude))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	var check MetadataCache
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("invalid JSON written: %v", err)
	}
}
