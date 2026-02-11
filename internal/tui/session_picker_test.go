package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestPickerSessionItemSessionTitle(t *testing.T) {
	t.Run("prefers summary over first prompt", func(t *testing.T) {
		item := pickerSessionItem{meta: thinkt.SessionMeta{
			Summary:     "Summary title",
			FirstPrompt: "First prompt title",
			ID:          "session-1",
		}}
		if got := item.sessionTitle(80); got != "Summary title" {
			t.Fatalf("expected summary title, got %q", got)
		}
	})

	t.Run("uses first prompt when summary missing", func(t *testing.T) {
		item := pickerSessionItem{meta: thinkt.SessionMeta{
			FirstPrompt: "First prompt title",
			ID:          "session-1",
		}}
		if got := item.sessionTitle(80); got != "First prompt title" {
			t.Fatalf("expected first prompt title, got %q", got)
		}
	})

	t.Run("ignores project id in first prompt", func(t *testing.T) {
		projectPath := filepath.Join(string(filepath.Separator), "tmp", "project-hash")
		item := pickerSessionItem{meta: thinkt.SessionMeta{
			FirstPrompt: projectPath,
			ProjectPath: projectPath,
			ID:          "session-123",
			FullPath:    filepath.Join(projectPath, "session-123.jsonl"),
		}}
		if got := item.sessionTitle(80); got != "session-123" {
			t.Fatalf("expected session id fallback, got %q", got)
		}
	})

	t.Run("falls back to file name when id is project id", func(t *testing.T) {
		projectPath := filepath.Join(string(filepath.Separator), "tmp", "project-hash")
		item := pickerSessionItem{meta: thinkt.SessionMeta{
			ProjectPath: projectPath,
			ID:          projectPath,
			FullPath:    filepath.Join(projectPath, "session-abc.jsonl"),
		}}
		if got := item.sessionTitle(80); got != "session-abc" {
			t.Fatalf("expected file-name fallback, got %q", got)
		}
	})

	t.Run("normalizes whitespace", func(t *testing.T) {
		item := pickerSessionItem{meta: thinkt.SessionMeta{
			FirstPrompt: "  hello\n\nworld\tagain  ",
		}}
		if got := item.sessionTitle(80); got != "hello world again" {
			t.Fatalf("expected normalized whitespace, got %q", got)
		}
	})

	t.Run("truncates by max length", func(t *testing.T) {
		item := pickerSessionItem{meta: thinkt.SessionMeta{
			FirstPrompt: strings.Repeat("a", 120),
		}}
		got := item.sessionTitle(20)
		if !strings.HasPrefix(got, strings.Repeat("a", 20)) || !strings.HasSuffix(got, "...") {
			t.Fatalf("expected truncated title with ellipsis, got %q", got)
		}
	})
}
