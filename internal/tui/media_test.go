package tui

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestUserEntryShowsMediaLocation(t *testing.T) {
	block := thinkt.MediaBlockURI("image/png", "https://e.com/x.png")
	got := renderMediaBlocks([]thinkt.ContentBlock{block}, 60)
	if len(got) != 1 || !strings.Contains(got[0], "https://e.com/x.png") {
		t.Fatalf("%q", got)
	}
}
