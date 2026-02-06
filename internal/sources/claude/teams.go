package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// teamConfig mirrors the JSON schema of ~/.claude/teams/{name}/config.json.
type teamConfig struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	CreatedAt     int64              `json:"createdAt"` // epoch ms
	LeadAgentID   string             `json:"leadAgentId"`
	LeadSessionID string             `json:"leadSessionId"`
	Members       []teamConfigMember `json:"members"`
}

type teamConfigMember struct {
	AgentID   string `json:"agentId"`
	Name      string `json:"name"`
	AgentType string `json:"agentType"`
	Model     string `json:"model"`
	JoinedAt  int64  `json:"joinedAt"` // epoch ms
	CWD       string `json:"cwd"`
	Color     string `json:"color"`
	Prompt    string `json:"prompt"`
}

// taskFile mirrors the JSON schema of ~/.claude/tasks/{name}/{id}.json.
type taskFile struct {
	ID          string          `json:"id"`
	Subject     string          `json:"subject"`
	Description string          `json:"description"`
	ActiveForm  string          `json:"activeForm"`
	Status      string          `json:"status"`
	Owner       string          `json:"owner"`
	Blocks      []string        `json:"blocks"`
	BlockedBy   []string        `json:"blockedBy"`
	Metadata    json.RawMessage `json:"metadata"`
}

// inboxMessage mirrors the JSON schema of inbox message entries.
type inboxMessage struct {
	From      string `json:"from"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"` // ISO 8601
	Color     string `json:"color"`
	Read      bool   `json:"read"`
}

// Package-level compiled regexps for subagent file parsing.
var (
	agentHashRe  = regexp.MustCompile(`agent-([a-f0-9]+)\.jsonl$`)
	teamPromptRe = regexp.MustCompile(`You are "([^"]+)" on team "([^"]+)"`)
)

// subagentInfo holds metadata extracted from a single subagent JSONL first line.
type subagentInfo struct {
	sessionID  string // from entry.SessionID
	slug       string // shared by all teammates in a session
	memberName string // extracted from prompt (e.g., "researcher")
	teamName   string // extracted from prompt (e.g., "my-team")
	hash       string // agent hash from filename
	path       string // full path to JSONL file
	cwd        string // working directory from entry
	timestamp  time.Time
}

// TeamStore implements thinkt.TeamStore for Claude Code teams.
type TeamStore struct {
	baseDir            string // ~/.claude
	historicalCache    []thinkt.Team
	historicalDone     bool
	historicalCachedAt time.Time
	cacheTTL           time.Duration
}

// NewTeamStore creates a new Claude team store.
func NewTeamStore(baseDir string) *TeamStore {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, ".claude")
	}
	return &TeamStore{baseDir: baseDir}
}

// SetCacheTTL sets the cache time-to-live for this team store.
func (ts *TeamStore) SetCacheTTL(d time.Duration) { ts.cacheTTL = d }

// Source returns the source type for this team store.
func (ts *TeamStore) Source() thinkt.Source {
	return thinkt.SourceClaude
}

// ListTeams returns all discovered teams (active from config + historical from subagent files).
func (ts *TeamStore) ListTeams(ctx context.Context) ([]thinkt.Team, error) {
	// 1. Load active teams from config
	var teams []thinkt.Team
	activeNames := make(map[string]bool)

	teamsDir := filepath.Join(ts.baseDir, "teams")
	entries, err := os.ReadDir(teamsDir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			team, err := ts.loadTeam(e.Name())
			if err != nil {
				continue
			}
			team.Status = thinkt.TeamStatusActive
			teams = append(teams, *team)
			activeNames[team.Name] = true
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read teams dir: %w", err)
	}

	// 2. Discover historical teams from subagent files
	historical := ts.discoverHistoricalTeams(activeNames)
	teams = append(teams, historical...)

	return teams, nil
}

