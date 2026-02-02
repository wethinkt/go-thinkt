package prompt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/sources/claude"
)

func TestDefaultTemplate(t *testing.T) {
	tmpl, err := DefaultTemplate()
	if err != nil {
		t.Fatalf("DefaultTemplate() error = %v", err)
	}
	if tmpl == nil {
		t.Fatal("DefaultTemplate() returned nil")
	}
}

func TestExecuteTemplate(t *testing.T) {
	tmpl, err := DefaultTemplate()
	if err != nil {
		t.Fatalf("DefaultTemplate() error = %v", err)
	}

	data := &TemplateData{
		Prompts: []claude.Prompt{
			{Text: "first prompt", Timestamp: "2026-01-24T10:00:00Z", UUID: "1"},
			{Text: "second prompt", Timestamp: "2026-01-24T10:00:01Z", UUID: "2"},
		},
		Count: 2,
	}

	var buf bytes.Buffer
	err = ExecuteTemplate(&buf, tmpl, data)
	if err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}

	output := buf.String()

	// Check for expected content
	if !strings.Contains(output, "first prompt") {
		t.Error("output missing first prompt")
	}
	if !strings.Contains(output, "second prompt") {
		t.Error("output missing second prompt")
	}
	if !strings.Contains(output, "2026-01-24T10:00:00Z") {
		t.Error("output missing timestamp")
	}
	if !strings.Contains(output, "---") {
		t.Error("output missing separator")
	}
}

func TestTemplateFuncs_formatTime(t *testing.T) {
	fn := templateFuncs["formatTime"].(func(string, string) string)

	got := fn("2026-01-24T10:30:45Z", "2006-01-02")
	want := "2026-01-24"
	if got != want {
		t.Errorf("formatTime() = %q, want %q", got, want)
	}

	// Invalid timestamp returns original
	got = fn("invalid", "2006-01-02")
	if got != "invalid" {
		t.Errorf("formatTime(invalid) = %q, want %q", got, "invalid")
	}
}

func TestTemplateFuncs_truncate(t *testing.T) {
	fn := templateFuncs["truncate"].(func(string, int) string)

	// Short string unchanged
	got := fn("hello", 10)
	if got != "hello" {
		t.Errorf("truncate(hello, 10) = %q, want %q", got, "hello")
	}

	// Long string truncated
	got = fn("hello world", 5)
	want := "hello..."
	if got != want {
		t.Errorf("truncate(hello world, 5) = %q, want %q", got, want)
	}
}

func TestTemplateFuncs_lineCount(t *testing.T) {
	fn := templateFuncs["lineCount"].(func(string) int)

	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello\nworld", 2},
		{"a\nb\nc", 3},
	}

	for _, tt := range tests {
		got := fn(tt.input)
		if got != tt.want {
			t.Errorf("lineCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestTemplateFuncs_wordCount(t *testing.T) {
	fn := templateFuncs["wordCount"].(func(string) int)

	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  hello   world  ", 2},
		{"one\ntwo\tthree", 3},
	}

	for _, tt := range tests {
		got := fn(tt.input)
		if got != tt.want {
			t.Errorf("wordCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestListEmbeddedTemplates(t *testing.T) {
	templates, err := ListEmbeddedTemplates()
	if err != nil {
		t.Fatalf("ListEmbeddedTemplates() error = %v", err)
	}

	if len(templates) == 0 {
		t.Error("ListEmbeddedTemplates() returned no templates")
	}

	// Should contain default template
	found := false
	for _, name := range templates {
		if name == "default.md.tmpl" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListEmbeddedTemplates() missing default.md.tmpl, got: %v", templates)
	}
}
