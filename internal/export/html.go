package export

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func renderHTML(w io.Writer, entries []thinkt.Entry, opts Options) error {
	title := html.EscapeString(opts.Title)

	// DOCTYPE + head
	fmt.Fprintf(w, "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	fmt.Fprintf(w, "<meta charset=\"UTF-8\">\n")
	fmt.Fprintf(w, "<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	fmt.Fprintf(w, "<title>%s</title>\n", title)
	fmt.Fprintf(w, "<style>\n%s\n</style>\n", exportCSS)
	fmt.Fprintf(w, "</head>\n<body>\n")

	fmt.Fprintf(w, "<h1>%s</h1>\n", title)

	toolResults := buildToolResultIndex(entries)
	inlined := make(map[string]bool)

	for _, entry := range entries {
		roleClass := "role-" + strings.ToLower(string(entry.Role))
		roleLabel := html.EscapeString(roleTitle(entry.Role))

		ts := ""
		if !entry.Timestamp.IsZero() {
			ts = entry.Timestamp.Format(time.RFC3339)
		}

		fmt.Fprintf(w, "<div class=\"entry\">\n")
		fmt.Fprintf(w, "  <div class=\"entry-header\">\n")
		fmt.Fprintf(w, "    <span class=\"role %s\">%s</span>\n", roleClass, roleLabel)
		if ts != "" {
			fmt.Fprintf(w, "    <span class=\"timestamp\">%s</span>\n", ts)
		}
		fmt.Fprintf(w, "  </div>\n")
		fmt.Fprintf(w, "  <div class=\"content\">\n")

		renderEntryHTML(w, &entry, toolResults, inlined)

		fmt.Fprintf(w, "  </div>\n")
		fmt.Fprintf(w, "</div>\n")
	}

	// Footer
	fmt.Fprintf(w, "<div class=\"meta\">Exported from thinkt on %s</div>\n", time.Now().Format(time.RFC3339))

	// Toggle script
	fmt.Fprintf(w, `<script>
document.addEventListener('click', function(e) {
  var header = e.target.closest('.thinking-header, .tool-header');
  if (!header) return;
  var block = header.parentElement;
  block.classList.toggle('expanded');
});
</script>
`)

	fmt.Fprintf(w, "</body>\n</html>\n")
	return nil
}

func renderEntryHTML(w io.Writer, entry *thinkt.Entry, toolResults map[string]*thinkt.ContentBlock, inlined map[string]bool) {
	if len(entry.ContentBlocks) > 0 {
		for _, block := range entry.ContentBlocks {
			switch block.Type {
			case "text":
				if block.Text != "" {
					fmt.Fprintf(w, "    <div class=\"text\">%s</div>\n", html.EscapeString(block.Text))
				}
			case "thinking":
				fmt.Fprintf(w, "    <div class=\"thinking\">\n")
				fmt.Fprintf(w, "      <div class=\"thinking-header\">\n")
				fmt.Fprintf(w, "        <span class=\"thinking-toggle\">&#9658;</span>\n")
				fmt.Fprintf(w, "        <span class=\"thinking-label\">Thinking</span>\n")
				fmt.Fprintf(w, "      </div>\n")
				fmt.Fprintf(w, "      <div class=\"thinking-content\">%s</div>\n", html.EscapeString(block.Thinking))
				fmt.Fprintf(w, "    </div>\n")
			case "tool_use":
				summary := toolSummary(block.ToolName, block.ToolInput)
				inputJSON, _ := json.MarshalIndent(block.ToolInput, "        ", "  ")

				fmt.Fprintf(w, "    <div class=\"tool\">\n")
				fmt.Fprintf(w, "      <div class=\"tool-header\">\n")
				fmt.Fprintf(w, "        <span class=\"tool-toggle\">&#9658;</span>\n")
				fmt.Fprintf(w, "        <span class=\"tool-bullet\">&#9642;</span>\n")
				fmt.Fprintf(w, "        <span class=\"tool-name\">%s</span>\n", html.EscapeString(block.ToolName))
				if summary != "" {
					fmt.Fprintf(w, "        <span class=\"tool-summary\">%s</span>\n", html.EscapeString(summary))
				}
				fmt.Fprintf(w, "      </div>\n")
				fmt.Fprintf(w, "      <div class=\"tool-content\">%s</div>\n", html.EscapeString(string(inputJSON)))

				// Inline paired result
				if result, ok := toolResults[block.ToolUseID]; ok {
					inlined[block.ToolUseID] = true
					errClass := ""
					label := "Result"
					if result.IsError {
						errClass = " tool-result--error"
						label = "Error"
					}
					fmt.Fprintf(w, "      <div class=\"tool-result-inline%s\">\n", errClass)
					fmt.Fprintf(w, "        <div class=\"tool-header\">\n")
					fmt.Fprintf(w, "          <span class=\"tool-toggle\">&#9658;</span>\n")
					fmt.Fprintf(w, "          <span class=\"tool-label\">%s</span>\n", label)
					fmt.Fprintf(w, "        </div>\n")
					fmt.Fprintf(w, "        <div class=\"tool-content\">%s</div>\n", html.EscapeString(result.ToolResult))
					fmt.Fprintf(w, "      </div>\n")
				}

				fmt.Fprintf(w, "    </div>\n")
			case "tool_result":
				if !inlined[block.ToolUseID] {
					errClass := ""
					label := "Result"
					if block.IsError {
						errClass = " tool-error"
						label = "Error"
					}
					fmt.Fprintf(w, "    <div class=\"tool tool-result%s\">\n", errClass)
					fmt.Fprintf(w, "      <div class=\"tool-header\">\n")
					fmt.Fprintf(w, "        <span class=\"tool-toggle\">&#9658;</span>\n")
					fmt.Fprintf(w, "        <span class=\"tool-label\">%s</span>\n", label)
					fmt.Fprintf(w, "      </div>\n")
					fmt.Fprintf(w, "      <div class=\"tool-content\">%s</div>\n", html.EscapeString(block.ToolResult))
					fmt.Fprintf(w, "    </div>\n")
				}
			case "image":
				fmt.Fprintf(w, "    <div class=\"media media-image\">\n")
				fmt.Fprintf(w, "      <div class=\"media-header\">\n")
				fmt.Fprintf(w, "        <span class=\"media-label\">Image</span>\n")
				fmt.Fprintf(w, "        <span class=\"media-type\">%s</span>\n", html.EscapeString(block.MediaType))
				fmt.Fprintf(w, "      </div>\n")
				fmt.Fprintf(w, "    </div>\n")
			case "document":
				fmt.Fprintf(w, "    <div class=\"media media-document\">\n")
				fmt.Fprintf(w, "      <div class=\"media-header\">\n")
				fmt.Fprintf(w, "        <span class=\"media-label\">Document</span>\n")
				fmt.Fprintf(w, "        <span class=\"media-type\">%s</span>\n", html.EscapeString(block.MediaType))
				fmt.Fprintf(w, "      </div>\n")
				fmt.Fprintf(w, "    </div>\n")
			}
		}
		return
	}

	// Fallback: entry.Text for entries with no content blocks
	if entry.Text != "" {
		fmt.Fprintf(w, "    <div class=\"text\">%s</div>\n", html.EscapeString(entry.Text))
	}
}
