package export

import "strings"

const maxToolSummaryLen = 80

// toolPrimaryKeys maps tool names to their most informative input field.
// Ported from thinkt-web conversation-renderers.ts TOOL_PRIMARY_KEYS.
var toolPrimaryKeys = map[string]string{
	"Read":         "file_path",
	"ReadFile":     "file_path",
	"Write":        "file_path",
	"Edit":         "file_path",
	"Glob":         "pattern",
	"Grep":         "pattern",
	"Bash":         "command",
	"WebFetch":     "url",
	"WebSearch":    "query",
	"Task":         "description",
	"NotebookEdit": "notebook_path",
}

// toolSummary produces a compact one-line summary of a tool call.
// E.g. Read → "src/main.ts", Bash → "npm run build"
func toolSummary(toolName string, toolInput any) string {
	m, ok := toolInput.(map[string]any)
	if !ok || m == nil {
		return ""
	}

	// Try well-known primary key.
	if key, exists := toolPrimaryKeys[toolName]; exists {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return truncateSummary(s)
			}
		}
	}

	// Fallback: first non-empty string value.
	for _, val := range m {
		if s, ok := val.(string); ok && s != "" {
			return truncateSummary(s)
		}
	}

	return ""
}

func truncateSummary(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxToolSummaryLen {
		return s
	}
	// "…" is 3 bytes; subtract 3 to keep total byte length at maxToolSummaryLen.
	return s[:maxToolSummaryLen-3] + "…"
}
