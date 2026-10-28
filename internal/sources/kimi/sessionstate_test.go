package kimi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const stateV2 = `{"id":"session_p1","version":2,"cwd":"/proj","createdAt":1790891444054,"updatedAt":1790894174521,"archived":false,"agents":{"main":{"homedir":"/x/agents/main","type":"main"},"agent-0":{"homedir":"/x/agents/agent-0","type":"sub","parentAgentId":"main","labels":{"parentAgentId":"main","profileName":"explore"}}},"custom":{},"lastPrompt":"fix 3.","title":"we've advanced this project","titleKind":"replaceable","lastTurnReason":"completed","isCustomTitle":false}`

const stateOld = `{"createdAt":"2026-07-30T00:09:55.213Z","updatedAt":"2026-07-30T01:25:55.909Z","title":"look at what we are doing","isCustomTitle":false,"agents":{"main":{"homedir":"/x/agents/main","type":"main","parentAgentId":null}},"custom":{},"workDir":"/Users/evan/projects/camelkoons","lastPrompt":"please set up a server"}`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFixture(t *testing.T, name, dest string) {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dest, string(data))
}

func TestReadStateV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, path, stateV2)
	s, err := readState(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "session_p1" || s.Version != 2 || s.CWD != "/proj" || s.Title != "we've advanced this project" || s.LastPrompt != "fix 3." || s.LastTurnReason != "completed" || s.IsCustomTitle {
		t.Fatalf("state = %+v", s)
	}
	if !s.CreatedAt.Equal(time.UnixMilli(1790891444054)) || !s.UpdatedAt.Equal(time.UnixMilli(1790894174521)) {
		t.Fatalf("times = %v %v", s.CreatedAt, s.UpdatedAt)
	}
	if len(s.Agents) != 2 || s.Agents["agent-0"].Type != "sub" || s.Agents["agent-0"].ParentAgentID != "main" || s.Agents["agent-0"].Labels.ProfileName != "explore" || s.Agents["main"].Type != "main" {
		t.Fatalf("agents = %+v", s.Agents)
	}
}

func TestReadStateOlderShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, path, stateOld)
	s, err := readState(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339Nano, "2026-07-30T00:09:55.213Z")
	if s.ID != "" || s.Version != 0 || s.WorkDir != "/Users/evan/projects/camelkoons" || !s.CreatedAt.Equal(want) || s.Title != "look at what we are doing" {
		t.Fatalf("state = %+v", s)
	}
	if _, err := readState(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing state must error")
	}
}

func TestParseStateTime(t *testing.T) {
	if got := parseStateTime([]byte(`1790891444054`)); !got.Equal(time.UnixMilli(1790891444054)) {
		t.Fatalf("number = %v", got)
	}
	want, _ := time.Parse(time.RFC3339Nano, "2026-07-30T00:09:55.213Z")
	if got := parseStateTime([]byte(`"2026-07-30T00:09:55.213Z"`)); !got.Equal(want) {
		t.Fatalf("string = %v", got)
	}
	if got := parseStateTime([]byte(`"garbage"`)); !got.IsZero() {
		t.Fatalf("garbage = %v", got)
	}
	if got := parseStateTime(nil); !got.IsZero() {
		t.Fatalf("nil = %v", got)
	}
}

func TestScanSessionFileFixture15(t *testing.T) {
	s := scanSessionFile("testdata/wire-1.5.jsonl", "session_fix1")
	if s.EntryCount != 17 || s.FirstPrompt != "fix the build" || s.LastPrompt != "Review the code structure" || s.Model != "kimi-code/k3" {
		t.Fatalf("scan = %+v", s)
	}
	if s.Cost == nil || s.Cost.InputTokens != 150 || s.Cost.OutputTokens != 30 || s.Cost.CachedInputTokens != 40 || s.Cost.DurationMs != 9000 {
		t.Fatalf("cost = %+v", s.Cost)
	}
}

func TestScanSessionFileFixture10(t *testing.T) {
	s := scanSessionFile("testdata/wire-1.0.jsonl", "ses_fix0")
	if s.EntryCount != 5 || s.FirstPrompt != "hello legacy" || s.Cost != nil || s.Model != "" {
		t.Fatalf("scan = %+v", s)
	}
	if missing := scanSessionFile(filepath.Join(t.TempDir(), "nope.jsonl"), "x"); missing.EntryCount != 0 {
		t.Fatalf("missing = %+v", missing)
	}
}

