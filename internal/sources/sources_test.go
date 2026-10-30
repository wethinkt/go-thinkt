package sources

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestSearchLineFilterersImplemented(t *testing.T) {
	want := map[thinkt.Source]bool{
		thinkt.SourceClaude:      true,
		thinkt.SourceCodex:       true,
		thinkt.SourceKimi:        true,
		thinkt.SourceAntigravity: true,
		thinkt.SourceGemini:      false,
		thinkt.SourceCopilot:     false,
		thinkt.SourceQwen:        false,
	}
	for s, has := range want {
		f := Factory(s)
		if f == nil {
			t.Fatalf("Factory(%s) = nil", s)
		}
		if _, ok := f.(thinkt.SearchLineFilterer); ok != has {
			t.Errorf("%s implements SearchLineFilterer = %v, want %v", s, ok, has)
		}
	}
	if Factory("nope") != nil {
		t.Error("Factory of an unknown source should be nil")
	}
}
