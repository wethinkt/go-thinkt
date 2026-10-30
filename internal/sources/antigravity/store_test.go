package antigravity

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// testStore returns a store over home with its own metadata cache dir. Its
// exclude list is just the home, because test repos live under the temp dir.
func testStore(t *testing.T, home, cacheDir string) *Store {
	t.Helper()
	s := NewStoreWithCacheDir(home, cacheDir)
	s.relatedExclude = []string{home}
	return s
}

func sessionIDs(sessions []thinkt.SessionMeta) []string {
	var ids []string
	for _, m := range sessions {
		ids = append(ids, m.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestListProjectsByFolder(t *testing.T) {
	s := testStore(t, testHome(t), t.TempDir())
	projects, err := s.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]thinkt.Project{}
	for _, p := range projects {
		byID[p.ID] = p
	}
	ds4dg, old := byID["/w/ds4dg"], byID["/w/old"]
	if len(projects) != 2 || ds4dg.SessionCount != 1 || ds4dg.SubagentCount != 1 || ds4dg.Name != "ds4dg" ||
		old.SessionCount != 1 || old.Source != thinkt.SourceAntigravity {
		t.Fatalf("%+v", projects)
	}
	if _, ok := byID["/w/ds4-go"]; ok {
		t.Fatal("second workspace folder became a project")
	}
}

func TestListSessionsHidesSubagentsByDefault(t *testing.T) {
	s := testStore(t, testHome(t), t.TempDir())
	ctx := context.Background()
	sessions, err := s.ListSessions(ctx, "/w/ds4dg")
	if err != nil || !slices.Equal(sessionIDs(sessions), []string{"11111111-aaaa"}) {
		t.Fatalf("%v %v", sessionIDs(sessions), err)
	}
	if sessions[0].SubagentCount != 1 {
		t.Fatalf("subagent count %d", sessions[0].SubagentCount)
	}
	all, _ := s.ListSessions(ctx, "/w/ds4dg", thinkt.WithSubagents())
	if !slices.Equal(sessionIDs(all), []string{"11111111-aaaa", "22222222-bbbb"}) {
		t.Fatalf("%v", sessionIDs(all))
	}
	for _, m := range all {
		if m.ID == "22222222-bbbb" && (m.ParentSessionID != "11111111-aaaa" || m.ProjectPath != "/w/ds4dg") {
			t.Fatalf("%+v", m)
		}
	}
}

func TestListSessionsTitleFallbacks(t *testing.T) {
	s := testStore(t, testHome(t), t.TempDir())
	ctx := context.Background()
	titles := map[string]string{}
	for _, p := range []string{"/w/ds4dg", "/w/old"} {
		sessions, err := s.ListSessions(ctx, p, thinkt.WithSubagents(), thinkt.WithEnrichSync())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range sessions {
			titles[m.ID] = m.Title
		}
	}
	want := map[string]string{"11111111-aaaa": "Fix Build", "22222222-bbbb": "Research Child", "44444444-dddd": "look around"}
	if !reflect.DeepEqual(titles, want) {
		t.Fatalf("%v", titles)
	}
}

func TestListSessionsWithoutSummaryDB(t *testing.T) {
	home := testHome(t)
	if err := os.Remove(filepath.Join(home, "conversation_summaries.db")); err != nil {
		t.Fatal(err)
	}
	s := testStore(t, home, t.TempDir())
	ctx := context.Background()
	projects, err := s.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range projects {
		sessions, err := s.ListSessions(ctx, p.ID, thinkt.WithSubagents(), thinkt.WithEnrichSync())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range sessions {
			ids = append(ids, m.ID)
			// The child loses its parent link without the db, so its task
			// prompt reads as an agent message and it has no prompt title.
			if m.Title == "" && m.ID != "22222222-bbbb" {
				t.Errorf("%s has no title", m.ID)
			}
		}
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"11111111-aaaa", "22222222-bbbb", "44444444-dddd"}) {
		t.Fatalf("%v", ids)
	}
}

// relatedHome builds a home whose main conversation works in repo A with
// repo B as a second workspace folder and as the target of its tool calls.
func relatedHome(t *testing.T) (home, repoA, repoB string) {
	t.Helper()
	root := t.TempDir()
	repoA, repoB = mkRepo(t, root, "ds4dg"), mkRepo(t, root, "ds4-go")
	home = filepath.Join(root, "antigravity-cli")
	writeTranscript(t, home, "11111111-aaaa", "main.jsonl", fullTranscript, repoB)
	writeSummariesDB(t, home, "file:///w/ds4dg", "file://"+repoA, "file:///w/ds4-go", "file://"+repoB)
	return home, repoA, repoB
}

func TestListSessionsRelatesExtraWorkspace(t *testing.T) {
	home, repoA, repoB := relatedHome(t)
	sessions, err := testStore(t, home, t.TempDir()).ListSessions(context.Background(), repoA, thinkt.WithEnrichSync())
	if err != nil || len(sessions) != 1 {
		t.Fatalf("%v %v", sessions, err)
	}
	rel := sessions[0].RelatedPaths
	if len(rel) != 1 || rel[0].Path != repoB || !slices.Contains(rel[0].Reasons, thinkt.RelatedWorkspace) ||
		!slices.Contains(rel[0].Reasons, thinkt.RelatedEdited) || rel[0].Count != 4 {
		t.Fatalf("%+v", rel)
	}
}

func TestListSessionsRelatedPathsSurviveCache(t *testing.T) {
	home, repoA, _ := relatedHome(t)
	cacheDir := t.TempDir()
	ctx := context.Background()
	first, err := testStore(t, home, cacheDir).ListSessions(ctx, repoA, thinkt.WithEnrichSync())
	if err != nil || len(first) != 1 || len(first[0].RelatedPaths) == 0 {
		t.Fatalf("%+v %v", first, err)
	}
	second, err := testStore(t, home, cacheDir).ListSessions(ctx, repoA, thinkt.WithEnrichSync())
	if err != nil || len(second) != 1 || !reflect.DeepEqual(first[0].RelatedPaths, second[0].RelatedPaths) {
		t.Fatalf("%+v vs %+v", first[0].RelatedPaths, second)
	}
}

func TestGetSessionMetaByIDAndPath(t *testing.T) {
	home := testHome(t)
	s := testStore(t, home, t.TempDir())
	ctx := context.Background()
	child, err := s.GetSessionMeta(ctx, "22222222-bbbb")
	if err != nil || child == nil || child.ParentSessionID != "11111111-aaaa" || child.FirstPrompt == "" {
		t.Fatalf("%+v %v", child, err)
	}
	byPath, err := s.GetSessionMeta(ctx, filepath.Join(logsDir(home, "11111111-aaaa"), fullTranscript))
	if err != nil || byPath == nil || byPath.ID != "11111111-aaaa" || byPath.EntryCount != 15 || byPath.Source != thinkt.SourceAntigravity {
		t.Fatalf("%+v %v", byPath, err)
	}
	if missing, err := s.GetSessionMeta(ctx, "nope"); missing != nil || err != nil {
		t.Fatalf("%+v %v", missing, err)
	}
}

func TestOpenSessionReadsFullTranscript(t *testing.T) {
	home := testHome(t)
	writeTranscript(t, home, "11111111-aaaa", "noworkspace.jsonl", plainTranscript, "") // different content
	s := testStore(t, home, t.TempDir())
	r, err := s.OpenSession(context.Background(), "11111111-aaaa")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	n := 0
	for {
		e, err := r.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if e.WorkspaceID == "" {
			t.Fatal("entry without workspace id")
		}
		n++
	}
	if n != 15 {
		t.Fatalf("read %d entries, want the full transcript's 15", n)
	}
}

func TestEmptyBrainDirsAreSkipped(t *testing.T) {
	home := testHome(t)
	if err := os.MkdirAll(filepath.Join(home, "brain", "55555555-eeee", "scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	sessions, err := testStore(t, home, t.TempDir()).loadAllSessions()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(sessionIDs(sessions), "55555555-eeee") || len(sessions) != 3 {
		t.Fatalf("%v", sessionIDs(sessions))
	}
}

func TestNoWorkspaceProjectIsAddressable(t *testing.T) {
	home := testHome(t)
	writeTranscript(t, home, "66666666-ffff", "child.jsonl", fullTranscript, "") // no row, no Cwd
	s := testStore(t, home, t.TempDir())
	ctx := context.Background()
	p, err := s.GetProject(ctx, noWorkspaceID)
	if err != nil || p == nil || p.ID != "no-workspace" || p.Path != "(no workspace)" || p.Name != "(no workspace)" || p.SessionCount != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	sessions, err := s.ListSessions(ctx, "no-workspace")
	if err != nil || !slices.Equal(sessionIDs(sessions), []string{"66666666-ffff"}) {
		t.Fatalf("%v %v", sessionIDs(sessions), err)
	}
}

func TestWorkspaceUsesInstallationID(t *testing.T) {
	home := testHome(t)
	if err := os.WriteFile(filepath.Join(home, "installation_id"), []byte("inst-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ws := testStore(t, home, t.TempDir()).Workspace(); ws.ID != "inst-1" || ws.Source != thinkt.SourceAntigravity || ws.BasePath != home {
		t.Fatalf("%+v", ws)
	}
}

func TestWatchConfigReachesLogs(t *testing.T) {
	cfg := (&Store{}).WatchConfig()
	if !slices.Equal(cfg.IncludeDirs, []string{"brain"}) || !slices.Equal(cfg.HiddenDirs, []string{".system_generated"}) ||
		!slices.Equal(cfg.ExcludeDirs, []string{"chunks"}) || cfg.MaxDepth != 4 {
		t.Fatalf("%+v", cfg)
	}
}

// Background results land in the transcript after the summary row was
// written: the later of the two times is when the conversation last changed.
func TestModifiedAtIsLaterOfSummaryAndFile(t *testing.T) {
	home := testHome(t)
	s := testStore(t, home, t.TempDir())
	sessions, err := s.loadAllSessions()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(logsDir(home, "11111111-aaaa"), fullTranscript))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range sessions {
		if m.ID == "11111111-aaaa" && (!m.ModifiedAt.Equal(info.ModTime()) || m.ModifiedAt.Location() != time.UTC) {
			t.Fatalf("ModifiedAt %v, file %v (summary row says 2026-10-28 20:40)", m.ModifiedAt, info.ModTime())
		}
	}
}

// A listing that changes nothing in the metadata cache leaves its file alone.
func TestListingDoesNotRewriteUnchangedCache(t *testing.T) {
	home, cacheDir := testHome(t), t.TempDir()
	if _, err := testStore(t, home, cacheDir).loadAllSessions(); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(cacheDir, "*"))
	if len(files) != 1 {
		t.Fatalf("cache files %v", files)
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(files[0], old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore(t, home, cacheDir).loadAllSessions(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(files[0]); err != nil || !info.ModTime().Equal(old) {
		t.Fatalf("cache rewritten: %v %v", info.ModTime(), err)
	}
}

func TestCommandCwd(t *testing.T) {
	if got := commandCwd(fixturePath(t, "noworkspace.jsonl"), "c"); got != "/w/old" {
		t.Fatalf("%q", got)
	}
	if got := commandCwd(fixturePath(t, "child.jsonl"), "c"); got != "" {
		t.Fatalf("%q", got)
	}
}
