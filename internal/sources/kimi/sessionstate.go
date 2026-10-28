package kimi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// stateAgent is one entry of state.json's agents map.
type stateAgent struct {
	Type          string `json:"type"`
	ParentAgentID string `json:"parentAgentId"`
	Labels        struct {
		ProfileName string `json:"profileName"`
	} `json:"labels"`
}

// stateFile is a session directory's state.json. Older files carry ISO
// times and workDir; v2 files carry an id, epoch-ms times, and cwd.
type stateFile struct {
	ID             string
	Version        int
	CWD            string
	WorkDir        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Title          string
	IsCustomTitle  bool
	LastPrompt     string
	LastTurnReason string
	Agents         map[string]stateAgent
}

type rawState struct {
	ID             string                `json:"id"`
	Version        int                   `json:"version"`
	CWD            string                `json:"cwd"`
	WorkDir        string                `json:"workDir"`
	CreatedAt      json.RawMessage       `json:"createdAt"`
	UpdatedAt      json.RawMessage       `json:"updatedAt"`
	Title          string                `json:"title"`
	IsCustomTitle  bool                  `json:"isCustomTitle"`
	LastPrompt     string                `json:"lastPrompt"`
	LastTurnReason string                `json:"lastTurnReason"`
	Agents         map[string]stateAgent `json:"agents"`
}

// readState decodes a state.json file.
func readState(path string) (stateFile, error) {
	var s stateFile
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	var r rawState
	if err := json.Unmarshal(data, &r); err != nil {
		return s, err
	}
	s = stateFile{
		ID: r.ID, Version: r.Version, CWD: r.CWD, WorkDir: r.WorkDir,
		CreatedAt: parseStateTime(r.CreatedAt), UpdatedAt: parseStateTime(r.UpdatedAt),
		Title: r.Title, IsCustomTitle: r.IsCustomTitle, LastPrompt: r.LastPrompt,
		LastTurnReason: r.LastTurnReason, Agents: r.Agents,
	}
	return s, nil
}

// parseStateTime reads an epoch-ms number or an RFC3339 string; zero otherwise.
func parseStateTime(raw json.RawMessage) time.Time {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return time.Time{}
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		return time.Time{}
	}
	var ms int64
	if json.Unmarshal(raw, &ms) != nil || ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// sessionScan is what one parser pass over a wire file yields for SessionMeta.
type sessionScan struct {
	FirstPrompt string
	LastPrompt  string
	Model       string
	EntryCount  int
	Cost        *thinkt.SessionCost
}

// scanSessionFile parses a wire file to EOF and summarises it. Wire files
// are small, so running the parser keeps the count exact by construction.
func scanSessionFile(path, sessionID string) sessionScan {
	var s sessionScan
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer func() { _ = f.Close() }()
	p := NewParser(f, sessionID)
	var cost thinkt.SessionCost
	for {
		e, err := p.NextEntry()
		if err != nil {
			break
		}
		s.EntryCount++
		if e.Model != "" {
			s.Model = e.Model
		}
		if e.Role == thinkt.RoleUser && e.Kind == thinkt.KindNone && e.Text != "" {
			if s.FirstPrompt == "" {
				s.FirstPrompt = e.Text
			}
			s.LastPrompt = e.Text
		}
		if e.Role == thinkt.RoleAssistant && e.Usage != nil {
			cost.InputTokens += e.Usage.InputTokens
			cost.OutputTokens += e.Usage.OutputTokens
			cost.CachedInputTokens += e.Usage.CacheReadInputTokens
		}
		if e.Kind == thinkt.KindTurnDuration {
			if ms, ok := e.Metadata["duration_ms"].(int64); ok {
				cost.DurationMs += ms
			}
		}
	}
	if cost != (thinkt.SessionCost{}) {
		s.Cost = &cost
	}
	return s
}

// applyTo fills the empty fields of meta from the scan; EntryCount is always
// overwritten because the scan is the authority for it.
func (s sessionScan) applyTo(meta *thinkt.SessionMeta) {
	if meta.FirstPrompt == "" {
		meta.FirstPrompt = s.FirstPrompt
	}
	if !thinkt.IsRealModel(meta.Model) && s.Model != "" {
		meta.Model = s.Model
	}
	meta.EntryCount = s.EntryCount
	if meta.LastPrompt == "" {
		meta.LastPrompt = s.LastPrompt
	}
	if meta.Summary == "" {
		meta.Summary = meta.Title
	}
	if meta.Cost == nil {
		meta.Cost = s.Cost
	}
}