func TestLoadSessionIndexAndWorkspaces(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "session_index.jsonl"),
		`{"sessionId":"ses_a","sessionDir":"/x/ses_a","workDir":"/wa"}`+"\n"+
			`not json`+"\n"+
			`{"sessionId":"session_b","sessionDir":"/x/session_b","workDir":"/wb"}`+"\n")
	writeFile(t, filepath.Join(base, "workspaces.json"), `{"version":1,"workspaces":{"wd_proj_abc":{"root":"/proj","name":"proj"},"wd_old_def":{"root":"/old","name":"old"}}}`)
	idx := loadSessionIndex(base)
	if len(idx) != 2 || idx["ses_a"] != "/wa" || idx["session_b"] != "/wb" {
		t.Fatalf("index = %v", idx)
	}
	ws := loadWorkspaces(base)
	if len(ws) != 2 || ws["wd_proj_abc"] != "/proj" || ws["wd_old_def"] != "/old" {
		t.Fatalf("workspaces = %v", ws)
	}
	empty := t.TempDir()
	if len(loadSessionIndex(empty)) != 0 || len(loadWorkspaces(empty)) != 0 {
		t.Fatal("missing files must yield empty maps")
	}
}

// sessionDirFixture writes one session directory with a main wire and the
// given subagent wires, returning the session directory path.
func sessionDirFixture(t *testing.T, base, wd, session, state string, agents ...string) string {
	t.Helper()
	dir := filepath.Join(base, "sessions", wd, session)
	writeFile(t, filepath.Join(dir, "state.json"), state)
	copyFixture(t, "wire-1.5.jsonl", filepath.Join(dir, "agents", "main", "wire.jsonl"))
	for _, a := range agents {
		copyFixture(t, "wire-1.4.jsonl", filepath.Join(dir, "agents", a, "wire.jsonl"))
	}
	return dir
}

func TestSessionsFromDirMainAndSubagents(t *testing.T) {
	base := t.TempDir()
	dir := sessionDirFixture(t, base, "wd_proj_abc", "session_p1", stateV2, "agent-0")
	sessions := sessionsFromDir(base, dir, "wd_proj_abc", "ws", nil, nil)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	main := sessions[0]
	if main.ID != "session_p1" || main.FullPath != filepath.Join(dir, "agents", "main", "wire.jsonl") || main.ProjectPath != "/proj" || main.Title != "we've advanced this project" || main.LastPrompt != "fix 3." || main.SubagentCount != 1 || main.ParentSessionID != "" || main.WorkspaceID != "ws" || main.ChunkCount != 1 {
		t.Fatalf("main = %+v", main)
	}
	if !main.CreatedAt.Equal(time.UnixMilli(1790891444054)) || !main.ModifiedAt.Equal(time.UnixMilli(1790894174521)) || main.FileSize == 0 {
		t.Fatalf("main times = %+v", main)
	}
	child := sessions[1]
	if child.ID != "session_p1/agent-0" || child.ParentSessionID != "session_p1" || child.Title != "explore agent-0" || child.ProjectPath != "/proj" || child.FullPath != filepath.Join(dir, "agents", "agent-0", "wire.jsonl") || child.ModifiedAt.IsZero() {
		t.Fatalf("child = %+v", child)
	}
}

func TestSessionsFromDirListsUnregisteredAgent(t *testing.T) {
	base := t.TempDir()
	dir := sessionDirFixture(t, base, "wd_proj_abc", "session_p1", stateV2, "agent-4")
	sessions := sessionsFromDir(base, dir, "wd_proj_abc", "ws", nil, nil)
	if len(sessions) != 2 || sessions[1].ID != "session_p1/agent-4" || sessions[1].Title != "agent-4" || sessions[0].SubagentCount != 1 {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestSessionsFromDirProjectPathFallbacks(t *testing.T) {
	base := t.TempDir()
	noPath := `{"id":"session_np","version":2,"createdAt":1,"updatedAt":2,"title":"t","agents":{"main":{"type":"main"}}}`
	dir := sessionDirFixture(t, base, "wd_proj_abc", "session_np", noPath)
	if s := sessionsFromDir(base, dir, "wd_proj_abc", "ws", map[string]string{"session_np": "/from-index"}, map[string]string{"wd_proj_abc": "/from-ws"}); s[0].ProjectPath != "/from-index" {
		t.Fatalf("index fallback = %+v", s[0])
	}
	if s := sessionsFromDir(base, dir, "wd_proj_abc", "ws", nil, map[string]string{"wd_proj_abc": "/from-ws"}); s[0].ProjectPath != "/from-ws" {
		t.Fatalf("workspace fallback = %+v", s[0])
	}
	if s := sessionsFromDir(base, dir, "wd_proj_abc", "ws", nil, nil); s[0].ProjectPath != "unknown" {
		t.Fatalf("unknown fallback = %+v", s[0])
	}
	old := sessionDirFixture(t, base, "wd_old_def", "ses_l1", stateOld)
	if s := sessionsFromDir(base, old, "wd_old_def", "ws", nil, nil); s[0].ProjectPath != "/Users/evan/projects/camelkoons" || s[0].ID != "ses_l1" {
		t.Fatalf("workDir = %+v", s[0])
	}
	noState := filepath.Join(base, "sessions", "wd_proj_abc", "session_nostate")
	copyFixture(t, "wire-1.0.jsonl", filepath.Join(noState, "agents", "main", "wire.jsonl"))
	if s := sessionsFromDir(base, noState, "wd_proj_abc", "ws", nil, nil); len(s) != 0 {
		t.Fatalf("no state.json must list nothing, got %+v", s)
	}
}
