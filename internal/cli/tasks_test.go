package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestFormatTasks(t *testing.T) {
	var b bytes.Buffer
	FormatTasks(&b, []thinkt.BackgroundTask{{ID: "b1", Kind: "process", Status: "failed", Description: "build", PromptID: "p1"}})
	for _, want := range []string{"b1", "process", "failed", "build", "p1"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in %q", want, b.String())
		}
	}
	b.Reset()
	FormatTasks(&b, nil)
	if !strings.Contains(b.String(), "No background tasks") {
		t.Fatalf("%q", b.String())
	}
}
