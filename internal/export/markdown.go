package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func renderMarkdown(w io.Writer, entries []thinkt.Entry, opts Options) error {
	fmt.Fprintf(w, "# %s\n\n", opts.Title)

	toolResults := buildToolResultIndex(entries)
	inlined := make(map[string]bool)

	for _, entry := range entries {
		content := renderEntryMarkdown(&entry, toolResults, inlined)
		if content == "" {
			continue
		}

		ts := ""
		if !entry.Timestamp.IsZero() {
			ts = " (" + entry.Timestamp.Format(time.RFC3339) + ")"
		}

		fmt.Fprintf(w, "---\n\n## %s%s\n\n%s", roleTitle(entry.Role), ts, content)
	}

	fmt.Fprintf(w, "---\n\n*Exported from thinkt on %s*\n", time.Now().Format(time.RFC3339))
	return nil
}

func renderEntryMarkdown(entry *thinkt.Entry, toolResults map[string]*thinkt.ContentBlock, inlined map[string]bool) string {
	var b strings.Builder

	if len(entry.ContentBlocks) > 0 {
		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "text":
				if block.Text != "" {
					b.WriteString(block.Text)
					b.WriteString("\n\n")
				}
			case "thinking":
				b.WriteString("<details>\n<summary>Thinking</summary>\n\n```\n")
				b.WriteString(block.Thinking)
				b.WriteString("\n```\n\n</details>\n\n")
			case "tool_use":
				summary := toolSummary(block.ToolName, block.ToolInput)
				summaryText := ""
				if summary != "" {
					summaryText = " (" + summary + ")"
				}
				input, _ := json.MarshalIndent(block.ToolInput, "", "  ")
				fmt.Fprintf(&b, "<details>\n<summary>Tool: %s%s</summary>\n\n```json\n%s\n```\n\n</details>\n\n",
					block.ToolName, summaryText, input)

				// Inline paired result
				if result, ok := toolResults[block.ToolUseID]; ok {
					inlined[block.ToolUseID] = true
					label := "Result"
					if result.IsError {
						label = "Error"
					}
					fmt.Fprintf(&b, "<details>\n<summary>%s</summary>\n\n```\n%s\n```\n\n</details>\n\n",
						label, result.ToolResult)
				}
			case "tool_result":
				if !inlined[block.ToolUseID] {
					label := "Result"
					if block.IsError {
						label = "Error"
					}
					fmt.Fprintf(&b, "<details>\n<summary>%s</summary>\n\n```\n%s\n```\n\n</details>\n\n",
						label, block.ToolResult)
				}
			case "image":
				fmt.Fprintf(&b, "> Image: %s\n\n", block.MediaType)
			case "document":
				fmt.Fprintf(&b, "> Document: %s\n\n", block.MediaType)
			}
		}
	}

	// Fallback: entry.Text for entries with no content blocks
	if b.Len() == 0 && entry.Text != "" {
		b.WriteString(entry.Text)
		b.WriteString("\n\n")
	}

	return b.String()
}

func buildToolResultIndex(entries []thinkt.Entry) map[string]*thinkt.ContentBlock {
	idx := make(map[string]*thinkt.ContentBlock)
	for i := range entries {
		for j := range entries[i].ContentBlocks {
			block := &entries[i].ContentBlocks[j]
			if block.Type == "tool_result" && block.ToolUseID != "" {
				idx[block.ToolUseID] = block
			}
		}
	}
	return idx
}

func roleTitle(role thinkt.Role) string {
	switch role {
	case thinkt.RoleUser:
		return "User"
	case thinkt.RoleAssistant:
		return "Assistant"
	case thinkt.RoleSystem:
		return "System"
	default:
		return string(role)
	}
}
