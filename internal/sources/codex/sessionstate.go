package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
	Related         []thinkt.RelatedPath
}

// scanSessionFile reads a rollout once and collects prompts, model, entry
// count, cost, and subagent linkage. A missing or unreadable file yields a
// zero scan.
func scanSessionFile(path string, related *thinkt.RelatedTracker) sessionScan {
	var s sessionScan
	f, err := os.Open(path)
	if err != nil {
		return s
	}
	defer func() { _ = f.Close() }()

	var cost thinkt.SessionCost
	var settingsModel, providerModel string
	var legacyKey, legacyText string            // the legacy event awaiting its wire twin
	var lastAgentText, lastReasoningText string // the harness item a wire record may twin
	var calls relatedCalls
	scanner := thinkt.NewScannerWithMaxCapacityCustom(f, 64*1024, thinkt.MaxScannerCapacity)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		h, class := ClassifyLine(line)
		weight := entryWeight(h, class)
		if class.IsEntry() && isDedupeCandidate(h) && h.LeadText != "" {
			// Same rule as the parser: a wire assistant message or reasoning
			// record is dropped only when it repeats the item just before it.
			twin := (h.PayloadType == "message" && h.LeadText == lastAgentText) ||
				(h.PayloadType == "reasoning" && h.LeadText == lastReasoningText)
			if !twin {
				weight = 1
			}
		}
		if weight > 0 || (class.IsEntry() && isDedupeCandidate(h)) {
			lastAgentText, lastReasoningText = "", ""
			switch h.Key() {
			case "item/AgentMessage":
				lastAgentText = h.LeadText
			case "item/Reasoning":
				lastReasoningText = h.LeadText
			}
		}
		if weight > 0 && legacyKey != "" {
			// A legacy event and the wire record that repeats it are one
			// entry, as the parser emits them.
			if isLegacyTwin(legacyKey, legacyText, h) {
				weight = 0
			}
			legacyKey, legacyText = "", ""
		}
		s.EntryCount += weight
		if weight > 0 && isLegacyEvent(h) {
			legacyKey, legacyText = h.Key(), legacyEventText(h.Payload)
		}
		switch h.Key() {
		case "session_meta":
			var r sessionMetaRecord
			if json.Unmarshal(h.Payload, &r) == nil {
				s.ParentSessionID = r.parentThreadID()
				calls.cwd = r.CWD
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
				CWD   string `json:"cwd"`
			}
			if json.Unmarshal(h.Payload, &tc) == nil {
				if thinkt.IsRealModel(tc.Model) {
					s.Model = tc.Model
				}
				if tc.CWD != "" {
					calls.cwd = tc.CWD
				}
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
			if class, kind := userMessageClass(h); class != thinkt.ClassTurn || kind != thinkt.KindNone {
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
				Changes map[string]fileChangeEntry `json:"changes"`
			}
			if decodeItem(h, &item) {
				calls.sawChange = true
				for path, ch := range item.Changes {
					related.Touch(path, calls.cwd, thinkt.RelatedEdited)
					if diff := changeDiff(path, ch); diff != "" {
						added, removed := diffLineCounts(diff)
						cost.LinesAdded += added
						cost.LinesRemoved += removed
					}
				}
			}
		case "item/CommandExecution":
			var item struct {
				CWD string `json:"cwd"`
			}
			if related != nil && decodeItem(h, &item) {
				calls.sawExec = true
				if item.CWD == "" {
					item.CWD = calls.cwd
				}
				related.Touch(item.CWD, calls.cwd, thinkt.RelatedCommand)
			}
		case "response_item/function_call", "response_item/custom_tool_call":
			if related != nil {
				calls.wire(h.Payload)
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
	calls.flush(related)
	s.Related = related.Paths()
	return s
}

// relatedCalls gathers the folders a rollout's wire calls name. Harness
// items are the record of what ran, and wire calls share no id with them, so
// the wire calls count only in a rollout with no item of that kind.
type relatedCalls struct {
	cwd                string // the turn's folder
	sawExec, sawChange bool
	execs, patches     []wirePath
}

// wirePath is a path a wire call named, and the folder it was named from.
type wirePath struct{ path, base string }

// patchFile matches the file headers of an apply_patch input.
var patchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

// wire notes the folder of a wire exec_command or apply_patch call.
func (c *relatedCalls) wire(payload json.RawMessage) {
	var call struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Input     string `json:"input"`
	}
	if json.Unmarshal(payload, &call) != nil {
		return
	}
	switch call.Name {
	case "exec_command":
		var args struct {
			Workdir string `json:"workdir"`
		}
		if json.Unmarshal([]byte(call.Arguments), &args) == nil {
			if args.Workdir == "" {
				args.Workdir = c.cwd
			}
			c.execs = append(c.execs, wirePath{args.Workdir, c.cwd})
		}
	case "apply_patch":
		for _, m := range patchFile.FindAllStringSubmatch(call.Input, -1) {
			c.patches = append(c.patches, wirePath{strings.TrimSpace(m[1]), c.cwd})
		}
	}
}

// flush records the wire calls of the kinds no item covered.
func (c *relatedCalls) flush(related *thinkt.RelatedTracker) {
	if !c.sawExec {
		for _, p := range c.execs {
			related.Touch(p.path, p.base, thinkt.RelatedCommand)
		}
	}
	if !c.sawChange {
		for _, p := range c.patches {
			related.Touch(p.path, p.base, thinkt.RelatedEdited)
		}
	}
}

// legacyEventText returns the trimmed text of a legacy event_msg
// user_message / agent_message (message) or agent_reasoning (text).
func legacyEventText(payload json.RawMessage) string {
	var p struct {
		Message string `json:"message"`
		Text    string `json:"text"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	if t := strings.TrimSpace(p.Message); t != "" {
		return t
	}
	return strings.TrimSpace(p.Text)
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
	if meta.RelatedPaths == nil {
		meta.RelatedPaths = s.Related
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
		RelatedPaths:    meta.RelatedPaths,
	}
}

// rolloutIDRE matches the session id that ends every rollout file name
// (rollout-<timestamp>-<uuid>.jsonl).
var rolloutIDRE = regexp.MustCompile(`([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})\.jsonl$`)

// rolloutIDFromPath returns the session id a rollout file name ends with, or
// "" when the name does not carry one.
func rolloutIDFromPath(path string) string {
	m := rolloutIDRE.FindStringSubmatch(filepath.Base(path))
	if m == nil {
		return ""
	}
	return m[1]
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
	defer func() { _ = f.Close() }()
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
