package search_test

import (
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/indexer/db"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
)

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func seedProjects(t *testing.T, d *db.DB, sources ...string) {
	t.Helper()
	for _, src := range sources {
		_, err := d.Exec(
			`INSERT INTO projects (id, name, path, source) VALUES (?, ?, ?, ?)`,
			src+"-proj", "project-"+src, "/path/"+src, src,
		)
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Exec(
			`INSERT INTO sessions (id, project_id, path) VALUES (?, ?, ?)`,
			src+"-sess", src+"-proj", "/path/"+src+"/session.jsonl",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestFindCandidates_FilterSources(t *testing.T) {
	d := openDB(t)
	seedProjects(t, d, "claude", "kimi", "gemini")

	svc := search.NewService(d, nil)

	// No FilterSources — returns all three
	all, err := svc.FindCandidates(search.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(all))
	}

	// FilterSources = ["claude", "kimi"] — excludes gemini
	filtered, err := svc.FindCandidates(search.SearchOptions{
		FilterSources: []string{"claude", "kimi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(filtered))
	}
	for _, c := range filtered {
		if c.Source == "gemini" {
			t.Fatal("gemini should be filtered out")
		}
	}

	// FilterSources = [] — returns all (no filtering)
	noFilter, err := svc.FindCandidates(search.SearchOptions{
		FilterSources: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(noFilter) != 3 {
		t.Fatalf("expected 3 candidates with empty FilterSources, got %d", len(noFilter))
	}

	// Both FilterSource and FilterSources set — intersection behavior
	both, err := svc.FindCandidates(search.SearchOptions{
		FilterSource:  "claude",
		FilterSources: []string{"claude", "kimi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 1 {
		t.Fatalf("expected 1 candidate with both filters, got %d", len(both))
	}
	if both[0].Source != "claude" {
		t.Fatalf("expected claude, got %s", both[0].Source)
	}
}
