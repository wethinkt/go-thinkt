package antigravity

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func media(blocks []thinkt.ContentBlock) []thinkt.ContentBlock {
	var out []thinkt.ContentBlock
	for _, b := range blocks {
		if b.Media != nil {
			out = append(out, b)
		}
		out = append(out, media(b.Children)...)
	}
	return out
}

func TestMediaBlocks(t *testing.T) {
	got := mediaBlocks([]mediaRef{{MimeType: "image/png", URI: "file:///b/x.png"}, {MimeType: "application/pdf", URI: "/b/y.pdf"}, {MimeType: "image/png"}})
	if len(got) != 2 || got[0].Media.URI != "/b/x.png" || got[1].Type != "document" {
		t.Fatalf("%+v", got)
	}
}

func TestUserInputKeepsTextAndGainsMedia(t *testing.T) {
	c := converter{}
	e := c.newEntry(thinkt.RoleUser, stepHead{})
	e.Text = "look"
	attachMedia(&e, []mediaRef{{MimeType: "image/png", URI: "/b/x.png"}})
	if len(e.ContentBlocks) != 2 || e.ContentBlocks[0].Text != "look" || e.ContentBlocks[1].Media == nil || e.Text != "look" {
		t.Fatalf("%+v", e.ContentBlocks)
	}
}

func TestResultEntryCarriesMedia(t *testing.T) {
	c := converter{}
	e := c.resultEntry(stepHead{Content: "shot", Media: []mediaRef{{MimeType: "image/png", URI: "/b/x.png"}}}, "t1")
	if got := media(e.ContentBlocks); len(got) != 1 || got[0].Media.URI != "/b/x.png" {
		t.Fatalf("%+v", e.ContentBlocks)
	}
}
