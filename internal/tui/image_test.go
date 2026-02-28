package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestRenderImageOnlyUserEntry(t *testing.T) {
	// 1x1 red PNG, valid base64
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/58BAwAI/AL+hc2rNAAAAABJRU5ErkJggg=="

	entry := &thinkt.Entry{
		Role: thinkt.RoleUser,
		ContentBlocks: []thinkt.ContentBlock{
			{
				Type:      "image",
				MediaType: "image/png",
				MediaData: b64,
			},
		},
	}

	result := RenderThinktEntry(entry, 80, nil)
	if result == "" {
		t.Fatal("image-only user entry rendered as empty")
	}
	if !strings.Contains(result, "Image") {
		t.Errorf("expected 'Image' label, got: %s", result)
	}
	if !strings.Contains(result, "image/png") {
		t.Errorf("expected 'image/png' in output, got: %s", result)
	}
	t.Logf("Rendered:\n%s", result)
}

func TestRenderUserEntryWithTextAndImage(t *testing.T) {
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/58BAwAI/AL+hc2rNAAAAABJRU5ErkJggg=="

	entry := &thinkt.Entry{
		Role: thinkt.RoleUser,
		Text: "Check this screenshot",
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "Check this screenshot"},
			{Type: "image", MediaType: "image/png", MediaData: b64},
		},
	}

	result := RenderThinktEntry(entry, 80, nil)
	if !strings.Contains(result, "Check this screenshot") {
		t.Error("missing text content")
	}
	if !strings.Contains(result, "image/png") {
		t.Error("missing image placeholder")
	}
	t.Logf("Rendered:\n%s", result)
}

func TestRenderAssistantImageBlock(t *testing.T) {
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/58BAwAI/AL+hc2rNAAAAABJRU5ErkJggg=="

	entry := &thinkt.Entry{
		Role: thinkt.RoleAssistant,
		ContentBlocks: []thinkt.ContentBlock{
			{Type: "text", Text: "Here is the image:"},
			{Type: "image", MediaType: "image/png", MediaData: b64},
		},
	}

	result := RenderThinktEntry(entry, 80, nil)
	if !strings.Contains(result, "image/png") {
		t.Error("missing image placeholder in assistant entry")
	}
	t.Logf("Rendered:\n%s", result)
}

func TestDecodeImageDimensions(t *testing.T) {
	// 1x1 PNG
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/58BAwAI/AL+hc2rNAAAAABJRU5ErkJggg=="
	dims := decodeImageDimensions("image/png", b64)
	if dims != "1x1" {
		t.Errorf("expected 1x1, got %q", dims)
	}
}

func TestKittyPlaceholderGrid(t *testing.T) {
	grid := kittyPlaceholderGrid(42, 3, 2)

	lines := strings.Split(grid, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(lines))
	}

	// Each row should contain 3 placeholder characters
	for i, line := range lines {
		count := strings.Count(line, string(kitty.Placeholder))
		if count != 3 {
			t.Errorf("row %d: expected 3 placeholders, got %d", i, count)
		}
	}

	// Should contain foreground color escape for ID 42 (R=0, G=0, B=42)
	if !strings.Contains(grid, "\x1b[38;2;0;0;42m") {
		t.Error("missing foreground color encoding for image ID 42")
	}

	// Should contain reset
	if !strings.Contains(grid, "\x1b[39m") {
		t.Error("missing foreground color reset")
	}
}

func TestImageDisplaySize(t *testing.T) {
	// 1x1 PNG — should give 1 column, 1 row
	b64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/58BAwAI/AL+hc2rNAAAAABJRU5ErkJggg=="
	cols, rows := imageDisplaySize(b64, 80)
	if cols < 1 || rows < 1 {
		t.Errorf("expected positive dimensions, got %dx%d", cols, rows)
	}
}

func TestImageTrackerAssignID(t *testing.T) {
	tracker := &imageTracker{
		assignments: make(map[string]int32),
	}

	id1 := tracker.assignImageID("abc123", 10, 5)
	id2 := tracker.assignImageID("abc123", 10, 5) // same content
	id3 := tracker.assignImageID("def456", 10, 5) // different content

	if id1 != id2 {
		t.Errorf("same content should get same ID: %d != %d", id1, id2)
	}
	if id1 == id3 {
		t.Error("different content should get different IDs")
	}

	pending := tracker.drainPending()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending images, got %d", len(pending))
	}

	// Drain again should be empty
	pending = tracker.drainPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after drain, got %d", len(pending))
	}
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{500, "500B"},
		{1500, "2KB"},
		{1_500_000, "1.5MB"},
	}
	for _, tt := range tests {
		got := formatByteSize(tt.n)
		if got != tt.want {
			t.Errorf("formatByteSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
