package antigravity

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// fixturePath returns the path of a testdata fixture.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

// writeTranscript writes a fixture as a conversation's transcript under home,
// with __REPO_B__ replaced by repoB.
func writeTranscript(t *testing.T, home, id, fixture, name, repoB string) string {
	t.Helper()
	dir := logsDir(home, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	data := bytes.ReplaceAll(fixtureBytes(t, fixture), []byte("__REPO_B__"), []byte(repoB))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeSummariesDB builds conversation_summaries.db under home from
// testdata/summaries.sql, with each "/w/..." workspace URI rewritten by
// replacements (old → new).
func writeSummariesDB(t *testing.T, home string, replacements ...string) {
	t.Helper()
	script := string(fixtureBytes(t, "summaries.sql"))
	for i := 0; i+1 < len(replacements); i += 2 {
		script = string(bytes.ReplaceAll([]byte(script), []byte(replacements[i]), []byte(replacements[i+1])))
	}
	db, err := sql.Open("sqlite3", filepath.Join(home, "conversation_summaries.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range bytes.Split([]byte(script), []byte(";\n")) {
		if s := bytes.TrimSpace(stmt); len(s) > 0 {
			if _, err := db.Exec(string(s)); err != nil {
				t.Fatalf("%v: %s", err, s)
			}
		}
	}
}

// testHome builds a temp Antigravity home with the three fixture
// conversations and the summaries db.
func testHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "antigravity-cli")
	writeTranscript(t, home, "11111111-aaaa", "main.jsonl", fullTranscript, "/w/ds4-go")
	writeTranscript(t, home, "22222222-bbbb", "child.jsonl", fullTranscript, "")
	writeTranscript(t, home, "44444444-dddd", "noworkspace.jsonl", plainTranscript, "")
	writeSummariesDB(t, home)
	return home
}

func TestLoadSummaries(t *testing.T) {
	home := testHome(t)
	rows, err := loadSummaries(home)
	r := rows["11111111-aaaa"]
	if err != nil || r.Title != "Fix Build" || !slices.Equal(r.Workspaces, []string{"/w/ds4dg", "/w/ds4-go"}) ||
		rows["22222222-bbbb"].ParentID != "11111111-aaaa" || rows["22222222-bbbb"].Preview != "Research Child" ||
		rows["22222222-bbbb"].AgentName != "research" || !rows["44444444-dddd"].Modified.IsZero() ||
		r.Modified.Year() != 2026 || r.StepCount != 16 || len(rows["44444444-dddd"].Workspaces) != 0 {
		t.Fatalf("%+v %v", rows, err)
	}
}

func TestLoadSummariesMissingDB(t *testing.T) {
	home := t.TempDir()
	rows, err := loadSummaries(home)
	if err == nil || len(rows) != 0 {
		t.Fatalf("%v %v", rows, err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "conversation_summaries.db")); statErr == nil {
		t.Fatal("loadSummaries created the db")
	}
}

func TestLoadLastConversations(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "cache", "last_conversations.json"), []byte(`{"/w/booba":"c9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadLastConversations(home); got["c9"] != "/w/booba" {
		t.Fatalf("%v", got)
	}
	if got := loadLastConversations(t.TempDir()); len(got) != 0 {
		t.Fatalf("missing file: %v", got)
	}
}

func TestWorkspaceFirstEntryWins(t *testing.T) {
	if got := resolveWorkspace(summaryRow{Workspaces: []string{"/w/ds4dg", "/w/ds4-go"}}, "/w/x", nil, "c"); got != "/w/ds4dg" {
		t.Fatal(got)
	}
}

func TestWorkspaceFallsBackToCwd(t *testing.T) {
	s := scanSessionFile(fixturePath(t, "noworkspace.jsonl"), "44444444-dddd", "", "", nil, nil)
	if s.Cwd != "/w/old" || resolveWorkspace(summaryRow{}, s.Cwd, nil, "44444444-dddd") != "/w/old" {
		t.Fatalf("%q", s.Cwd)
	}
}

func TestWorkspaceFallsBackToLastConversations(t *testing.T) {
	if got := resolveWorkspace(summaryRow{}, "", map[string]string{"c9": "/w/booba"}, "c9"); got != "/w/booba" {
		t.Fatal(got)
	}
	if got := resolveWorkspace(summaryRow{}, "", nil, "c9"); got != "" {
		t.Fatal(got)
	}
}

func TestScanSessionFileSummarises(t *testing.T) {
	s := scanSessionFile(fixturePath(t, "main.jsonl"), "11111111-aaaa", "", "/w/ds4dg", nil, nil)
	if s.EntryCount != 15 || s.FirstPrompt != "fix the build" || s.LastPrompt != "thanks" ||
		s.Model != "Gemini 4 Argon (High)" || s.Cost == nil || s.Cost.InputTokens != 100 || s.CreatedAt.IsZero() {
		t.Fatalf("%+v", s)
	}
}

func TestScanSessionFileRelatesExtraWorkspaces(t *testing.T) {
	root := t.TempDir()
	proj, other := mkRepo(t, root, "ds4dg"), mkRepo(t, root, "ds4-go")
	s := scanSessionFile(fixturePath(t, "noworkspace.jsonl"), "c", "", proj, []string{other}, nil)
	if len(s.Related) != 1 || s.Related[0].Path != other || !slices.Contains(s.Related[0].Reasons, "workspace") {
		t.Fatalf("%+v", s.Related)
	}
}

func TestParseSummaryTime(t *testing.T) {
	if got := parseSummaryTime("2026-06-10 14:02:22.02715+00:00"); got.Year() != 2026 || got.Nanosecond() != 27150000 {
		t.Fatal(got)
	}
	if got := parseSummaryTime("2026-05-20 21:18:14+00:00"); !got.Equal(time.Date(2026, 5, 20, 21, 18, 14, 0, time.UTC)) {
		t.Fatal(got)
	}
	if !parseSummaryTime("0001-01-01 00:00:00+00:00").IsZero() || !parseSummaryTime("junk").IsZero() {
		t.Fatal("zero")
	}
}