// GetTeam returns a team by name with resolved member-to-session mappings.
// Falls back to historical teams when active config is not found.
func (ts *TeamStore) GetTeam(ctx context.Context, name string) (*thinkt.Team, error) {
	team, err := ts.loadTeam(name)
	if err != nil {
		// If config not found, search historical teams
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			historical := ts.discoverHistoricalTeams(nil)
			for i := range historical {
				if historical[i].Name == name {
					return &historical[i], nil
				}
			}
			return nil, fmt.Errorf("team %q not found", name)
		}
		return nil, err
	}

	team.Status = thinkt.TeamStatusActive
	// Resolve member SourceAgentIDs by scanning subagent files
	ts.resolveMembers(team)
	return team, nil
}

// GetTeamTasks returns the shared task board for a team.
func (ts *TeamStore) GetTeamTasks(ctx context.Context, teamName string) ([]thinkt.TeamTask, error) {
	tasksDir := filepath.Join(ts.baseDir, "tasks", teamName)
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tasks dir: %w", err)
	}

	var tasks []thinkt.TeamTask
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(tasksDir, e.Name()))
		if err != nil {
			continue
		}

		var tf taskFile
		if err := json.Unmarshal(data, &tf); err != nil {
			continue
		}

		// Check for internal task metadata
		isInternal := false
		if len(tf.Metadata) > 0 {
			var meta map[string]any
			if json.Unmarshal(tf.Metadata, &meta) == nil {
				if v, ok := meta["_internal"]; ok {
					if b, ok := v.(bool); ok {
						isInternal = b
					}
				}
			}
		}

		tasks = append(tasks, thinkt.TeamTask{
			ID:          tf.ID,
			Subject:     tf.Subject,
			Description: tf.Description,
			ActiveForm:  tf.ActiveForm,
			Status:      tf.Status,
			Owner:       tf.Owner,
			Blocks:      tf.Blocks,
			BlockedBy:   tf.BlockedBy,
			IsInternal:  isInternal,
		})
	}
	return tasks, nil
}

// GetTeamMessages returns inbox messages for a team member.
func (ts *TeamStore) GetTeamMessages(ctx context.Context, teamName, memberName string) ([]thinkt.TeamMessage, error) {
	inboxPath := filepath.Join(ts.baseDir, "teams", teamName, "inboxes", memberName+".json")
	data, err := os.ReadFile(inboxPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read inbox: %w", err)
	}

	var msgs []inboxMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, fmt.Errorf("parse inbox: %w", err)
	}

	result := make([]thinkt.TeamMessage, len(msgs))
	for i, m := range msgs {
		var ts time.Time
		if m.Timestamp != "" {
			ts, _ = time.Parse(time.RFC3339, m.Timestamp)
		}
		result[i] = thinkt.TeamMessage{
			From:      m.From,
			Text:      m.Text,
			Timestamp: ts,
			Color:     m.Color,
			Read:      m.Read,
		}
	}
	return result, nil
}

// loadTeam reads and parses a team config.
func (ts *TeamStore) loadTeam(name string) (*thinkt.Team, error) {
	configPath := filepath.Join(ts.baseDir, "teams", name, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read team config: %w", err)
	}

	var cfg teamConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse team config: %w", err)
	}

	members := make([]thinkt.TeamMember, len(cfg.Members))
	for i, m := range cfg.Members {
		members[i] = thinkt.TeamMember{
			Name:      m.Name,
			AgentID:   m.AgentID,
			AgentType: m.AgentType,
			Model:     m.Model,
			JoinedAt:  time.UnixMilli(m.JoinedAt),
			CWD:       m.CWD,
			Color:     m.Color,
		}
	}

	return &thinkt.Team{
		Name:          cfg.Name,
		Description:   cfg.Description,
		CreatedAt:     time.UnixMilli(cfg.CreatedAt),
		LeadAgentID:   cfg.LeadAgentID,
		LeadSessionID: cfg.LeadSessionID,
		Members:       members,
		Source:        thinkt.SourceClaude,
	}, nil
}

