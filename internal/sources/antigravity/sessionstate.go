package antigravity

import (
	"os"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// sessionScan is what one parser pass over a transcript yields for
// SessionMeta.
type sessionScan struct {
	FirstPrompt string
	LastPrompt  string
	Model       string
	EntryCount  int
	Cost        *thinkt.SessionCost
	Cwd         string // most common run_command Cwd, for the workspace fallback
	CreatedAt   time.Time
	Related     []thinkt.RelatedPath
}

// scanSessionFile parses a transcript to EOF and summarises it. Related
// folders are those outside projectPath that tool calls touched, plus the
// conversation's extra workspace folders; paths under exclude are skipped.
func scanSessionFile(path, conversationID, parentID, projectPath string, extraWorkspaces, exclude []string) sessionScan {
	var s sessionScan
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer func() { _ = f.Close() }()
	p := NewParser(f, conversationID, parentID)
	p.TrackRelated(projectPath, exclude)
	for _, w := range extraWorkspaces {
		p.related.workspace(w)
	}
	var cost thinkt.SessionCost
	var cwds cwdTally
	for {
		e, err := p.NextEntry()
		if err != nil {
			break
		}
		s.EntryCount++
		if s.CreatedAt.IsZero() && !e.Timestamp.IsZero() {
			s.CreatedAt = e.Timestamp
		}
		if e.Model != "" {
			s.Model = e.Model
		}
		if e.Role == thinkt.RoleUser && e.Kind == thinkt.KindNone && e.Text != "" {
			if s.FirstPrompt == "" {
				s.FirstPrompt = e.Text
			}
			s.LastPrompt = e.Text
		}
		if e.Role != thinkt.RoleAssistant {
			continue
		}
		if e.Usage != nil {
			cost.InputTokens += e.Usage.InputTokens
			cost.OutputTokens += e.Usage.OutputTokens
			cost.CachedInputTokens += e.Usage.CacheReadInputTokens
		}
		cwds.add(e)
	}
	s.Cwd = cwds.top()
	if cost != (thinkt.SessionCost{}) {
		s.Cost = &cost
	}
	s.Related = p.RelatedPaths()
	return s
}

// cwdTally counts the folders run_command calls ran in.
type cwdTally struct {
	counts map[string]int
	order  []string
}

// add counts the run_command Cwd arguments of an assistant entry.
func (c *cwdTally) add(e *thinkt.Entry) {
	if e.Role != thinkt.RoleAssistant {
		return
	}
	for _, b := range e.ContentBlocks {
		if b.Type != "tool_use" || b.ToolName != "run_command" {
			continue
		}
		args, _ := b.ToolInput.(map[string]any)
		raw, _ := args["Cwd"].(string)
		if cwd := unquotePath(raw); cwd != "" {
			if c.counts == nil {
				c.counts = map[string]int{}
			}
			if c.counts[cwd] == 0 {
				c.order = append(c.order, cwd)
			}
			c.counts[cwd]++
		}
	}
}

// top returns the most common folder, the first seen on a tie; "" if none.
func (c *cwdTally) top() string {
	best := ""
	for _, cwd := range c.order {
		if c.counts[cwd] > c.counts[best] {
			best = cwd
		}
	}
	return best
}

// commandCwd returns the folder a transcript's commands most often ran in,
// for a conversation with no workspace. It skips the full scan's prompt,
// usage and related-folder work.
func commandCwd(path, conversationID string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	p := NewParser(f, conversationID, "")
	var cwds cwdTally
	for e, err := p.NextEntry(); err == nil; e, err = p.NextEntry() {
		cwds.add(e)
	}
	return cwds.top()
}

// resolveWorkspace picks a conversation's project folder: the first
// workspace folder, else the most common command Cwd, else the workspace
// whose last_conversations.json entry names it; "" means no workspace.
func resolveWorkspace(row summaryRow, scanCwd string, last map[string]string, id string) string {
	switch {
	case len(row.Workspaces) > 0:
		return row.Workspaces[0]
	case scanCwd != "":
		return scanCwd
	default:
		return last[id]
	}
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
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = s.CreatedAt
	}
	if meta.Title == "" {
		meta.Title = s.FirstPrompt
	}
	if meta.Summary == "" {
		meta.Summary = meta.Title
	}
	if meta.Cost == nil {
		meta.Cost = s.Cost
	}
	if meta.RelatedPaths == nil {
		meta.RelatedPaths = s.Related
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
		RelatedPaths:    meta.RelatedPaths,
	}
}
