package conversation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestMessageCarriesPromptID(t *testing.T) {
	entries := []thinkt.Entry{{UUID: "a1", Role: thinkt.RoleAssistant, TurnID: "msg_1", PromptID: "p1", Text: "hi"}}
	doc, err := Build(entries, BuildOptions{SessionID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.Messages[0].PromptID; got != "p1" {
		t.Fatalf("PromptID = %q", got)
	}
	var out bytes.Buffer
	if err := WriteHTML(&out, doc, View{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `data-prompt-id="p1"`) {
		t.Fatalf("html lacks data-prompt-id: %s", out.String())
	}
}
