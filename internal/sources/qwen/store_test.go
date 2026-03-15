package qwen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// TestStore_ExtractProjectName_FromDecodedPath tests project name extraction
// when the project hash can be decoded to a path.
func TestStore_ExtractProjectName_FromDecodedPath(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Test dash-encoded path decoding
	projectHash := "-Users-evan-myproject"
	name := store.extractProjectName(projectHash)
	if name != "myproject" {
		t.Errorf("expected 'myproject', got %q", name)
	}
}

// TestStore_ExtractProjectName_FromCWD tests project name extraction
// by reading CWD from session files when decoding fails.
func TestStore_ExtractProjectName_FromCWD(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project structure with a non-decodable hash
	projectHash := "abc123def456"
	chatsDir := filepath.Join(tmpDir, "projects", projectHash, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with CWD
	sessionFile := filepath.Join(chatsDir, "session1.jsonl")
	entry := map[string]any{
		"uuid":      "test-uuid",
		"type":      "user",
		"cwd":       "/Users/evan/test-project",
		"timestamp": time.Now().Format(time.RFC3339),
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"text": "Hello"},
			},
		},
	}
	line, _ := json.Marshal(entry)
	if err := os.WriteFile(sessionFile, []byte(string(line)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(tmpDir)
	name := store.extractProjectName(projectHash)
	if name != "test-project" {
		t.Errorf("expected 'test-project', got %q", name)
	}
}

// TestStore_ExtractProjectName_EmptyCWD tests when CWD is empty or missing.
func TestStore_ExtractProjectName_EmptyCWD(t *testing.T) {
	tmpDir := t.TempDir()

	projectHash := "abc123"
	chatsDir := filepath.Join(tmpDir, "projects", projectHash, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create session without CWD
	sessionFile := filepath.Join(chatsDir, "session1.jsonl")
	entry := map[string]any{
		"uuid":      "test-uuid",
		"type":      "user",
		"timestamp": time.Now().Format(time.RFC3339),
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"text": "Hello"},
			},
		},
	}
	line, _ := json.Marshal(entry)
	if err := os.WriteFile(sessionFile, []byte(string(line)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(tmpDir)
	name := store.extractProjectName(projectHash)
	if name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

// TestParseQwenEntry_UserMessage tests parsing user message entries.
func TestParseQwenEntry_UserMessage(t *testing.T) {
	entry := map[string]any{
		"uuid":      "user-uuid-123",
		"type":      "user",
		"timestamp": "2026-03-15T10:00:00Z",
		"cwd":       "/Users/evan/test",
		"gitBranch": "main",
		"version":   "0.12.3",
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"text": "Hello, how are you?"},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.UUID != "user-uuid-123" {
		t.Errorf("expected UUID 'user-uuid-123', got %q", parsed.UUID)
	}
	if parsed.Role != thinkt.RoleUser {
		t.Errorf("expected role User, got %v", parsed.Role)
	}
	if parsed.Text != "Hello, how are you?" {
		t.Errorf("expected text 'Hello, how are you?', got %q", parsed.Text)
	}
	if parsed.CWD != "/Users/evan/test" {
		t.Errorf("expected CWD '/Users/evan/test', got %q", parsed.CWD)
	}
	if parsed.GitBranch != "main" {
		t.Errorf("expected git branch 'main', got %q", parsed.GitBranch)
	}
}

// TestParseQwenEntry_AssistantWithThinking tests parsing assistant messages with thinking blocks.
func TestParseQwenEntry_AssistantWithThinking(t *testing.T) {
	entry := map[string]any{
		"uuid":      "assistant-uuid-456",
		"type":      "assistant",
		"model":     "coder-model",
		"timestamp": "2026-03-15T10:00:01Z",
		"message": map[string]any{
			"role": "model",
			"parts": []any{
				map[string]any{
					"text":    "Let me think about this...",
					"thought": true,
				},
				map[string]any{
					"text":    "Here's my answer",
					"thought": false,
				},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != thinkt.RoleAssistant {
		t.Errorf("expected role Assistant, got %v", parsed.Role)
	}
	if parsed.Model != "coder-model" {
		t.Errorf("expected model 'coder-model', got %q", parsed.Model)
	}
	if len(parsed.ContentBlocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(parsed.ContentBlocks))
	}
	if parsed.ContentBlocks[0].Type != "thinking" {
		t.Errorf("expected first block to be thinking, got %q", parsed.ContentBlocks[0].Type)
	}
	if parsed.ContentBlocks[0].Thinking != "Let me think about this..." {
		t.Errorf("expected thinking content, got %q", parsed.ContentBlocks[0].Thinking)
	}
	if parsed.ContentBlocks[1].Type != "text" {
		t.Errorf("expected second block to be text, got %q", parsed.ContentBlocks[1].Type)
	}
}

// TestParseQwenEntry_ToolCall tests parsing assistant messages with function calls.
func TestParseQwenEntry_ToolCall(t *testing.T) {
	entry := map[string]any{
		"uuid":      "tool-call-uuid",
		"type":      "assistant",
		"model":     "coder-model",
		"timestamp": "2026-03-15T10:00:02Z",
		"message": map[string]any{
			"role": "model",
			"parts": []any{
				map[string]any{
					"text": "I'll call a function",
				},
				map[string]any{
					"functionCall": map[string]any{
						"id":   "call_abc123",
						"name": "list_directory",
						"args": map[string]any{
							"path": "/Users/evan/test",
						},
					},
				},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(parsed.ContentBlocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(parsed.ContentBlocks))
	}

	toolBlock := parsed.ContentBlocks[1]
	if toolBlock.Type != "tool_use" {
		t.Errorf("expected tool_use block, got %q", toolBlock.Type)
	}
	if toolBlock.ToolUseID != "call_abc123" {
		t.Errorf("expected tool ID 'call_abc123', got %q", toolBlock.ToolUseID)
	}
	if toolBlock.ToolName != "list_directory" {
		t.Errorf("expected tool name 'list_directory', got %q", toolBlock.ToolName)
	}
	if toolBlock.ToolInput == nil {
		t.Error("expected tool input to be set")
	}
}

// TestParseQwenEntry_ToolResult tests parsing tool result entries.
func TestParseQwenEntry_ToolResult(t *testing.T) {
	entry := map[string]any{
		"uuid":      "tool-result-uuid",
		"type":      "tool_result",
		"timestamp": "2026-03-15T10:00:03Z",
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{
					"functionResponse": map[string]any{
						"id":   "call_abc123",
						"name": "list_directory",
						"response": map[string]any{
							"output": "file1.txt\nfile2.txt",
						},
					},
				},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != thinkt.RoleTool {
		t.Errorf("expected role Tool, got %v", parsed.Role)
	}
	if len(parsed.ContentBlocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(parsed.ContentBlocks))
	}

	resultBlock := parsed.ContentBlocks[0]
	if resultBlock.Type != "tool_result" {
		t.Errorf("expected tool_result block, got %q", resultBlock.Type)
	}
	if resultBlock.ToolUseID != "call_abc123" {
		t.Errorf("expected tool ID 'call_abc123', got %q", resultBlock.ToolUseID)
	}
	if resultBlock.ToolResult != "file1.txt\nfile2.txt" {
		t.Errorf("expected tool result 'file1.txt\\nfile2.txt', got %q", resultBlock.ToolResult)
	}
}

// TestParseQwenEntry_SystemSlashCommand tests parsing system entries with slash commands.
func TestParseQwenEntry_SystemSlashCommand(t *testing.T) {
	entry := map[string]any{
		"uuid":      "system-uuid",
		"type":      "system",
		"subtype":   "slash_command",
		"timestamp": "2026-03-15T10:00:04Z",
		"systemPayload": map[string]any{
			"rawCommand": "/model",
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != thinkt.RoleSystem {
		t.Errorf("expected role System, got %v", parsed.Role)
	}
	if parsed.Text != "Slash command: /model" {
		t.Errorf("expected text 'Slash command: /model', got %q", parsed.Text)
	}
	if parsed.Metadata["subtype"] != "slash_command" {
		t.Errorf("expected subtype 'slash_command', got %v", parsed.Metadata["subtype"])
	}
}

// TestParseQwenEntry_SystemTelemetry tests parsing system telemetry entries.
func TestParseQwenEntry_SystemTelemetry(t *testing.T) {
	entry := map[string]any{
		"uuid":      "system-telemetry-uuid",
		"type":      "system",
		"subtype":   "ui_telemetry",
		"timestamp": "2026-03-15T10:00:05Z",
		"systemPayload": map[string]any{
			"uiEvent": map[string]any{
				"event.name": "qwen-code.api_response",
				"model":      "coder-model",
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Role != thinkt.RoleSystem {
		t.Errorf("expected role System, got %v", parsed.Role)
	}
	if parsed.Metadata["subtype"] != "ui_telemetry" {
		t.Errorf("expected subtype 'ui_telemetry', got %v", parsed.Metadata["subtype"])
	}
}

// TestParseQwenEntry_WithParentUUID tests entries with parent UUID for threading.
func TestParseQwenEntry_WithParentUUID(t *testing.T) {
	entry := map[string]any{
		"uuid":       "child-uuid",
		"parentUuid": "parent-uuid",
		"type":       "user",
		"timestamp":  "2026-03-15T10:00:06Z",
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"text": "Follow-up question"},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.ParentUUID == nil {
		t.Fatal("expected ParentUUID to be set")
	}
	if *parsed.ParentUUID != "parent-uuid" {
		t.Errorf("expected parent UUID 'parent-uuid', got %q", *parsed.ParentUUID)
	}
}

// TestParseQwenEntry_WithUsageMetadata tests parsing assistant entries with usage metadata.
func TestParseQwenEntry_WithUsageMetadata(t *testing.T) {
	entry := map[string]any{
		"uuid":      "assistant-with-usage",
		"type":      "assistant",
		"model":     "coder-model",
		"timestamp": "2026-03-15T10:00:07Z",
		"usageMetadata": map[string]any{
			"promptTokenCount":        100,
			"candidatesTokenCount":    50,
			"thoughtsTokenCount":      10,
			"totalTokenCount":         160,
			"cachedContentTokenCount": 80,
		},
		"message": map[string]any{
			"role": "model",
			"parts": []any{
				map[string]any{"text": "Response"},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	usageMeta, ok := parsed.Metadata["usageMetadata"]
	if !ok {
		t.Fatal("expected usageMetadata to be present")
	}

	// usageMetadata is stored as a struct, check it exists and has expected fields
	usageMap, ok := usageMeta.(map[string]any)
	if ok {
		// If it's a map, check values
		if usageMap["promptTokenCount"] != float64(100) {
			t.Errorf("expected promptTokenCount 100, got %v", usageMap["promptTokenCount"])
		}
		if usageMap["totalTokenCount"] != float64(160) {
			t.Errorf("expected totalTokenCount 160, got %v", usageMap["totalTokenCount"])
		}
	} else {
		// If it's a struct, just verify it exists (the struct fields are private)
		if usageMeta == nil {
			t.Error("expected usageMetadata struct to be non-nil")
		}
	}
}

// TestParseQwenEntry_MalformedJSON tests handling of malformed JSON entries.
func TestParseQwenEntry_MalformedJSON(t *testing.T) {
	line := []byte(`{"uuid": "test", "type": "user", invalid json}`)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
	if parsed != nil {
		t.Error("expected nil result for malformed JSON")
	}
}

// TestParseQwenEntry_EmptyMessage tests handling of empty message content.
func TestParseQwenEntry_EmptyMessage(t *testing.T) {
	entry := map[string]any{
		"uuid":      "empty-message-uuid",
		"type":      "user",
		"timestamp": "2026-03-15T10:00:08Z",
		"message": map[string]any{
			"role":  "user",
			"parts": []any{},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed.Text != "" {
		t.Errorf("expected empty text, got %q", parsed.Text)
	}
	if len(parsed.ContentBlocks) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(parsed.ContentBlocks))
	}
}

// TestParseQwenEntry_MultipleTextParts tests messages with multiple text parts.
func TestParseQwenEntry_MultipleTextParts(t *testing.T) {
	entry := map[string]any{
		"uuid":      "multi-part-uuid",
		"type":      "user",
		"timestamp": "2026-03-15T10:00:09Z",
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"text": "First part"},
				map[string]any{"text": "Second part"},
				map[string]any{"text": "Third part"},
			},
		},
	}
	line, _ := json.Marshal(entry)

	parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedText := "First part\nSecond part\nThird part"
	if parsed.Text != expectedText {
		t.Errorf("expected text %q, got %q", expectedText, parsed.Text)
	}
	if len(parsed.ContentBlocks) != 3 {
		t.Errorf("expected 3 content blocks, got %d", len(parsed.ContentBlocks))
	}
}

// TestParseQwenEntry_TimestampParsing tests various timestamp formats.
func TestParseQwenEntry_TimestampParsing(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		wantValid bool
	}{
		{"RFC3339", "2026-03-15T10:00:00Z", true},
		{"RFC3339Nano", "2026-03-15T10:00:00.123456789Z", true},
		{"WithOffset", "2026-03-15T10:00:00+05:00", true},
		{"Invalid", "not-a-timestamp", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := map[string]any{
				"uuid":      "timestamp-test-uuid",
				"type":      "user",
				"timestamp": tt.timestamp,
				"message": map[string]any{
					"role": "user",
					"parts": []any{
						map[string]any{"text": "Test"},
					},
				},
			}
			line, _ := json.Marshal(entry)

			parsed, err := parseQwenEntry(line, 1, thinkt.SourceQwen, "workspace-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantValid && parsed.Timestamp.IsZero() {
				t.Error("expected valid timestamp, got zero")
			}
			if !tt.wantValid && !parsed.Timestamp.IsZero() {
				t.Errorf("expected zero timestamp for invalid input, got %v", parsed.Timestamp)
			}
		})
	}
}

// TestDecodeProjectPath tests the project path decoding logic.
func TestDecodeProjectPath(t *testing.T) {
	tests := []struct {
		hash string
		want string
	}{
		{
			hash: "-Users-evan-wethinkt-go-thinkt",
			want: "Users/evan/wethinkt/go/thinkt", // dashes become slashes
		},
		{
			hash: "-home-user-project",
			want: "home/user/project",
		},
		{
			hash: "random-hash-abc123",
			want: "",
		},
		{
			hash: "",
			want: "",
		},
	}

	store := &Store{baseDir: t.TempDir()}
	for _, tt := range tests {
		got := store.decodeProjectPath(tt.hash)
		if got != tt.want {
			t.Errorf("decodeProjectPath(%q) = %q, want %q", tt.hash, got, tt.want)
		}
	}
}

// TestStore_ListProjects_EmptyDirectory tests listing projects when none exist.
func TestStore_ListProjects_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "projects"), 0755); err != nil {
		t.Fatal(err)
	}

	store := NewStore(tmpDir)
	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

// TestStore_ListProjects_WithSessions tests listing projects that have sessions.
func TestStore_ListProjects_WithSessions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project structure
	projectHash := "-Users-evan-testproject"
	chatsDir := filepath.Join(tmpDir, "projects", projectHash, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file
	sessionFile := filepath.Join(chatsDir, "session1.jsonl")
	entry := map[string]any{
		"uuid":      "test-uuid",
		"type":      "user",
		"cwd":       "/Users/evan/testproject",
		"timestamp": time.Now().Format(time.RFC3339),
		"message": map[string]any{
			"role": "user",
			"parts": []any{
				map[string]any{"text": "Hello"},
			},
		},
	}
	line, _ := json.Marshal(entry)
	if err := os.WriteFile(sessionFile, []byte(string(line)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(tmpDir)
	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	p := projects[0]
	if p.ID != projectHash {
		t.Errorf("expected project ID %q, got %q", projectHash, p.ID)
	}
	if p.Name != "testproject" {
		t.Errorf("expected project name 'testproject', got %q", p.Name)
	}
	if p.SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", p.SessionCount)
	}
}

// TestStore_ReadSessionMeta tests reading session metadata.
func TestStore_ReadSessionMeta(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a session file with various metadata
	sessionFile := filepath.Join(tmpDir, "session.jsonl")
	entries := []map[string]any{
		{
			"uuid":      "uuid-1",
			"type":      "user",
			"cwd":       "/Users/evan/test",
			"gitBranch": "main",
			"timestamp": "2026-03-15T10:00:00Z",
			"message": map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "First prompt in the session"},
				},
			},
		},
		{
			"uuid":      "uuid-2",
			"type":      "assistant",
			"model":     "coder-model",
			"timestamp": "2026-03-15T10:00:01Z",
			"message": map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{"text": "Response"},
				},
			},
		},
	}

	var lines []string
	for _, e := range entries {
		line, _ := json.Marshal(e)
		lines = append(lines, string(line))
	}
	if err := os.WriteFile(sessionFile, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(tmpDir)
	meta, err := store.readSessionMeta(sessionFile, "project-hash", 1024, time.Now(), "workspace-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if meta.FirstPrompt != "First prompt in the session" {
		t.Errorf("expected first prompt 'First prompt in the session', got %q", meta.FirstPrompt)
	}
	if meta.Model != "coder-model" {
		t.Errorf("expected model 'coder-model', got %q", meta.Model)
	}
	// Note: GitBranch is not currently extracted in readSessionMeta
	// but is available in the full Entry parsing
}
