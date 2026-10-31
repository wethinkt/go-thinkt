package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// convertToolResultContent decodes a tool_result's content, which is a string
// or an array of blocks. The returned text joins the text children; every
// array element becomes a child so images and unfamiliar block types survive.
// mediaBlock converts an image or document source, keeping Claude's block
// type.
func mediaBlock(blockType string, src *MediaSource) thinkt.ContentBlock {
	b := thinkt.MediaBlock(src.MediaType, src.Data)
	b.Type = blockType
	return b
}

func convertToolResultContent(raw json.RawMessage) (string, []thinkt.ContentBlock) {
	if len(raw) == 0 {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var elements []json.RawMessage
	if json.Unmarshal(raw, &elements) != nil {
		return string(raw), nil
	}
	var texts []string
	children := make([]thinkt.ContentBlock, 0, len(elements))
	for _, element := range elements {
		var block ContentBlock
		if json.Unmarshal(element, &block) != nil {
			children = append(children, thinkt.ContentBlock{Type: "unknown", Text: string(element)})
			continue
		}
		switch block.Type {
		case "text":
			texts = append(texts, block.Text)
			children = append(children, thinkt.ContentBlock{Type: "text", Text: block.Text})
		case "image", "document":
			child := thinkt.ContentBlock{Type: block.Type}
			if block.Source != nil {
				child = mediaBlock(block.Type, block.Source)
			}
			children = append(children, child)
		default:
			children = append(children, thinkt.ContentBlock{Type: block.Type, Text: rawText(element)})
		}
	}
	return strings.Join(texts, "\n"), children
}

// toolUseResult is the typed, model-invisible result Claude Code records next
// to a tool_result turn. Only the fields the conversation uses are decoded.
type toolUseResult struct {
	Stdout              string      `json:"stdout"`
	Stderr              string      `json:"stderr"`
	Interrupted         bool        `json:"interrupted"`
	PersistedOutputPath string      `json:"persistedOutputPath"`
	FilePath            string      `json:"filePath"`
	StructuredPatch     []patchHunk `json:"structuredPatch"`
	AgentID             string      `json:"agentId"`
	Status              string      `json:"status"`
}

// patchHunk is one jsdiff hunk from structuredPatch.
type patchHunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// parseToolUseResult decodes an object-shaped toolUseResult. Strings, arrays,
// null, and empty values report false.
func parseToolUseResult(raw json.RawMessage) (toolUseResult, bool) {
	var r toolUseResult
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "{") {
		return r, false
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return r, false
	}
	return r, true
}

// unifiedDiff renders structuredPatch hunks as unified diff text. Lines keep
// the " ", "+", "-" prefixes Claude Code wrote; no context is recomputed.
func unifiedDiff(filePath string, hunks []patchHunk) string {
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", filePath, filePath)
	for _, h := range hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, line := range h.Lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// applyToolUseResult enriches the tool_result block and its entry with the
// typed result: a diff child, stdout/stderr when the inline content is empty,
// the subagent session id, and spill/interrupt metadata.
func applyToolUseResult(block *thinkt.ContentBlock, entry *thinkt.Entry, r toolUseResult) {
	if len(r.StructuredPatch) > 0 {
		block.Children = append(block.Children, thinkt.ContentBlock{Type: thinkt.BlockTypeDiff, Text: unifiedDiff(r.FilePath, r.StructuredPatch)})
	}
	if block.ToolResult == "" && (r.Stdout != "" || r.Stderr != "") {
		block.ToolResult = r.Stdout
		if r.Stderr != "" {
			block.Children = append(block.Children, thinkt.ContentBlock{Type: "text", Text: r.Stderr})
		}
	}
	if r.AgentID != "" {
		id := "agent-" + r.AgentID
		if session, _ := entry.Metadata["session_id"].(string); session != "" {
			id = session + "/" + id
		}
		block.ChildSessionID = id
		entry.ChildSessionID = id
	}
	if r.PersistedOutputPath != "" {
		entry.Metadata["persisted_output_path"] = r.PersistedOutputPath
	}
	if r.Interrupted {
		entry.Metadata["interrupted"] = true
	}
}
