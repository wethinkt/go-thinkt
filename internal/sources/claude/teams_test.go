package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestTeamStoreListTeams(t *testing.T) {
	// Create a temp directory structure mimicking ~/.claude/teams/
	tmpDir := t.TempDir()
	teamsDir := filepath.Join(tmpDir, "teams", "test-team")
	os.MkdirAll(teamsDir, 0755)

	config := teamConfig{
		Name:          "test-team",
		Description:   "A test team",
		CreatedAt:     time.Now().UnixMilli(),
		LeadAgentID:   "lead@test-team",
		LeadSessionID: "abc-123",
		Members: []teamConfigMember{
			{
				AgentID:   "lead@test-team",
				Name:      "lead",
				AgentType: "team-lead",
				Model:     "claude-opus-4-6",
				JoinedAt:  time.Now().UnixMilli(),
				CWD:       "/tmp/test",
			},
			{
				AgentID:   "worker@test-team",
				Name:      "worker",
				AgentType: "general-purpose",
				Model:     "claude-sonnet-4-5-20250929",
				JoinedAt:  time.Now().UnixMilli(),
				CWD:       "/tmp/test",
				Color:     "blue",
			},
		},
	}

	data, _ := json.Marshal(config)
	os.WriteFile(filepath.Join(teamsDir, "config.json"), data, 0644)

	ts := NewTeamStore(tmpDir)
	ctx := context.Background()

	teams, err := ts.ListTeams(ctx)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}

	team := teams[0]
	if team.Name != "test-team" {
		t.Errorf("expected team name 'test-team', got %q", team.Name)
	}
	if team.LeadAgentID != "lead@test-team" {
		t.Errorf("expected lead agent ID 'lead@test-team', got %q", team.LeadAgentID)
	}
	if len(team.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(team.Members))
	}
	if team.Source != thinkt.SourceClaude {
		t.Errorf("expected source 'claude', got %q", team.Source)
	}
	if team.Status != thinkt.TeamStatusActive {
		t.Errorf("expected status 'active', got %q", team.Status)
	}
}

