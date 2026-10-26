package thinkt

import (
	"testing"
	"time"
)

func TestMetadataCacheParserVersion(t *testing.T) {
	mc := &MetadataCache{Sessions: make(map[string]CachedSession)}
	now := time.Now().Truncate(time.Second)
	mc.Sessions["/s/old.jsonl"] = CachedSession{FirstPrompt: "old", ModifiedAt: now, FileSize: 10, ParserVersion: 1}
	if _, ok := mc.Lookup("/s/old.jsonl", now, 10); ok {
		t.Fatal("entry from an older parser version must be stale")
	}
	meta := SessionMeta{FullPath: "/s/old.jsonl", ModifiedAt: now, FileSize: 10}
	if mc.MergeInto(&meta) || meta.FirstPrompt != "" {
		t.Fatal("MergeInto must not use a stale parser version")
	}
	mc.Set("/s/new.jsonl", CachedSession{FirstPrompt: "new", ModifiedAt: now, FileSize: 10})
	got, ok := mc.Lookup("/s/new.jsonl", now, 10)
	if !ok || got.ParserVersion != ParserVersion {
		t.Fatalf("Set must stamp ParserVersion: ok=%v entry=%+v", ok, got)
	}
}

func TestMergeIntoTitleFallsBackToSummary(t *testing.T) {
	mc := &MetadataCache{Sessions: make(map[string]CachedSession)}
	now := time.Now().Truncate(time.Second)
	mc.Set("/s/a.jsonl", CachedSession{Title: "Title A", LastPrompt: "last", SubagentCount: 2, ModifiedAt: now, FileSize: 10})
	meta := SessionMeta{FullPath: "/s/a.jsonl", ModifiedAt: now, FileSize: 10}
	if !mc.MergeInto(&meta) {
		t.Fatal("expected merge")
	}
	if meta.Title != "Title A" || meta.Summary != "Title A" || meta.LastPrompt != "last" || meta.SubagentCount != 2 {
		t.Fatalf("merged %+v", meta)
	}
	keep := SessionMeta{FullPath: "/s/a.jsonl", ModifiedAt: now, FileSize: 10, Summary: "existing"}
	mc.MergeInto(&keep)
	if keep.Summary != "existing" || keep.Title != "Title A" {
		t.Fatalf("existing summary must be kept: %+v", keep)
	}
}
