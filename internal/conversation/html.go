package conversation

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

//go:embed conversation.css
var stylesheet string

// Stylesheet is shared by the browser and terminal HTML consumers. It uses a
// constrained block layout rather than relying on browser flex/grid behavior.
func Stylesheet() string { return stylesheet }

// View selects presentation without mutating content or source anchors.
type View struct {
	HiddenRoles  map[thinkt.Role]bool
	HiddenKinds  map[string]bool
	HiddenBlocks map[string]bool // block ID -> hidden, for role-specific filters
	Collapsed    map[string]bool // block ID -> collapsed
	// HiddenVisibility hides whole messages by their Visibility. WritePage
	// defaults it to hide Secondary; WriteHTML with nil hides nothing.
	HiddenVisibility map[thinkt.Visibility]bool
}

// WriteHTML writes an embeddable conversation fragment. Include Stylesheet in
// the host page. Raw HTML in Markdown is disabled; images are placeholders until
// the host explicitly resolves assets. No scripts or remote assets are emitted.
func WriteHTML(w io.Writer, doc *Document, view View) error {
	var out bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(mediaPlaceholder{}, 100))))
	visible := make(map[string]bool)
	for _, message := range doc.Messages {
		if view.messageVisible(message) {
			for _, block := range message.Blocks {
				visible[block.ID] = view.blockVisible(block)
			}
		}
	}
	out.WriteString("<article class=\"thinkt-conversation\">\n")
	for _, message := range doc.Messages {
		if !view.messageVisible(message) {
			continue
		}
		class := "entry role-" + string(message.Role)
		if message.Error != "" {
			class += " is-error"
		}
		fmt.Fprintf(&out, "<section class=\"%s\" id=\"%s\" data-session-id=\"%s\" data-entry-id=\"%s\" data-entry-index=\"%d\"", html.EscapeString(class), html.EscapeString(message.ID), html.EscapeString(message.Origin.SessionID), html.EscapeString(message.Origin.EntryUUID), message.Origin.EntryIndex)
		if message.TurnID != "" {
			fmt.Fprintf(&out, " data-turn-id=\"%s\"", html.EscapeString(message.TurnID))
		}
		if message.PromptID != "" {
			fmt.Fprintf(&out, " data-prompt-id=\"%s\"", html.EscapeString(message.PromptID))
		}
		if message.Kind != "" {
			fmt.Fprintf(&out, " data-kind=\"%s\"", html.EscapeString(string(message.Kind)))
		}
		out.WriteString(">\n")
		fmt.Fprintf(&out, "<h2 class=\"entry-header\">%s</h2>\n", html.EscapeString(roleLabel(message.Role)))
		var metadata []string
		if !message.Timestamp.IsZero() {
			metadata = append(metadata, message.Timestamp.Format(time.RFC3339))
		}
		if message.Source != "" {
			metadata = append(metadata, string(message.Source))
		}
		if message.Model != "" {
			metadata = append(metadata, message.Model)
		}
		if a := message.Attribution; a != nil {
			for _, part := range []string{a.Agent, a.Skill, a.Plugin, a.MCPServer} {
				if part != "" {
					metadata = append(metadata, part)
				}
			}
		}
		if message.StopReason == "refusal" {
			metadata = append(metadata, "refusal")
		}
		if message.Error != "" {
			metadata = append(metadata, "error: "+message.Error)
		}
		if len(metadata) > 0 {
			fmt.Fprintf(&out, "<p class=\"metadata\">%s</p>\n", html.EscapeString(strings.Join(metadata, " · ")))
		}
		for _, block := range message.Blocks {
			if !view.blockVisible(block) {
				continue
			}
			if err := writeBlock(&out, md, block, view, visible, false); err != nil {
				return err
			}
		}
		out.WriteString("</section>\n")
	}
	out.WriteString("</article>\n")
	_, err := w.Write(out.Bytes())
	return err
}