func TestTeamStoreGetTeamTasks(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, "tasks", "test-team")
	os.MkdirAll(tasksDir, 0755)

	// Create task files
	task1 := taskFile{
		ID:          "1",
		Subject:     "Do something",
		Description: "A detailed description",
		ActiveForm:  "Doing something",
		Status:      "completed",
		Owner:       "worker",
		Blocks:      []string{"2"},
	}
	data1, _ := json.Marshal(task1)
	os.WriteFile(filepath.Join(tasksDir, "1.json"), data1, 0644)

	task2 := taskFile{
		ID:          "2",
		Subject:     "Do more",
		Status:      "pending",
		BlockedBy:   []string{"1"},
		Metadata:    json.RawMessage(`{"_internal": true}`),
	}
	data2, _ := json.Marshal(task2)
	os.WriteFile(filepath.Join(tasksDir, "2.json"), data2, 0644)

	ts := NewTeamStore(tmpDir)
	ctx := context.Background()

	tasks, err := ts.GetTeamTasks(ctx, "test-team")
	if err != nil {
		t.Fatalf("GetTeamTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Find internal task
	var internalTask *thinkt.TeamTask
	for i, task := range tasks {
		if task.IsInternal {
			internalTask = &tasks[i]
		}
	}
	if internalTask == nil {
		t.Error("expected to find an internal task")
	} else if internalTask.ID != "2" {
		t.Errorf("expected internal task ID '2', got %q", internalTask.ID)
	}
}

func TestTeamStoreGetTeamMessages(t *testing.T) {
	tmpDir := t.TempDir()
	inboxDir := filepath.Join(tmpDir, "teams", "test-team", "inboxes")
	os.MkdirAll(inboxDir, 0755)

	msgs := []inboxMessage{
		{
			From:      "worker",
			Text:      "Hello lead!",
			Timestamp: "2026-02-06T15:00:00.000Z",
			Color:     "blue",
			Read:      true,
		},
		{
			From:      "worker",
			Text:      `{"type":"task_completed","taskId":"1"}`,
			Timestamp: "2026-02-06T15:05:00.000Z",
			Color:     "blue",
			Read:      false,
		},
	}
	data, _ := json.Marshal(msgs)
	os.WriteFile(filepath.Join(inboxDir, "lead.json"), data, 0644)

	ts := NewTeamStore(tmpDir)
	ctx := context.Background()

	messages, err := ts.GetTeamMessages(ctx, "test-team", "lead")
	if err != nil {
		t.Fatalf("GetTeamMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].From != "worker" {
		t.Errorf("expected from 'worker', got %q", messages[0].From)
	}
	if messages[1].Read {
		t.Error("expected second message to be unread")
	}
}

func TestTeamStoreNoTeamsDir(t *testing.T) {
	tmpDir := t.TempDir()
	ts := NewTeamStore(tmpDir)
	ctx := context.Background()

	teams, err := ts.ListTeams(ctx)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 0 {
		t.Errorf("expected 0 teams, got %d", len(teams))
	}
}

// writeSubagentJSONL writes a minimal JSONL subagent file with the given first entry fields.
func writeSubagentJSONL(t *testing.T, path, slug, sessionID, cwd, promptText string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)

	entry := map[string]any{
		"type":      "user",
		"uuid":      "test-uuid",
		"sessionId": sessionID,
		"cwd":       cwd,
		"timestamp": "2026-02-06T10:00:00.000Z",
	}
	if slug != "" {
		entry["slug"] = slug
	}
	if promptText != "" {
		entry["message"] = map[string]any{
			"role":    "user",
			"content": promptText,
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write subagent: %v", err)
	}
}

func TestScanSubagentFile(t *testing.T) {
	tmpDir := t.TempDir()
	ts := NewTeamStore(tmpDir)

	t.Run("teammate with slug", func(t *testing.T) {
		path := filepath.Join(tmpDir, "agent-abc1234.jsonl")
		writeSubagentJSONL(t, path, "purring-mochi", "sess-1", "/tmp/project",
			"<teammate-message teammate_id=\"lead\" summary=\"Research\">\n"+
				`You are "researcher" on team "my-team". Your task is to research things.`+
				"\n</teammate-message>")

		info := ts.scanSubagentFile(path)
		if info == nil {
			t.Fatal("expected non-nil info for teammate subagent")
		}
		if info.memberName != "researcher" {
			t.Errorf("expected memberName 'researcher', got %q", info.memberName)
		}
		if info.teamName != "my-team" {
			t.Errorf("expected teamName 'my-team', got %q", info.teamName)
		}
		if info.slug != "purring-mochi" {
			t.Errorf("expected slug 'purring-mochi', got %q", info.slug)
		}
		if info.hash != "abc1234" {
			t.Errorf("expected hash 'abc1234', got %q", info.hash)
		}
		if info.sessionID != "sess-1" {
			t.Errorf("expected sessionID 'sess-1', got %q", info.sessionID)
		}
	})

	t.Run("explore without slug", func(t *testing.T) {
		path := filepath.Join(tmpDir, "agent-def5678.jsonl")
		writeSubagentJSONL(t, path, "", "sess-2", "/tmp/project",
			"Search for files matching pattern *.go")

		info := ts.scanSubagentFile(path)
		if info != nil {
			t.Error("expected nil info for explore subagent (no slug)")
		}
	})

	t.Run("teammate without team prompt", func(t *testing.T) {
		path := filepath.Join(tmpDir, "agent-ghi9012.jsonl")
		writeSubagentJSONL(t, path, "some-slug", "sess-3", "/tmp/project",
			"Just do something without teammate-message tag")

		info := ts.scanSubagentFile(path)
		if info != nil {
			t.Error("expected nil info for subagent without teammate-message tag")
		}
	})

	t.Run("slug with teammate-message but no team pattern", func(t *testing.T) {
		path := filepath.Join(tmpDir, "agent-jkl3456.jsonl")
		writeSubagentJSONL(t, path, "some-slug", "sess-4", "/tmp/project",
			"<teammate-message teammate_id=\"lead\" summary=\"Work\">\nDo some work.\n</teammate-message>")

		info := ts.scanSubagentFile(path)
		if info != nil {
			t.Error("expected nil info for teammate-message without team assignment pattern")
		}
	})
}

func TestDiscoverHistoricalTeams(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subagent files under projects/proj1/sess1/subagents/
	subDir := filepath.Join(tmpDir, "projects", "proj1", "sess1", "subagents")

	writeSubagentJSONL(t,
		filepath.Join(subDir, "agent-aaa1111.jsonl"),
		"my-slug", "sess1", "/tmp/project",
		"<teammate-message teammate_id=\"lead\" summary=\"Research\">\n"+
			`You are "researcher" on team "old-team". Research things.`+
			"\n</teammate-message>")

	writeSubagentJSONL(t,
		filepath.Join(subDir, "agent-bbb2222.jsonl"),
		"my-slug", "sess1", "/tmp/project",
		"<teammate-message teammate_id=\"lead\" summary=\"Code\">\n"+
			`You are "coder" on team "old-team". Write code.`+
			"\n</teammate-message>")

	// Explore subagent (should be ignored)
	writeSubagentJSONL(t,
		filepath.Join(subDir, "agent-ccc3333.jsonl"),
		"", "sess1", "/tmp/project",
		"Explore the codebase")

	ts := NewTeamStore(tmpDir)
	historical := ts.discoverHistoricalTeams(nil)

	if len(historical) != 1 {
		t.Fatalf("expected 1 historical team, got %d", len(historical))
	}

	team := historical[0]
	if team.Name != "old-team" {
		t.Errorf("expected team name 'old-team', got %q", team.Name)
	}
	if team.Status != thinkt.TeamStatusInactive {
		t.Errorf("expected status inactive, got %q", team.Status)
	}
	if len(team.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(team.Members))
	}

	// Verify members
	memberNames := make(map[string]bool)
	for _, m := range team.Members {
		memberNames[m.Name] = true
	}
	if !memberNames["researcher"] || !memberNames["coder"] {
		t.Errorf("expected members researcher and coder, got %v", memberNames)
	}
}

func TestListTeamsMerge(t *testing.T) {
	tmpDir := t.TempDir()

	// Create active team config
	teamsDir := filepath.Join(tmpDir, "teams", "my-team")
	os.MkdirAll(teamsDir, 0755)
	config := teamConfig{
		Name:          "my-team",
		CreatedAt:     time.Now().UnixMilli(),
		LeadAgentID:   "lead@my-team",
		LeadSessionID: "sess-active",
		Members: []teamConfigMember{
			{AgentID: "lead@my-team", Name: "lead", AgentType: "team-lead", JoinedAt: time.Now().UnixMilli()},
		},
	}
	data, _ := json.Marshal(config)
	os.WriteFile(filepath.Join(teamsDir, "config.json"), data, 0644)

	// Create historical subagent for same team name (should be deduped)
	subDir := filepath.Join(tmpDir, "projects", "proj1", "sess-old", "subagents")
	writeSubagentJSONL(t,
		filepath.Join(subDir, "agent-aaa1111.jsonl"),
		"slug1", "sess-old", "/tmp",
		"<teammate-message teammate_id=\"lead\" summary=\"Work\">\n"+
			`You are "worker" on team "my-team". Do work.`+
			"\n</teammate-message>")

	// Create historical subagent for different team name (should appear)
	subDir2 := filepath.Join(tmpDir, "projects", "proj1", "sess-old2", "subagents")
	writeSubagentJSONL(t,
		filepath.Join(subDir2, "agent-ddd4444.jsonl"),
		"slug2", "sess-old2", "/tmp",
		"<teammate-message teammate_id=\"lead\" summary=\"Help\">\n"+
			`You are "helper" on team "other-team". Help out.`+
			"\n</teammate-message>")

	ts := NewTeamStore(tmpDir)
	ctx := context.Background()

	teams, err := ts.ListTeams(ctx)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}

	if len(teams) != 2 {
		t.Fatalf("expected 2 teams (1 active + 1 historical), got %d", len(teams))
	}

	// Check that my-team is active (from config, not historical)
	var myTeam, otherTeam *thinkt.Team
	for i := range teams {
		switch teams[i].Name {
		case "my-team":
			myTeam = &teams[i]
		case "other-team":
			otherTeam = &teams[i]
		}
	}

	if myTeam == nil {
		t.Fatal("expected to find 'my-team'")
	}
	if myTeam.Status != thinkt.TeamStatusActive {
		t.Errorf("expected 'my-team' to be active, got %q", myTeam.Status)
	}

	if otherTeam == nil {
		t.Fatal("expected to find 'other-team'")
	}
	if otherTeam.Status != thinkt.TeamStatusInactive {
		t.Errorf("expected 'other-team' to be inactive, got %q", otherTeam.Status)
	}
}

func TestListTeamsHistoricalOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// No teams config dir, only subagent files
	subDir := filepath.Join(tmpDir, "projects", "proj1", "sess1", "subagents")
	writeSubagentJSONL(t,
		filepath.Join(subDir, "agent-fff6666.jsonl"),
		"old-slug", "sess1", "/tmp",
		"<teammate-message teammate_id=\"lead\" summary=\"Analyze\">\n"+
			`You are "analyst" on team "deleted-team". Analyze things.`+
			"\n</teammate-message>")

	ts := NewTeamStore(tmpDir)
	ctx := context.Background()

	teams, err := ts.ListTeams(ctx)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if teams[0].Name != "deleted-team" {
		t.Errorf("expected team name 'deleted-team', got %q", teams[0].Name)
	}
	if teams[0].Status != thinkt.TeamStatusInactive {
		t.Errorf("expected status inactive, got %q", teams[0].Status)
	}
}
