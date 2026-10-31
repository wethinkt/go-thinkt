package codex

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func images(blocks []thinkt.ContentBlock) []thinkt.ContentBlock {
	var out []thinkt.ContentBlock
	for _, b := range blocks {
		if b.Media != nil {
			out = append(out, b)
		}
		out = append(out, images(b.Children)...)
	}
	return out
}

func TestWireOutputImagesAreBlocks(t *testing.T) {
	entries := parseAll(t,
		`{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"s","cwd":"/p"}}`,
		`{"timestamp":"2026-10-01T00:00:01Z","type":"response_item","payload":{"type":"function_call","name":"view","call_id":"c","arguments":"{}"}}`,
		`{"timestamp":"2026-10-01T00:00:02Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c","output":[{"type":"input_text","text":"seen"},{"type":"input_image","image_url":"data:image/png;base64,eHg=","detail":"auto"}]}}`)
	var got thinkt.ContentBlock
	for _, e := range entries {
		for _, b := range e.ContentBlocks {
			if b.Type == "tool_result" {
				got = b
			}
		}
	}
	if got.ToolResult != "seen" || len(got.Children) != 1 || got.Children[0].Media == nil || got.Children[0].MediaType != "image/png" {
		t.Fatalf("%+v", got)
	}
}

func TestMessageImagesCarryRefs(t *testing.T) {
	blocks := imageBlocks([]contentPart{{Type: "input_image", ImageURL: "data:image/png;base64,eHg="}, {Type: "input_image", ImageURL: "https://e.com/x.png"}}, "hi")
	if len(blocks) != 3 || blocks[0].Type != "text" || blocks[1].Media.Size != 2 || blocks[2].Media.URI != "https://e.com/x.png" {
		t.Fatalf("%+v", blocks)
	}
}

func TestMcpImageAndImageViewAreBlocks(t *testing.T) {
	entries := parseAll(t,
		`{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"s","cwd":"/p"}}`,
		`{"timestamp":"2026-10-01T00:00:01Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"McpToolCall","id":"m","server":"s","tool":"t","status":"completed","result":{"content":[{"type":"image","data":"eHg=","mimeType":"image/png"}]}}}}`,
		`{"timestamp":"2026-10-01T00:00:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"ImageView","id":"v","path":"/b/x.png"}}}`)
	var found []thinkt.ContentBlock
	for _, e := range entries {
		found = append(found, images(e.ContentBlocks)...)
	}
	if len(found) != 2 || found[0].Media.Size != 2 || found[1].Media.URI != "/b/x.png" {
		t.Fatalf("%+v", found)
	}
}
