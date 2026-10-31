package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// sessionScan is everything one pass over a transcript yields for
// SessionMeta: the hints the old 50-line scan found, the entry count, and
// the session-state records that never become entries.
type sessionScan struct {
	FirstPrompt string
	Model       string
	Title       string
	Summary     string // legacy summary record; title fallback
	LastPrompt  string
	EntryCount  int
	PRLinks     []thinkt.PRLink
	Worktree    *thinkt.WorktreeInfo
	Cost        *thinkt.SessionCost
	Related     []thinkt.RelatedPath
}

// scanSessionFile classifies every line of a transcript. It unmarshals
// message bodies only until the first prompt and first real model are
// known; session-state lines are small and decoded in full. The last
// non-empty ai-title wins.
func scanSessionFile(path string, related *thinkt.RelatedTracker) sessionScan {
	var s sessionScan
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer f.Close()

	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxLineCapacity)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		typ, _, class := ClassifyLine(line)
		switch class {
		case thinkt.ClassSessionState:
			s.applyState(typ, line)
			continue
		case thinkt.ClassTurn:
			s.EntryCount++
			if typ == EntryTypeAssistant {
				trackToolCalls(related, line)
			}
			if s.FirstPrompt != "" && thinkt.IsRealModel(s.Model) {
				continue
			}
			var entry struct {
				Type    EntryType       `json:"type"`
				Message json.RawMessage `json:"message"`
			}
			if json.Unmarshal(line, &entry) != nil {
				continue
			}
			switch typ {
			case EntryTypeUser:
				if s.FirstPrompt == "" {
					var msg UserMessage
					if json.Unmarshal(entry.Message, &msg) == nil {
						s.FirstPrompt = msg.Content.GetText()
					}
				}
			case EntryTypeAssistant:
				if !thinkt.IsRealModel(s.Model) {
					var msg struct {
						Model string `json:"model"`
					}
					if json.Unmarshal(entry.Message, &msg) == nil && thinkt.IsRealModel(msg.Model) {
						s.Model = msg.Model
					}
				}
			}
		default:
			if class.IsEntry() {
				s.EntryCount++
			}
			if typ == EntryTypeSummary {
				var r struct {
					Summary string `json:"summary"`
				}
				if json.Unmarshal(line, &r) == nil && strings.TrimSpace(r.Summary) != "" {
					s.Summary = r.Summary
				}
			}
		}
	}
	s.Related = related.Paths()
	return s
}

// relatedTools names the tools whose calls say which folder a session read
// or edited. Bash carries no folder of its own; its record's cwd does.
var relatedTools = thinkt.ToolPaths{
	"Read":         {Arg: "file_path", Reason: thinkt.RelatedRead},
	"NotebookRead": {Arg: "notebook_path", Reason: thinkt.RelatedRead},
	"Grep":         {Arg: "path", Reason: thinkt.RelatedRead},
	"Glob":         {Arg: "path", Reason: thinkt.RelatedRead},
	"LS":           {Arg: "path", Reason: thinkt.RelatedRead},
	"Edit":         {Arg: "file_path", Reason: thinkt.RelatedEdited},
	"MultiEdit":    {Arg: "file_path", Reason: thinkt.RelatedEdited},
	"Write":        {Arg: "file_path", Reason: thinkt.RelatedEdited},
	"NotebookEdit": {Arg: "notebook_path", Reason: thinkt.RelatedEdited},
}