// cached returns the cache entry for this scan.
func (s sessionScan) cached(meta thinkt.SessionMeta) thinkt.CachedSession {
	return thinkt.CachedSession{
		FirstPrompt:     meta.FirstPrompt,
		Model:           meta.Model,
		EntryCount:      meta.EntryCount,
		GitBranch:       meta.GitBranch,
		ProjectPath:     meta.ProjectPath,
		CreatedAt:       meta.CreatedAt,
		ModifiedAt:      meta.ModifiedAt,
		FileSize:        meta.FileSize,
		Title:           meta.Title,
		LastPrompt:      meta.LastPrompt,
		SubagentCount:   meta.SubagentCount,
		ParentSessionID: meta.ParentSessionID,
		Cost:            meta.Cost,
	}
}

// loadSessionIndex reads <baseDir>/session_index.jsonl into sessionId →
// workDir; bad lines are skipped and a missing file yields an empty map.
func loadSessionIndex(baseDir string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(baseDir, "session_index.jsonl"))
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var rec struct {
			SessionID string `json:"sessionId"`
			WorkDir   string `json:"workDir"`
		}
		if json.Unmarshal(bytes.TrimSpace(sc.Bytes()), &rec) != nil || rec.SessionID == "" || rec.WorkDir == "" {
			continue
		}
		out[rec.SessionID] = rec.WorkDir
	}
	return out
}

// loadWorkspaces reads <baseDir>/workspaces.json into wd_ directory name →
// root path; a missing or malformed file yields an empty map.
func loadWorkspaces(baseDir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(baseDir, "workspaces.json"))
	if err != nil {
		return out
	}
	var ws struct {
		Workspaces map[string]struct {
			Root string `json:"root"`
		} `json:"workspaces"`
	}
	if json.Unmarshal(data, &ws) != nil {
		return out
	}
	for name, w := range ws.Workspaces {
		if w.Root != "" {
			out[name] = w.Root
		}
	}
	return out
}

// sessionsFromDir lists the main session of one session directory and one
// session per subagent wire under agents/. Nothing is listed when state.json
// or the main wire is missing.
func sessionsFromDir(baseDir, sessionDir, wdName, workspaceID string, index, workspaces map[string]string) []thinkt.SessionMeta {
	state, err := readState(filepath.Join(sessionDir, "state.json"))
	if err != nil {
		return nil
	}
	mainWire := filepath.Join(sessionDir, "agents", "main", "wire.jsonl")
	info, err := os.Stat(mainWire)
	if err != nil {
		return nil
	}
	sessionID := filepath.Base(sessionDir)
	projectPath := state.CWD
	if projectPath == "" {
		projectPath = state.WorkDir
	}
	if projectPath == "" {
		projectPath = index[sessionID]
	}
	if projectPath == "" {
		projectPath = workspaces[wdName]
	}
	if projectPath == "" {
		projectPath = "unknown"
	}
	createdAt := state.CreatedAt
	if createdAt.IsZero() {
		createdAt = info.ModTime()
	}
	modifiedAt := state.UpdatedAt
	if modifiedAt.IsZero() {
		modifiedAt = info.ModTime()
	}
	main := thinkt.SessionMeta{
		ID:          sessionID,
		ProjectPath: projectPath,
		FullPath:    mainWire,
		Title:       state.Title,
		Summary:     state.Title,
		LastPrompt:  state.LastPrompt,
		FileSize:    info.Size(),
		CreatedAt:   createdAt,
		ModifiedAt:  modifiedAt,
		Source:      thinkt.SourceKimi,
		WorkspaceID: workspaceID,
		ChunkCount:  1,
	}

	var children []thinkt.SessionMeta
	agentDirs, _ := filepath.Glob(filepath.Join(sessionDir, "agents", "*", "wire.jsonl"))
	sort.Strings(agentDirs)
	for _, wire := range agentDirs {
		agentID := filepath.Base(filepath.Dir(wire))
		if agentID == "main" {
			continue
		}
		cinfo, err := os.Stat(wire)
		if err != nil {
			continue
		}
		title := agentID
		if a, ok := state.Agents[agentID]; ok && a.Labels.ProfileName != "" {
			title = a.Labels.ProfileName + " " + agentID
		}
		children = append(children, thinkt.SessionMeta{
			ID:              sessionID + "/" + agentID,
			ParentSessionID: sessionID,
			ProjectPath:     projectPath,
			FullPath:        wire,
			Title:           title,
			Summary:         title,
			FileSize:        cinfo.Size(),
			CreatedAt:       cinfo.ModTime(),
			ModifiedAt:      cinfo.ModTime(),
			Source:          thinkt.SourceKimi,
			WorkspaceID:     workspaceID,
			ChunkCount:      1,
		})
	}
	main.SubagentCount = len(children)
	return append([]thinkt.SessionMeta{main}, children...)
}
