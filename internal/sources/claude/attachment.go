package claude

import (
	"encoding/json"
	"fmt"
	"strings"
)

// renderedAttachment is one element of an attachment record's rendered array:
// the exact text that was injected into the model context.
type renderedAttachment struct {
	Content string `json:"content"`
}

// attachmentText produces display text for a context attachment. Typed
// payloads get a short, specific rendering; anything else falls back to the
// rendered context text, then to the pretty-printed payload.
func attachmentText(attachmentType string, payload json.RawMessage, rendered []string) string {
	if text, ok := typedAttachmentText(attachmentType, payload); ok {
		return text
	}
	if len(rendered) > 0 {
		return strings.Join(rendered, "\n")
	}
	return rawText(payload)
}

func typedAttachmentText(attachmentType string, payload json.RawMessage) (string, bool) {
	switch attachmentType {
	case "hook_success":
		var a struct{ HookName, Stdout, Stderr string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		text := a.HookName + ": " + a.Stdout
		if a.Stderr != "" {
			text += "\n\n" + a.Stderr
		}
		return text, true
	case "hook_additional_context":
		var a struct{ Content []string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		return strings.Join(a.Content, "\n"), true
	case "queued_command":
		var a struct{ Prompt string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		return a.Prompt, true
	case "task_status":
		var a struct{ Description, Status string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		return a.Description + " — " + a.Status, true
	case "instructions":
		var a struct{ Files []struct{ Path string } }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		paths := make([]string, 0, len(a.Files))
		for _, f := range a.Files {
			paths = append(paths, f.Path)
		}
		return strings.Join(paths, "\n"), true
	case "file", "edited_text_file":
		var a struct{ Filename string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		return a.Filename, true
	case "invoked_skills":
		var a struct{ Skills []struct{ Name string } }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		names := make([]string, 0, len(a.Skills))
		for _, s := range a.Skills {
			names = append(names, s.Name)
		}
		return strings.Join(names, ", "), true
	case "skill_listing":
		var a struct{ Content string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		return a.Content, true
	case "diagnostics":
		var a struct {
			Files []struct {
				URI         string `json:"uri"`
				Diagnostics []struct{ Message string }
			}
		}
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d files with diagnostics", len(a.Files))
		for _, f := range a.Files {
			for _, d := range f.Diagnostics {
				b.WriteString("\n" + f.URI + ": " + d.Message)
			}
		}
		return b.String(), true
	case "compact_file_reference":
		var a struct{ Filename, DisplayPath string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		if a.DisplayPath != "" {
			return a.DisplayPath, true
		}
		return a.Filename, true
	case "read_truncation_notice":
		var a struct{ Banner string }
		if json.Unmarshal(payload, &a) != nil {
			return "", false
		}
		return a.Banner, true
	}
	return "", false
}

// attachmentToolUseID extracts the toolUseID an attachment refers to, if any.
func attachmentToolUseID(payload json.RawMessage) string {
	var a struct {
		ToolUseID string `json:"toolUseID"`
	}
	_ = json.Unmarshal(payload, &a)
	return a.ToolUseID
}
