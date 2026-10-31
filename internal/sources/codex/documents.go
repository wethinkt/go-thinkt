package codex

import (
	"context"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var _ thinkt.DocumentStore = (*Store)(nil)

// SessionDocuments returns a session's plan. Codex keeps no plan file, so
// each update_plan call and Plan item is a version of one stated document.
func (s *Store) SessionDocuments(ctx context.Context, sessionID string) ([]thinkt.AgentDocument, error) {
	tracker := thinkt.NewDocumentTracker(nil, nil)
	err := thinkt.VisitEntries(ctx, s, sessionID, func(e *thinkt.Entry) {
		if e.Subtype == "plan" && e.Text != "" {
			tracker.Note("plan", thinkt.DocPlan, e, "Plan", e.Text)
		}
		for _, b := range e.ContentBlocks {
			if args, ok := b.ToolInput.(map[string]any); ok && b.ToolName == "update_plan" {
				if text := planChecklist(args); text != "" {
					tracker.Note("plan", thinkt.DocPlan, e, b.ToolName, text)
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return tracker.Documents(), nil
}

// planChecklist renders update_plan arguments as a checklist.
func planChecklist(args map[string]any) string {
	var b strings.Builder
	if why, _ := args["explanation"].(string); why != "" {
		b.WriteString(why + "\n\n")
	}
	steps, _ := args["plan"].([]any)
	if len(steps) == 0 {
		return ""
	}
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		text, _ := step["step"].(string)
		status, _ := step["status"].(string)
		mark := " "
		switch status {
		case "completed":
			mark = "x"
		case "in_progress":
			mark = "~"
		}
		b.WriteString("- [" + mark + "] " + text + "\n")
	}
	return b.String()
}
