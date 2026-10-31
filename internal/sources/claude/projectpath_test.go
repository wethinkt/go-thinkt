package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const pathSessionID = "11111111-2222-4333-8444-555555555555"

// pathHome lays out a store with one project stored in dir under an index
// naming original, or no index when original is empty.
func pathHome(t *testing.T, dir, original string) (home, projectDir, session string) {
	t.Helper()
	t.Setenv("THINKT_HOME", t.TempDir())
	home = filepath.Join(t.TempDir(), "claude")
	projectDir = filepath.Join(home, "projects", dir)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session = writeTempSession(t, projectDir, pathSessionID+".jsonl", `{"type":"user","uuid":"u1","cwd":"/w","message":{"role":"user","content":"hi"}}`+"\n")
	if original != "" {
		writeTempSession(t, projectDir, "sessions-index.json", `{"version":1,"originalPath":"`+original+`","entries":[]}`)
	}
	return home, projectDir, session
}

func TestSessionsCarryTheRealProjectPath(t *testing.T) {
	home, projectDir, session := pathHome(t, "-real-proj", "/real/proj")
	s := NewStore(home)
	ctx := context.Background()

	list, err := s.ListSessions(ctx, projectDir)
	if err != nil || len(list) != 1 || list[0].ProjectPath != "/real/proj" {
		t.Fatalf("%+v %v", list, err)
	}
	byPath, err := s.GetSessionMeta(ctx, session)
	if err != nil || byPath == nil || byPath.ProjectPath != "/real/proj" {
		t.Fatalf("%+v %v", byPath, err)
	}
	byID, err := s.GetSessionMeta(ctx, pathSessionID)
	if err != nil || byID == nil || byID.ProjectPath != "/real/proj" {
		t.Fatalf("%+v %v", byID, err)
	}
	loaded, err := s.LoadSession(ctx, session)
	if err != nil || loaded == nil || loaded.Meta.ProjectPath != "/real/proj" {
		t.Fatalf("%+v %v", loaded, err)
	}
	projects, err := s.ListProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].Path != list[0].ProjectPath {
		t.Fatalf("%+v %v", projects, err)
	}
}

func TestSessionsDecodeThePathWithoutAnIndex(t *testing.T) {
	home, projectDir, _ := pathHome(t, "-no-index-here", "")
	s := NewStore(home)
	list, err := s.ListSessions(context.Background(), projectDir)
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	if got, want := list[0].ProjectPath, projectPath(projectDir); got != want || got == projectDir {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A session's ProjectPath works as a project id: the TUI lists by it.
func TestListSessionsAcceptsTheProjectPath(t *testing.T) {
	home, _, _ := pathHome(t, "-real-proj", "/real/proj")
	list, err := NewStore(home).ListSessions(context.Background(), "/real/proj")
	if err != nil || len(list) != 1 || list[0].ID != pathSessionID {
		t.Fatalf("%+v %v", list, err)
	}
}

func TestResumeUsesTheProjectPath(t *testing.T) {
	_, projectDir, session := pathHome(t, "-real-proj", "/real/proj")
	got := resolveProjectPath(thinkt.SessionMeta{FullPath: session, ProjectPath: projectDir})
	if got != "/real/proj" {
		t.Fatalf("%q", got)
	}
}
