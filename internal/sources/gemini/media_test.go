package gemini

import (
	"testing"
)

func TestUserPartsCarryMedia(t *testing.T) {
	r := replayLines(t, hdr,
		`{"id":"u","timestamp":"2026-05-17T11:00:00Z","type":"user","content":[{"text":"look"},{"inlineData":{"mimeType":"image/png","data":"eHg="}},{"fileData":{"mimeType":"application/pdf","fileUri":"file:///b/y.pdf"}}]}`)
	entries := convert(r, "s")
	if len(entries) != 1 {
		t.Fatalf("%+v", entries)
	}
	bs := entries[0].ContentBlocks
	if len(bs) != 3 || bs[0].Text != "look" || bs[1].Media.Size != 2 || bs[2].Media.URI != "/b/y.pdf" || bs[2].Type != "document" {
		t.Fatalf("%+v", bs)
	}
	if entries[0].Text != "look" {
		t.Fatalf("%q", entries[0].Text)
	}
}
