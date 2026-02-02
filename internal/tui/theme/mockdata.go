// Package theme provides theming support for the TUI.
package theme

import (
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// MockEntries returns sample entries for theme preview.
func MockEntries() []thinkt.Entry {
	now := time.Now()
	return []thinkt.Entry{
		{
			UUID:      "mock-user-1",
			Role:      thinkt.RoleUser,
			Timestamp: now,
			Text:      "Can you help me understand how this code works?",
		},
		{
			UUID:      "mock-assistant-1",
			Role:      thinkt.RoleAssistant,
			Timestamp: now.Add(1 * time.Second),
			ContentBlocks: []thinkt.ContentBlock{
				{
					Type:     "thinking",
					Thinking: "Let me analyze the code structure and identify the key components...",
				},
				{
					Type: "text",
					Text: "I'll explain the code structure. Let me first read the main file to understand the architecture.",
				},
				{
					Type:      "tool_use",
					ToolUseID: "toolu_01ABC",
					ToolName:  "Read",
					ToolInput: map[string]any{"file_path": "/src/main.go"},
				},
			},
		},
		{
			UUID:      "mock-tool-result-1",
			Role:      thinkt.RoleTool,
			Timestamp: now.Add(2 * time.Second),
			ContentBlocks: []thinkt.ContentBlock{
				{
					Type:       "tool_result",
					ToolUseID:  "toolu_01ABC",
					ToolResult: "package main\n\nfunc main() {\n    // ...\n}",
				},
			},
		},
		{
			UUID:      "mock-assistant-2",
			Role:      thinkt.RoleAssistant,
			Timestamp: now.Add(3 * time.Second),
			ContentBlocks: []thinkt.ContentBlock{
				{
					Type: "text",
					Text: "This is a simple Go application with a main function. The structure follows standard Go conventions.",
				},
			},
		},
	}
}

// MockSession returns a sample session for theme preview.
func MockSession() *thinkt.Session {
	entries := MockEntries()
	return &thinkt.Session{
		Meta: thinkt.SessionMeta{
			ID:          "mock-session",
			ProjectPath: "/example/project",
			FullPath:    "/example/project/session.jsonl",
			FirstPrompt: "Can you help me understand how this code works?",
			EntryCount:  len(entries),
			CreatedAt:   time.Now(),
			ModifiedAt:  time.Now(),
			Source:      thinkt.SourceClaude,
		},
		Entries: entries,
	}
}