// trackToolCalls feeds an assistant line's tool calls to related. Only lines
// holding a tool call are decoded, so the scan stays a byte classifier.
func trackToolCalls(related *thinkt.RelatedTracker, line []byte) {
	if related == nil || !bytes.Contains(line, []byte(`"tool_use"`)) {
		return
	}
	var rec struct {
		Cwd     string `json:"cwd"`
		Message struct {
			Content []struct {
				Type  string         `json:"type"`
				Name  string         `json:"name"`
				Input map[string]any `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &rec) != nil {
		return
	}
	for _, b := range rec.Message.Content {
		switch {
		case b.Type != "tool_use":
		case b.Name == "Bash":
			related.Touch(rec.Cwd, "", thinkt.RelatedCommand)
		default:
			related.Call(relatedTools, b.Name, b.Input, rec.Cwd)
		}
	}
}

// displayTitle is the ai-title, or the legacy summary when no title was
// ever written.
func (s sessionScan) displayTitle() string {
	if s.Title != "" {
		return s.Title
	}
	return s.Summary
}

// applyState folds one session-state record into the scan.
func (s *sessionScan) applyState(typ EntryType, line []byte) {
	switch typ {
	case EntryTypeAITitle:
		var r struct {
			AITitle string `json:"aiTitle"`
		}
		if json.Unmarshal(line, &r) == nil && strings.TrimSpace(r.AITitle) != "" {
			s.Title = r.AITitle
		}
	case EntryTypeLastPrompt:
		var r struct {
			LastPrompt string `json:"lastPrompt"`
		}
		if json.Unmarshal(line, &r) == nil && r.LastPrompt != "" {
			s.LastPrompt = r.LastPrompt
		}
	case EntryTypePRLink:
		var r struct {
			PRNumber     int    `json:"prNumber"`
			PRURL        string `json:"prUrl"`
			PRRepository string `json:"prRepository"`
			Timestamp    string `json:"timestamp"`
		}
		if json.Unmarshal(line, &r) == nil {
			link := thinkt.PRLink{Number: r.PRNumber, URL: r.PRURL, Repository: r.PRRepository}
			link.Timestamp, _ = time.Parse(time.RFC3339, r.Timestamp)
			s.PRLinks = append(s.PRLinks, link)
		}
	case EntryTypeWorktreeState:
		var r struct {
			WorktreeSession struct {
				OriginalCwd    string `json:"originalCwd"`
				WorktreePath   string `json:"worktreePath"`
				WorktreeBranch string `json:"worktreeBranch"`
				OriginalBranch string `json:"originalBranch"`
			} `json:"worktreeSession"`
		}
		if json.Unmarshal(line, &r) == nil {
			w := r.WorktreeSession
			s.Worktree = &thinkt.WorktreeInfo{Path: w.WorktreePath, Branch: w.WorktreeBranch, OriginalCwd: w.OriginalCwd, OriginalBranch: w.OriginalBranch}
		}
	case EntryTypeRelocated:
		var r struct {
			RelocatedCwd string `json:"relocatedCwd"`
		}
		if json.Unmarshal(line, &r) == nil && r.RelocatedCwd != "" {
			if s.Worktree == nil {
				s.Worktree = &thinkt.WorktreeInfo{}
			}
			if s.Worktree.Path == "" {
				s.Worktree.Path = r.RelocatedCwd
			}
		}
	case EntryTypeCostState:
		var r struct {
			TotalCostUSD      float64 `json:"totalCostUSD"`
			TotalDuration     int64   `json:"totalDuration"`
			TotalAPIDuration  int64   `json:"totalAPIDuration"`
			TotalToolDuration int64   `json:"totalToolDuration"`
			TotalLinesAdded   int     `json:"totalLinesAdded"`
			TotalLinesRemoved int     `json:"totalLinesRemoved"`
		}
		if json.Unmarshal(line, &r) == nil {
			s.Cost = &thinkt.SessionCost{TotalUSD: r.TotalCostUSD, DurationMs: r.TotalDuration, APIDurationMs: r.TotalAPIDuration, ToolDurationMs: r.TotalToolDuration, LinesAdded: r.TotalLinesAdded, LinesRemoved: r.TotalLinesRemoved}
		}
	}
}

// applyTo fills the empty fields of meta from the scan.
func (s sessionScan) applyTo(meta *thinkt.SessionMeta) {
	if meta.FirstPrompt == "" {
		meta.FirstPrompt = s.FirstPrompt
	}
	if !thinkt.IsRealModel(meta.Model) && s.Model != "" {
		meta.Model = s.Model
	}
	meta.EntryCount = s.EntryCount
	if meta.Title == "" {
		meta.Title = s.displayTitle()
	}
	if meta.Summary == "" {
		meta.Summary = meta.Title
	}
	if meta.LastPrompt == "" {
		meta.LastPrompt = s.LastPrompt
	}
	if meta.PRLinks == nil {
		meta.PRLinks = s.PRLinks
	}
	if meta.Worktree == nil {
		meta.Worktree = s.Worktree
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
		FirstPrompt:   meta.FirstPrompt,
		Model:         meta.Model,
		EntryCount:    meta.EntryCount,
		GitBranch:     meta.GitBranch,
		ProjectPath:   meta.ProjectPath,
		CreatedAt:     meta.CreatedAt,
		ModifiedAt:    meta.ModifiedAt,
		FileSize:      meta.FileSize,
		Title:         meta.Title,
		LastPrompt:    meta.LastPrompt,
		SubagentCount: meta.SubagentCount,
		PRLinks:       meta.PRLinks,
		Worktree:      meta.Worktree,
		Cost:          meta.Cost,
		RelatedPaths:  meta.RelatedPaths,
	}
}

// subagentsDir is the directory holding a session's subagent transcripts.
func subagentsDir(sessionPath string) string {
	return filepath.Join(strings.TrimSuffix(sessionPath, ".jsonl"), "subagents")
}

// countSubagents counts agent-*.jsonl transcripts under a session.
func countSubagents(sessionPath string) int {
	matches, err := filepath.Glob(filepath.Join(subagentsDir(sessionPath), "agent-*.jsonl"))
	if err != nil {
		return 0
	}
	return len(matches)
}
