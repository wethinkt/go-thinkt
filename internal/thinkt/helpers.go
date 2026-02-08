// Package thinkt provides helper methods for working with session data.
package thinkt

import (
	"strings"
	"time"
)

// TextContent returns the text content of an entry.
// If the Text field is set, it returns that.
// Otherwise, it extracts text from ContentBlocks.
func (e *Entry) TextContent() string {
	// If Text field is explicitly set, use it
	if e.Text != "" {
		return e.Text
	}

	// Extract text from content blocks
	var texts []string
	for _, b := range e.ContentBlocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// HasThinking returns true if this entry contains thinking blocks.
func (e *Entry) HasThinking() bool {
	for _, b := range e.ContentBlocks {
		if b.Type == "thinking" && b.Thinking != "" {
			return true
		}
	}
	return false
}

// ThinkingBlocks returns all thinking content from this entry.
func (e *Entry) ThinkingBlocks() []ContentBlock {
	var blocks []ContentBlock
	for _, b := range e.ContentBlocks {
		if b.Type == "thinking" {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// ToolUses returns all tool_use blocks from this entry.
func (e *Entry) ToolUses() []ContentBlock {
	var blocks []ContentBlock
	for _, b := range e.ContentBlocks {
		if b.Type == "tool_use" {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// GetToolResult finds the tool_result block matching the given tool_use ID.
// This correlates tool calls with their results in the same or subsequent entries.
func (e *Entry) GetToolResult(toolUseID string) *ContentBlock {
	for _, b := range e.ContentBlocks {
		if b.Type == "tool_result" && b.ToolUseID == toolUseID {
			return &b
		}
	}
	return nil
}

// IsUserPrompt returns true if this is a user entry with non-empty text.
func (e *Entry) IsUserPrompt() bool {
	return e.Role == RoleUser && e.TextContent() != ""
}

// Session Helpers

// UserPrompts returns all user prompts from the session.
func (s *Session) UserPrompts() []Entry {
	var prompts []Entry
	for _, e := range s.Entries {
		if e.IsUserPrompt() {
			prompts = append(prompts, e)
		}
	}
	return prompts
}

// GetEntryByUUID finds an entry by its UUID.
func (s *Session) GetEntryByUUID(uuid string) *Entry {
	for i := range s.Entries {
		if s.Entries[i].UUID == uuid {
			return &s.Entries[i]
		}
	}
	return nil
}

// GetToolResult searches the session for a tool result matching the tool use ID.
// This is useful for correlating tool calls with their results across entries.
func (s *Session) GetToolResult(toolUseID string) *ContentBlock {
	for _, e := range s.Entries {
		if result := e.GetToolResult(toolUseID); result != nil {
			return result
		}
	}
	return nil
}

// Branches returns entries that are sidechains (branched conversations).
// Only applicable for Claude Code sessions with branching support.
func (s *Session) Branches() []Entry {
	var branches []Entry
	for _, e := range s.Entries {
		if e.IsSidechain {
			branches = append(branches, e)
		}
	}
	return branches
}

// RootEntries returns entries that have no parent (top-level entries).
func (s *Session) RootEntries() []Entry {
	var roots []Entry
	for _, e := range s.Entries {
		if e.ParentUUID == nil || *e.ParentUUID == "" {
			roots = append(roots, e)
		}
	}
	return roots
}

// Duration returns the time span from first to last entry.
func (s *Session) Duration() time.Duration {
	if len(s.Entries) < 2 {
		return 0
	}
	first := s.Entries[0].Timestamp
	last := s.Entries[len(s.Entries)-1].Timestamp
	return last.Sub(first)
}

// TotalTokenUsage returns total token usage across all entries.
func (s *Session) TotalTokenUsage() TokenUsage {
	var total TokenUsage
	for _, e := range s.Entries {
		if e.Usage != nil {
			total.InputTokens += e.Usage.InputTokens
			total.OutputTokens += e.Usage.OutputTokens
			total.CacheCreationInputTokens += e.Usage.CacheCreationInputTokens
			total.CacheReadInputTokens += e.Usage.CacheReadInputTokens
		}
	}
	return total
}
