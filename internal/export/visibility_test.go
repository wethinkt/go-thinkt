package export

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/conversation"
	"github.com/wethinkt/go-thinkt/internal/target"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestHTMLExportSystemRevealsSecondary(t *testing.T) {
	entries := []thinkt.Entry{
		{UUID: "u1", Role: thinkt.RoleUser, Text: "hello"},
		{UUID: "s1", Role: thinkt.RoleSystem, Kind: thinkt.KindTurnDuration, Visibility: thinkt.VisibilitySecondary, Text: "turn_duration"},
	}
	doc, err := conversation.Build(entries, conversation.BuildOptions{SessionID: "sess"})
	if err != nil {
		t.Fatal(err)
	}
	secondaryID := doc.Messages[1].ID

	withSystem := target.DefaultFilter()
	withSystem.IncludeSystem = true
	var out bytes.Buffer
	if err := ExportHTML(&out, entries, Options{SessionID: "sess", HTMLFilter: &withSystem}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), secondaryID) {
		t.Fatal("--system HTML export must include secondary entries")
	}

	without := target.DefaultFilter()
	out.Reset()
	if err := ExportHTML(&out, entries, Options{SessionID: "sess", HTMLFilter: &without}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), secondaryID) {
		t.Fatal("default HTML export must hide secondary entries")
	}
}
