package thinkt

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestSourceAntigravityNames(t *testing.T) {
	s := SourceAntigravity
	if s.String() != "antigravity" || s.DisplayName() != "Antigravity CLI" ||
		s.Description() != "Antigravity CLI conversations (~/.gemini/antigravity-cli)" {
		t.Fatalf("got %q %q %q", s.String(), s.DisplayName(), s.Description())
	}
	if !slices.Contains(AllSources, SourceAntigravity) {
		t.Fatal("AllSources lacks antigravity")
	}
	if SourceKimi.Description() != "Kimi Code sessions (~/.kimi-code)" {
		t.Fatalf("kimi description %q", SourceKimi.Description())
	}
}

func TestMergeIntoCopiesRelatedPaths(t *testing.T) {
	mc, err := LoadMetadataCache(SourceAntigravity, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mod := time.Unix(100, 0)
	rel := []RelatedPath{{Path: "/r/ds4-go", Reasons: []string{RelatedEdited}, Count: 3}}
	mc.Set("/f", CachedSession{ModifiedAt: mod, FileSize: 7, RelatedPaths: rel})
	meta := SessionMeta{FullPath: "/f", ModifiedAt: mod, FileSize: 7}
	if !mc.MergeInto(&meta) || !reflect.DeepEqual(meta.RelatedPaths, rel) {
		t.Fatalf("got %+v", meta.RelatedPaths)
	}
}
