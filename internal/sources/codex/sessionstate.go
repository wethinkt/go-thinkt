package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// sessionMetaRecord is the payload of a session_meta line.
type sessionMetaRecord struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	Model         string `json:"model"`
	ModelProvider string `json:"model_provider"`
	CLIVersion    string `json:"cli_version"`
	Git           struct {
		Branch string `json:"branch"`
	} `json:"git"`
	Source        json.RawMessage `json:"source"`
	AgentNickname string          `json:"agent_nickname"`
	AgentPath     string          `json:"agent_path"`
}

// parentThreadID returns the spawning thread of a subagent rollout, decoded
// from source.subagent.thread_spawn.parent_thread_id; "" for a string source.
func (r sessionMetaRecord) parentThreadID() string {
	if len(r.Source) == 0 || r.Source[0] != '{' {
		return ""
	}
	var src struct {
		Subagent struct {
			ThreadSpawn struct {
				ParentThreadID string `json:"parent_thread_id"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	}
	if json.Unmarshal(r.Source, &src) != nil {
		return ""
	}
	return src.Subagent.ThreadSpawn.ParentThreadID
}

// subagentTitle is the display title a subagent rollout falls back to.
func (r sessionMetaRecord) subagentTitle() string {
	if r.parentThreadID() == "" {
		return ""
	}
	return strings.TrimSpace(r.AgentNickname + " " + r.AgentPath)
}

// sessionScan is what one full pass over a rollout yields for SessionMeta.
type sessionScan struct {
	FirstPrompt     string
	LastPrompt      string
	Model           string
	Title           string
	EntryCount      int
	Cost            *thinkt.SessionCost
	ParentSessionID string
}

// scanSessionFile reads a rollout once and collects prompts, model, entry
// count, cost, and subagent linkage. A missing or unreadable file yields a
// zero scan.
func scanSessionFile(path string) sessionScan {
	var s sessionScan
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer f.Close()

	var cost thinkt.SessionCost
	var settingsModel, providerModel string
	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		h, class := ClassifyLine(line)
		s.EntryCount += entryWeight(h, class)
		switch h.Key() {
		case "session_meta":
			var r sessionMetaRecord
			if json.Unmarshal(h.Payload, &r) == nil {
				s.ParentSessionID = r.parentThreadID()
				s.Title = r.subagentTitle()
				if thinkt.IsRealModel(r.Model) {
					providerModel = r.Model
				} else if r.ModelProvider != "" {
					providerModel = r.ModelProvider
				}
			}
		case "turn_context":
			var tc struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(h.Payload, &tc) == nil && thinkt.IsRealModel(tc.Model) {
				s.Model = tc.Model
			}
		case "event_msg/thread_settings_applied":
			var ts struct {
				ThreadSettings struct {
					Model string `json:"model"`
				} `json:"thread_settings"`
			}
			if json.Unmarshal(h.Payload, &ts) == nil && ts.ThreadSettings.Model != "" {
				settingsModel = ts.ThreadSettings.Model
			}
		case "response_item/message":
			if h.Role != "user" {
				continue
			}
			if class, kind := userMessageClass(h); class != ClassTurn || kind != thinkt.KindNone {
				continue
			}
			if text := userPromptText(h.Payload); text != "" {
				if s.FirstPrompt == "" {
					s.FirstPrompt = text
				}
				s.LastPrompt = text
			}
		case "event_msg/user_message":
			var p struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(h.Payload, &p) == nil {
				if text := strings.TrimSpace(p.Message); text != "" {
					if s.FirstPrompt == "" {
						s.FirstPrompt = text
					}
					s.LastPrompt = text
				}
			}
		case "event_msg/task_complete":
			var p struct {
				DurationMs int64 `json:"duration_ms"`
			}
			if json.Unmarshal(h.Payload, &p) == nil {
				cost.DurationMs += p.DurationMs
			}
		case "item/FileChange":
			var item struct {
				Changes map[string]struct {
					UnifiedDiff string `json:"unified_diff"`
				} `json:"changes"`
			}
			if decodeItem(h, &item) {
				for _, ch := range item.Changes {
					if ch.UnifiedDiff == "" {
						continue
					}
					added, removed := diffLineCounts(ch.UnifiedDiff)
					cost.LinesAdded += added
					cost.LinesRemoved += removed
				}
			}
		case "token_usage_record":
			if u := decodeUsage(h.Payload); u != nil {
				cost.InputTokens += u.InputTokens
				cost.OutputTokens += u.OutputTokens
				cost.CachedInputTokens += u.CacheReadInputTokens
			}
		}
	}
	if s.Model == "" {
		s.Model = settingsModel
	}
	if s.Model == "" {
		s.Model = providerModel
	}
	if cost != (thinkt.SessionCost{}) {
		s.Cost = &cost
	}
	return s
}

// userPromptText joins the input_text parts of a wire user message.
func userPromptText(payload json.RawMessage) string {
	var m wireMessage
	if json.Unmarshal(payload, &m) != nil {
		return ""
	}
	return joinParts(m.Content, "input_text", "text")
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
	if meta.Title == "" {
		meta.Title = s.Title
	}
	if meta.Summary == "" {
		meta.Summary = meta.Title
	}
	if meta.LastPrompt == "" {
		meta.LastPrompt = s.LastPrompt
	}
	if meta.Cost == nil {
		meta.Cost = s.Cost
	}
	if meta.ParentSessionID == "" {
		meta.ParentSessionID = s.ParentSessionID
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

// loadSessionIndex reads <baseDir>/session_index.jsonl and returns thread
// names by session id; the last non-empty name for an id wins. Bad lines are
// skipped and a missing file yields an empty map.
func loadSessionIndex(baseDir string) map[string]string {
	titles := map[string]string{}
	f, err := os.Open(filepath.Join(baseDir, "session_index.jsonl"))
	if err != nil {
		return titles
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		var rec struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(bytes.TrimSpace(scanner.Bytes()), &rec) != nil {
			continue
		}
		if rec.ID == "" || strings.TrimSpace(rec.ThreadName) == "" {
			continue
		}
		titles[rec.ID] = strings.TrimSpace(rec.ThreadName)
	}
	return titles
}