// resolveMembers correlates team config members to their subagent JSONL files
// by scanning the subagents directory and matching prompt content.
func (ts *TeamStore) resolveMembers(team *thinkt.Team) {
	// Find the subagents directory for this team's lead session
	subagentsDir := ts.findSubagentsDir(team.LeadSessionID)
	if subagentsDir == "" {
		return
	}

	// List all agent-*.jsonl files
	pattern := filepath.Join(subagentsDir, "agent-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	for _, path := range matches {
		m := agentHashRe.FindStringSubmatch(filepath.Base(path))
		if len(m) < 2 {
			continue
		}
		hash := m[1]

		// Read first line to get the prompt/first message
		memberName := ts.identifyMember(path, team.Members)
		if memberName == "" {
			continue
		}

		// Update the matching member
		for i := range team.Members {
			if team.Members[i].Name == memberName {
				team.Members[i].SourceAgentID = hash
				team.Members[i].SessionPath = path
				break
			}
		}
	}
}

// findSubagentsDir locates the subagents directory for a session.
func (ts *TeamStore) findSubagentsDir(sessionID string) string {
	projectsDir := filepath.Join(ts.baseDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(projectsDir, e.Name(), sessionID, "subagents")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// identifyMember reads the first entry of a subagent JSONL to identify which
// team member it belongs to, by matching the prompt content against member names.
func (ts *TeamStore) identifyMember(path string, members []thinkt.TeamMember) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	parser := NewParser(f)
	entry, err := parser.NextEntry()
	if err != nil || entry == nil {
		return ""
	}

	// Get the first message text
	var text string
	if msg := entry.GetUserMessage(); msg != nil {
		text = msg.Content.GetText()
	}
	if text == "" {
		return ""
	}

	// Check for teammate-message tag identifying the sender
	// Format: <teammate-message teammate_id="researcher" ...>
	// The teammate_id is who SENT the message (the lead), but the content
	// includes the task assignment which identifies the recipient.
	// The prompt content in the config should match.
	for _, m := range members {
		if m.AgentType == "team-lead" {
			continue // skip lead, they don't have subagent files typically
		}
		// Match by: "You are \"memberName\" on team" pattern in the prompt
		if strings.Contains(text, fmt.Sprintf("You are \"%s\" on team", m.Name)) {
			return m.Name
		}
		// Also try matching the config prompt directly (it's included in the first message)
		if strings.Contains(text, fmt.Sprintf(`"name":"%s"`, m.Name)) {
			return m.Name
		}
		// Match by subject in task_assignment JSON
		if strings.Contains(text, fmt.Sprintf(`"assignedBy":"%s"`, m.Name)) {
			return m.Name
		}
	}

	return ""
}

// scanSubagentFile reads the first entry of a subagent JSONL file and extracts
// team metadata. Returns nil for non-teammate files (explore subagents without slug).
func (ts *TeamStore) scanSubagentFile(path string) *subagentInfo {
	m := agentHashRe.FindStringSubmatch(filepath.Base(path))
	if len(m) < 2 {
		return nil
	}
	hash := m[1]

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	parser := NewParser(f)
	entry, err := parser.NextEntry()
	if err != nil || entry == nil {
		return nil
	}

	// Teammates have a slug; explore subagents don't
	if entry.Slug == "" {
		return nil
	}

	// Extract member name and team name from the first user message.
	// Real teammates start with <teammate-message ...> containing the assignment.
	var text string
	if msg := entry.GetUserMessage(); msg != nil {
		text = msg.Content.GetText()
	}
	if text == "" || !strings.HasPrefix(strings.TrimSpace(text), "<teammate-message") {
		return nil
	}

	promptMatches := teamPromptRe.FindStringSubmatch(text)
	if len(promptMatches) < 3 {
		return nil
	}

	var entryTime time.Time
	if entry.Timestamp != "" {
		entryTime, _ = time.Parse(time.RFC3339, entry.Timestamp)
	}

	return &subagentInfo{
		sessionID:  entry.SessionID,
		slug:       entry.Slug,
		memberName: promptMatches[1],
		teamName:   promptMatches[2],
		hash:       hash,
		path:       path,
		cwd:        entry.CWD,
		timestamp:  entryTime,
	}
}

// discoverHistoricalTeams walks all subagent files to find teams that no longer
// have active config (deleted teams). Results are cached after first call.
// If activeNames is non-nil, teams whose names appear in it are skipped.
func (ts *TeamStore) discoverHistoricalTeams(activeNames map[string]bool) []thinkt.Team {
	// Check if cache is still valid (not expired by TTL)
	stale := ts.historicalDone && ts.cacheTTL > 0 && time.Since(ts.historicalCachedAt) > ts.cacheTTL
	if stale {
		ts.historicalDone = false
		ts.historicalCache = nil
	}

	if ts.historicalDone {
		if len(activeNames) == 0 {
			return ts.historicalCache
		}
		var filtered []thinkt.Team
		for _, t := range ts.historicalCache {
			if !activeNames[t.Name] {
				filtered = append(filtered, t)
			}
		}
		return filtered
	}
	ts.historicalDone = true
	ts.historicalCachedAt = time.Now()

	projectsDir := filepath.Join(ts.baseDir, "projects")
	projectEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	// Group subagent info by (slug, teamName) — slug is the shared session identifier
	type groupKey struct{ slug, teamName string }
	groups := make(map[groupKey][]subagentInfo)

	for _, pe := range projectEntries {
		if !pe.IsDir() {
			continue
		}
		projectPath := filepath.Join(projectsDir, pe.Name())
		sessionEntries, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}
		for _, se := range sessionEntries {
			if !se.IsDir() {
				continue
			}
			subagentsDir := filepath.Join(projectPath, se.Name(), "subagents")
			agentFiles, err := filepath.Glob(filepath.Join(subagentsDir, "agent-*.jsonl"))
			if err != nil || len(agentFiles) == 0 {
				continue
			}
			for _, agentPath := range agentFiles {
				info := ts.scanSubagentFile(agentPath)
				if info == nil {
					continue
				}
				key := groupKey{slug: info.slug, teamName: info.teamName}
				groups[key] = append(groups[key], *info)
			}
		}
	}

	for key, infos := range groups {
		team := buildHistoricalTeam(key.slug, key.teamName, infos)
		ts.historicalCache = append(ts.historicalCache, team)
	}

	if len(activeNames) == 0 {
		return ts.historicalCache
	}
	var filtered []thinkt.Team
	for _, t := range ts.historicalCache {
		if !activeNames[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// buildHistoricalTeam constructs a Team from grouped subagent info.
func buildHistoricalTeam(_, teamName string, infos []subagentInfo) thinkt.Team {
	var earliest time.Time
	members := make([]thinkt.TeamMember, 0, len(infos))

	for _, info := range infos {
		if earliest.IsZero() || (!info.timestamp.IsZero() && info.timestamp.Before(earliest)) {
			earliest = info.timestamp
		}
		members = append(members, thinkt.TeamMember{
			Name:          info.memberName,
			SourceAgentID: info.hash,
			SessionPath:   info.path,
			CWD:           info.cwd,
			JoinedAt:      info.timestamp,
		})
	}

	var leadSessionID string
	if len(infos) > 0 {
		leadSessionID = infos[0].sessionID
	}

	return thinkt.Team{
		Name:          teamName,
		CreatedAt:     earliest,
		LeadSessionID: leadSessionID,
		Members:       members,
		Source:        thinkt.SourceClaude,
		Status:        thinkt.TeamStatusInactive,
	}
}
