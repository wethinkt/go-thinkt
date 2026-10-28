package index

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/index/db"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// listOnlyStore lists a fixed set of sessions for one project and never
// opens them; it stands in for a source whose listing no longer includes a
// rollout the index indexed earlier.
type listOnlyStore struct {
	project  thinkt.Project
	sessions []thinkt.SessionMeta
}

func (s *listOnlyStore) Source() thinkt.Source { return thinkt.SourceCodex }
func (s *listOnlyStore) Workspace() thinkt.Workspace {
	return thinkt.Workspace{ID: "ws", Source: thinkt.SourceCodex}
}
func (s *listOnlyStore) ListProjects(ctx context.Context) ([]thinkt.Project, error) {
	return []thinkt.Project{s.project}, nil
}
func (s *listOnlyStore) GetProject(ctx context.Context, id string) (*thinkt.Project, error) {
	p := s.project
	return &p, nil
}
func (s *listOnlyStore) ListSessions(ctx context.Context, projectID string, _ ...thinkt.ListSessionsOption) ([]thinkt.SessionMeta, error) {
	return s.sessions, nil
}
func (s *listOnlyStore) GetSessionMeta(ctx context.Context, id string) (*thinkt.SessionMeta, error) {
	return nil, nil
}
func (s *listOnlyStore) LoadSession(ctx context.Context, id string) (*thinkt.Session, error) {
	return nil, nil
}
func (s *listOnlyStore) OpenSession(ctx context.Context, id string) (thinkt.SessionReader, error) {
	return &emptyReader{}, nil
}
func (s *listOnlyStore) WatchConfig() thinkt.WatchConfig { return thinkt.DefaultWatchConfig() }

type emptyReader struct{}

func (r *emptyReader) ReadNext() (*thinkt.Entry, error) { return nil, io.EOF }
func (r *emptyReader) Metadata() thinkt.SessionMeta     { return thinkt.SessionMeta{} }
func (r *emptyReader) Close() error                     { return nil }

func TestIngestProjectPrunesUnlistedSessions(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	project := thinkt.Project{ID: "/proj", Path: "/proj", Name: "proj", Source: thinkt.SourceCodex}
	scoped := db.ScopedProjectID(project.Source, project.ID)
	stalePath := "/proj/sessions/rollout-child.jsonl"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO projects (id, path, name, source) VALUES (?, ?, ?, ?)`, []any{scoped, project.Path, project.Name, "codex"}},
		{`INSERT INTO sessions (id, project_id, path, entry_count) VALUES (?, ?, ?, ?)`, []any{"child", scoped, stalePath, 2}},
		{`INSERT INTO entries (session_id, uuid, role) VALUES (?, ?, ?)`, []any{"child", "u1", "user"}},
		{`INSERT INTO entries (session_id, uuid, role) VALUES (?, ?, ?)`, []any{"child", "u2", "assistant"}},
		{`INSERT INTO sync_state (file_path, last_mod_time, file_size, lines_read, parser_version) VALUES (?, ?, ?, ?, ?)`, []any{stalePath, 1, 1, 2, thinkt.ParserVersion - 1}},
	} {
		if _, err := database.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}

	registry := thinkt.NewRegistry()
	registry.Register(&listOnlyStore{project: project})
	ing := NewIngester(database, registry)
	if err := ing.IngestProject(ctx, project, 1, 1); err != nil {
		t.Fatalf("IngestProject: %v", err)
	}

	var sessions, entries, states int
	_ = database.QueryRow(`SELECT count(*) FROM sessions WHERE project_id = ?`, scoped).Scan(&sessions)
	_ = database.QueryRow(`SELECT count(*) FROM entries WHERE session_id = ?`, "child").Scan(&entries)
	_ = database.QueryRow(`SELECT count(*) FROM sync_state WHERE file_path = ?`, stalePath).Scan(&states)
	if sessions != 0 || entries != 0 || states != 0 {
		t.Fatalf("stale rows remain: sessions=%d entries=%d sync_state=%d", sessions, entries, states)
	}
}

func TestPruneUnlistedProjectsRemovesStaleProjectsOfRegisteredSources(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	stale := db.ScopedProjectID(thinkt.SourceCodex, "/old")
	kept := db.ScopedProjectID(thinkt.SourceCodex, "/proj")
	other := db.ScopedProjectID(thinkt.SourceKimi, "oldhash")
	stalePath := "/old/sessions/rollout-s.jsonl"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO projects (id, path, name, source) VALUES (?, ?, ?, ?)`, []any{stale, "/old", "old", "codex"}},
		{`INSERT INTO projects (id, path, name, source) VALUES (?, ?, ?, ?)`, []any{kept, "/proj", "proj", "codex"}},
		{`INSERT INTO projects (id, path, name, source) VALUES (?, ?, ?, ?)`, []any{other, "oldhash", "oldhash", "kimi"}},
		{`INSERT INTO sessions (id, project_id, path, entry_count) VALUES (?, ?, ?, ?)`, []any{"s", stale, stalePath, 2}},
		{`INSERT INTO entries (session_id, uuid, role) VALUES (?, ?, ?)`, []any{"s", "u1", "user"}},
		{`INSERT INTO entries (session_id, uuid, role) VALUES (?, ?, ?)`, []any{"s", "u2", "assistant"}},
		{`INSERT INTO sync_state (file_path, last_mod_time, file_size, lines_read, parser_version) VALUES (?, ?, ?, ?, ?)`, []any{stalePath, 1, 1, 2, 1}},
	} {
		if _, err := database.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}

	project := thinkt.Project{ID: "/proj", Path: "/proj", Name: "proj", Source: thinkt.SourceCodex}
	registry := thinkt.NewRegistry()
	registry.Register(&listOnlyStore{project: project})
	ing := NewIngester(database, registry)
	if err := ing.PruneUnlistedProjects(ctx, []thinkt.Project{project}); err != nil {
		t.Fatalf("PruneUnlistedProjects: %v", err)
	}

	count := func(sql string, arg any) int {
		var n int
		if err := database.QueryRow(sql, arg).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM projects WHERE id = ?`, stale); n != 0 {
		t.Fatalf("stale project remains (%d)", n)
	}
	if n := count(`SELECT count(*) FROM sessions WHERE project_id = ?`, stale); n != 0 {
		t.Fatalf("stale sessions remain (%d)", n)
	}
	if n := count(`SELECT count(*) FROM entries WHERE session_id = ?`, "s"); n != 0 {
		t.Fatalf("stale entries remain (%d)", n)
	}
	if n := count(`SELECT count(*) FROM sync_state WHERE file_path = ?`, stalePath); n != 0 {
		t.Fatalf("stale sync_state remains (%d)", n)
	}
	if n := count(`SELECT count(*) FROM projects WHERE id = ?`, kept); n != 1 {
		t.Fatalf("listed project removed")
	}
	if n := count(`SELECT count(*) FROM projects WHERE id = ?`, other); n != 1 {
		t.Fatalf("project of an unregistered source removed")
	}
}
