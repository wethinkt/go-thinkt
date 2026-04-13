package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	indexdb "github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// openEmptyIndexDB creates a temporary SQLite index database with schema but no data.
func openEmptyIndexDB(t *testing.T) *indexdb.DB {
	t.Helper()
	db, err := indexdb.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("failed to open empty index db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestListProjects_EmptySQLiteFallsBackToRegistry(t *testing.T) {
	idb := openEmptyIndexDB(t)

	registry := thinkt.NewRegistry()
	registry.Register(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "proj-1", Name: "my-project", Path: t.TempDir(), Source: thinkt.SourceClaude, LastModified: time.Now()},
		},
	})

	result, err := listProjects(context.Background(), idb, registry, "", false, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project from registry fallback, got %d", len(result.Projects))
	}
	if result.Projects[0].ID != "proj-1" {
		t.Fatalf("expected project proj-1, got %s", result.Projects[0].ID)
	}
}

func TestListSessions_EmptySQLiteFallsBackToRegistry(t *testing.T) {
	idb := openEmptyIndexDB(t)

	registry := thinkt.NewRegistry()
	registry.Register(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "proj-1", Name: "my-project", Path: t.TempDir(), Source: thinkt.SourceClaude},
		},
		sessions: map[string][]thinkt.SessionMeta{
			"proj-1": {
				{ID: "sess-1", FullPath: "/tmp/sess-1.jsonl", Source: thinkt.SourceClaude, ModifiedAt: time.Now()},
			},
		},
	})

	result, err := listSessions(context.Background(), idb, registry, thinkt.SourceClaude, "proj-1", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session from registry fallback, got %d", len(result.Sessions))
	}
	if result.Sessions[0].ID != "sess-1" {
		t.Fatalf("expected session sess-1, got %s", result.Sessions[0].ID)
	}
}

func TestListProjects_NilDBUsesRegistry(t *testing.T) {
	registry := thinkt.NewRegistry()
	registry.Register(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "proj-1", Name: "my-project", Path: t.TempDir(), Source: thinkt.SourceClaude, LastModified: time.Now()},
		},
	})

	result, err := listProjects(context.Background(), nil, registry, "", false, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}
}

func TestListSessions_NilDBUsesRegistry(t *testing.T) {
	registry := thinkt.NewRegistry()
	registry.Register(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "proj-1", Name: "my-project", Path: t.TempDir(), Source: thinkt.SourceClaude},
		},
		sessions: map[string][]thinkt.SessionMeta{
			"proj-1": {
				{ID: "sess-1", FullPath: "/tmp/sess-1.jsonl", Source: thinkt.SourceClaude, ModifiedAt: time.Now()},
			},
		},
	})

	result, err := listSessions(context.Background(), nil, registry, thinkt.SourceClaude, "proj-1", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
}
