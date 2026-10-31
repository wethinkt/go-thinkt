package claude

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// systemKind maps a system record subtype to its kind and default visibility.
// Away summaries and compaction boundaries are part of the conversation;
// durations, notices, and local slash commands are secondary.
func systemKind(subtype string) (thinkt.EntryKind, thinkt.Visibility) {
	switch subtype {
	case "turn_duration":
		return thinkt.KindTurnDuration, thinkt.VisibilitySecondary
	case "away_summary":
		return thinkt.KindAwaySummary, thinkt.VisibilityNormal
	case "compact_boundary":
		return thinkt.KindCompactBoundary, thinkt.VisibilityNormal
	case "informational":
		return thinkt.KindInformational, thinkt.VisibilitySecondary
	case "local_command":
		return thinkt.KindLocalCommand, thinkt.VisibilitySecondary
	default:
		return thinkt.KindUnknown, thinkt.VisibilitySecondary
	}
}

// userWrapperKinds maps the opening tag of a harness-authored user prompt to
// its kind. Detection looks at the first tag of the trimmed text only.
var userWrapperKinds = []struct {
	prefix string
	kind   thinkt.EntryKind
}{
	{"<system-reminder", thinkt.KindMeta},
	{"<task-notification", thinkt.KindTaskNotification},
	{"<agent-message", thinkt.KindAgentMessage},
	{"<local-command-stdout", thinkt.KindCommandOutput},
	{"<local-command-caveat", thinkt.KindCommandOutput},
	{"<bash-stdout", thinkt.KindCommandOutput},
	{"<bash-input", thinkt.KindCommandOutput},
}

// taskTagRe matches a tag of a <task-notification> body.
var taskTagRe = regexp.MustCompile(`(?s)<(task-id|tool-use-id|status|output-file|summary)>(.*?)</`)

// parseTaskNotification reads the task link of a <task-notification>, nil
// without a task or tool id. A status means the task ended; without one the
// note is an update.
func parseTaskNotification(text string) *thinkt.TaskLink {
	l := thinkt.TaskLink{Event: thinkt.TaskUpdate}
	for _, m := range taskTagRe.FindAllStringSubmatch(text, -1) {
		value := html.UnescapeString(strings.TrimSpace(m[2]))
		switch m[1] {
		case "task-id":
			l.ID = value
		case "tool-use-id":
			l.ToolUseID = value
		case "status":
			l.Status, l.Event = value, thinkt.TaskFinished
		case "output-file":
			l.OutputPath = value
		case "summary":
			l.Description = value
		}
	}
	if l.ID == "" && l.ToolUseID == "" {
		return nil
	}
	return &l
}

// detectUserKind classifies a user prompt by its wrapper tag. A tagged
// wrapper wins over isMeta so relayed agent messages keep their kind.
func detectUserKind(text string, isMeta bool) thinkt.EntryKind {
	trimmed := strings.TrimSpace(text)
	for _, w := range userWrapperKinds {
		if strings.HasPrefix(trimmed, w.prefix) {
			return w.kind
		}
	}
	if isMeta {
		return thinkt.KindMeta
	}
	return thinkt.KindNone
}

// userKindVisibility: meta reminders and command echoes are secondary;
// notifications, agent messages, and compaction summaries stay visible.
func userKindVisibility(kind thinkt.EntryKind) thinkt.Visibility {
	switch kind {
	case thinkt.KindMeta, thinkt.KindCommandOutput:
		return thinkt.VisibilitySecondary
	default:
		return thinkt.VisibilityNormal
	}
}

// compactionInfo decodes compactMetadata from a compact_boundary record.
func compactionInfo(raw json.RawMessage, logicalParent string) *thinkt.CompactionInfo {
	info := &thinkt.CompactionInfo{LogicalParentUUID: logicalParent}
	if len(raw) == 0 {
		return info
	}
	var meta struct {
		Trigger          string `json:"trigger"`
		PreTokens        int    `json:"preTokens"`
		DurationMs       int    `json:"durationMs"`
		PreservedSegment struct {
			HeadUUID   string `json:"headUuid"`
			AnchorUUID string `json:"anchorUuid"`
			TailUUID   string `json:"tailUuid"`
		} `json:"preservedSegment"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return info
	}
	info.Trigger = meta.Trigger
	info.PreTokens = meta.PreTokens
	info.DurationMs = meta.DurationMs
	info.HeadUUID = meta.PreservedSegment.HeadUUID
	info.AnchorUUID = meta.PreservedSegment.AnchorUUID
	info.TailUUID = meta.PreservedSegment.TailUUID
	return info
}

// attribution builds the source-neutral attribution, or nil when empty.
func attribution(e *Entry) *thinkt.Attribution {
	if e.AttributionAgent == "" && e.AttributionSkill == "" && e.AttributionPlugin == "" && e.AttributionMcpServer == "" && e.AttributionMcpTool == "" {
		return nil
	}
	return &thinkt.Attribution{Agent: e.AttributionAgent, Skill: e.AttributionSkill, Plugin: e.AttributionPlugin, MCPServer: e.AttributionMcpServer, MCPTool: e.AttributionMcpTool}
}

// rawText pretty-prints an unknown record for display.
func rawText(raw json.RawMessage) string {
	var buf strings.Builder
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	buf.Write(data)
	return buf.String()
}
