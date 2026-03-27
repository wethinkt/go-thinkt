package summarize

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestExtractThinkingText(t *testing.T) {
	longThinking := strings.Repeat("a", 60)
	entry := thinkt.Entry{
		Role: thinkt.RoleAssistant,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "Here is my answer"},
			{Type: "thinking", Thinking: longThinking},
		},
	}
	result := ExtractThinkingText(entry)
	if result != longThinking {
		t.Errorf("expected thinking text, got %q", result)
	}
	if strings.Contains(result, "Here is my answer") {
		t.Error("should not include text blocks")
	}
}

func TestExtractThinkingTextSkipsShort(t *testing.T) {
	entry := thinkt.Entry{
		Role: thinkt.RoleAssistant,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: "short"},
		},
	}
	result := ExtractThinkingText(entry)
	if result != "" {
		t.Errorf("expected empty string for short thinking, got %q", result)
	}
}

func TestExtractThinkingTextSkipsUser(t *testing.T) {
	longThinking := strings.Repeat("a", 60)
	entry := thinkt.Entry{
		Role: thinkt.RoleUser,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "thinking", Thinking: longThinking},
		},
	}
	result := ExtractThinkingText(entry)
	if result != "" {
		t.Errorf("expected empty string for user entry, got %q", result)
	}
}

func TestExtractSessionContext(t *testing.T) {
	longThinking := strings.Repeat("b", 60)
	entries := []thinkt.Entry{
		{
			Role: thinkt.RoleUser,
			Text: "Fix the login bug",
		},
		{
			Role: thinkt.RoleAssistant,
			ContentBlocks: []thinkt.ContentBlock{
				{Type: "thinking", Thinking: longThinking},
			},
		},
	}
	result := ExtractSessionContext(entries)
	if !strings.Contains(result, "[user] Fix the login bug") {
		t.Error("should contain [user] prefixed text")
	}
	if !strings.Contains(result, "[thinking] "+longThinking) {
		t.Error("should contain [thinking] prefixed text")
	}
}

func TestTruncate(t *testing.T) {
	// Short string unchanged
	short := "hello"
	if truncate(short, 10) != "hello" {
		t.Errorf("short string should be unchanged, got %q", truncate(short, 10))
	}

	// Long string truncated with ellipsis
	long := strings.Repeat("x", 20)
	result := truncate(long, 10)
	if len(result) != 13 { // 10 + "..."
		t.Errorf("expected length 13, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated string should end with ...")
	}
}