// writeBlock renders one block and, nested inside its body, its children.
func writeBlock(out *bytes.Buffer, md goldmark.Markdown, block Block, view View, visible map[string]bool, child bool) error {
	class := "block"
	if child {
		class += " block-child"
	}
	class += " block-" + block.Kind
	if block.IsError {
		class += " is-error"
	}
	if view.Collapsed[block.ID] {
		class += " is-collapsed"
	}
	fmt.Fprintf(out, "<section class=\"%s\" id=\"%s\">\n", html.EscapeString(class), html.EscapeString(block.ID))
	if block.Kind != "text" {
		label := blockLabel(block)
		if view.Collapsed[block.ID] {
			label += " (collapsed)"
		}
		fmt.Fprintf(out, "<h3 class=\"%s-header\">%s</h3>\n", html.EscapeString(block.Kind), html.EscapeString(label))
	}
	out.WriteString("<div class=\"block-body\">\n")
	switch block.Kind {
	case "text":
		if err := md.Convert([]byte(block.Text), out); err != nil {
			return fmt.Errorf("conversation: Markdown: %w", err)
		}
	case "image", "document":
		fmt.Fprintf(out, "<p class=\"media-placeholder\" data-media-id=\"%s\">%s — %s</p>\n", html.EscapeString(block.ID), blockLabel(block), html.EscapeString(strings.TrimSpace(block.MediaType+" "+block.MediaURI)))
	case thinkt.BlockTypeDiff:
		writeDiff(out, block.Text)
	default:
		fmt.Fprintf(out, "<pre><code>%s</code></pre>\n", html.EscapeString(block.Text))
	}
	for _, related := range block.Related {
		if !visible[related] {
			continue
		}
		fmt.Fprintf(out, "<p class=\"related\"><a href=\"#%s\">%s</a></p>\n", html.EscapeString(related), relatedLabel(block))
	}
	for _, nested := range block.Children {
		if !view.blockVisible(nested) {
			continue
		}
		if err := writeBlock(out, md, nested, view, visible, true); err != nil {
			return err
		}
	}
	out.WriteString("</div>\n</section>\n")
	return nil
}

// writeDiff renders unified diff text with one classed span per line. CSS
// only; no scripting.
func writeDiff(out *bytes.Buffer, text string) {
	out.WriteString("<pre class=\"diff\">")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, line := range lines {
		class := "diff-ctx"
		switch {
		case strings.HasPrefix(line, "@@"):
			class = "diff-hunk"
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			class = "diff-file"
		case strings.HasPrefix(line, "+"):
			class = "diff-add"
		case strings.HasPrefix(line, "-"):
			class = "diff-del"
		}
		if i > 0 {
			out.WriteByte('\n')
		}
		fmt.Fprintf(out, "<span class=\"%s\">%s</span>", class, html.EscapeString(line))
	}
	out.WriteString("</pre>\n")
}

func (v View) blockVisible(block Block) bool {
	return !v.HiddenKinds[block.Kind] && !v.HiddenBlocks[block.ID]
}

func (v View) messageVisible(message Message) bool {
	if v.HiddenRoles[message.Role] || v.HiddenVisibility[message.Visibility] {
		return false
	}
	if len(message.Blocks) == 0 {
		return true
	}
	for _, block := range message.Blocks {
		if v.blockVisible(block) {
			return true
		}
	}
	return false
}

// WritePage writes a self-contained page using the same fragment and stylesheet
// a terminal host can pass to nthtml. Content is expanded unless View says otherwise.
func WritePage(w io.Writer, doc *Document, view View) error {
	if view.HiddenVisibility == nil {
		view.HiddenVisibility = map[thinkt.Visibility]bool{thinkt.VisibilitySecondary: true}
	}
	var out bytes.Buffer
	out.WriteString("<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"UTF-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&out, "<title>%s</title>\n<style>%s</style></head><body class=\"thinkt-page\">\n<h1>%s</h1>\n", html.EscapeString(doc.Title), stylesheet, html.EscapeString(doc.Title))
	if err := WriteHTML(&out, doc, view); err != nil {
		return err
	}
	out.WriteString("<p class=\"metadata\">Exported from thinkt</p>\n</body></html>\n")
	_, err := w.Write(out.Bytes())
	return err
}

func roleLabel(role thinkt.Role) string {
	switch role {
	case thinkt.RoleUser:
		return "User"
	case thinkt.RoleAssistant:
		return "Assistant"
	case thinkt.RoleTool:
		return "Tool"
	case thinkt.RoleSystem:
		return "System"
	case thinkt.RoleSummary:
		return "Summary"
	case thinkt.RoleProgress:
		return "Progress"
	case thinkt.RoleCheckpoint:
		return "Checkpoint"
	default:
		return string(role)
	}
}

func blockLabel(block Block) string {
	switch block.Kind {
	case "thinking":
		return "Thinking"
	case "tool_use":
		return "Tool: " + block.ToolName
	case "tool_result":
		if block.IsError {
			return "Tool error"
		}
		return "Tool result"
	case "image":
		return "Image"
	case "document":
		return "Document"
	case thinkt.BlockTypeDiff:
		return "Diff"
	default:
		return "Content: " + block.Kind
	}
}

func relatedLabel(block Block) string {
	if block.Kind == "tool_use" {
		return "Tool result"
	}
	return "Tool call"
}

// A Markdown image must not make the browser fetch content that nthtml cannot
// access. Keep its alt text as a placeholder until both hosts have an asset resolver.
type mediaPlaceholder struct{}

func (mediaPlaceholder) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, func(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, err := w.WriteString("<span class=\"media-placeholder\">[Image: ")
			return ast.WalkContinue, err
		}
		_, err := w.WriteString("]</span>")
		return ast.WalkContinue, err
	})
}
