package claude

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// docTools names the tools that write plan files.
var docTools = map[string]thinkt.DocTool{
	"Write": {Path: "file_path", Content: "content"},
	"Edit":  {Path: "file_path", Old: "old_string", New: "new_string", All: "replace_all"},
}

var _ thinkt.DocumentStore = (*Store)(nil)

// SessionDocuments returns the plans a session wrote under <home>/plans and
// the plans it stated with ExitPlanMode.
func (s *Store) SessionDocuments(ctx context.Context, sessionID string) ([]thinkt.AgentDocument, error) {
	plans := filepath.Join(s.baseDir, "plans")
	tracker := thinkt.NewDocumentTracker(docTools, func(path string) bool {
		return filepath.Dir(filepath.Clean(path)) == plans && strings.HasSuffix(path, ".md")
	})
	err := thinkt.VisitEntries(ctx, s, sessionID, func(e *thinkt.Entry) {
		tracker.Entry(e)
		for _, b := range e.ContentBlocks {
			if args, ok := b.ToolInput.(map[string]any); ok && b.ToolName == "ExitPlanMode" {
				if plan, _ := args["plan"].(string); plan != "" {
					tracker.Note("plan", thinkt.DocPlan, e, b.ToolName, plan)
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return tracker.Documents(), nil
}
