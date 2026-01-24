package prompt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

func TestFormatter_Markdown(t *testing.T) {
	prompts := []claude.Prompt{
		{Text: "hello", Timestamp: "2026-01-24T10:00:00Z"},
		{Text: "world", Timestamp: "2026-01-24T10:00:01Z"},
	}

	var buf bytes.Buffer
	f := NewFormatter(&buf, FormatMarkdown)
	if err := f.Write(prompts); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "## 2026-01-24T10:00:00Z") {
		t.Errorf("output missing timestamp header")
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("output missing prompt text")
	}
	if !strings.Contains(got, "---") {
		t.Errorf("output missing separator")
	}
}

func TestFormatter_JSON(t *testing.T) {
	prompts := []claude.Prompt{
		{Text: "test", Timestamp: "2026-01-24T10:00:00Z", UUID: "abc123"},
	}

	var buf bytes.Buffer
	f := NewFormatter(&buf, FormatJSON)
	if err := f.Write(prompts); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"Text": "test"`) {
		t.Errorf("output missing Text field, got: %s", got)
	}
	if !strings.Contains(got, `"Timestamp": "2026-01-24T10:00:00Z"`) {
		t.Errorf("output missing Timestamp field, got: %s", got)
	}
}

func TestFormatter_Plain(t *testing.T) {
	prompts := []claude.Prompt{
		{Text: "hello", Timestamp: "2026-01-24T10:00:00Z"},
		{Text: "world", Timestamp: "2026-01-24T10:00:01Z"},
	}

	var buf bytes.Buffer
	f := NewFormatter(&buf, FormatPlain)
	if err := f.Write(prompts); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("output missing prompt text, got: %s", got)
	}
	// Plain format should NOT contain timestamp headers
	if strings.Contains(got, "2026-01-24") {
		t.Errorf("plain format should not contain timestamps, got: %s", got)
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
		err   bool
	}{
		{"markdown", FormatMarkdown, false},
		{"md", FormatMarkdown, false},
		{"json", FormatJSON, false},
		{"plain", FormatPlain, false},
		{"text", FormatPlain, false},
		{"txt", FormatPlain, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		got, err := ParseFormat(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseFormat(%q) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseFormat(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
