package kimi

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

var blobHash = strings.Repeat("ab", 32)

func TestToolResultImagesCarryRefs(t *testing.T) {
	b := toolResultBlock("t", []any{
		map[string]any{"type": "image_url", "imageUrl": map[string]any{"url": "data:image/png;base64,eHg="}},
		map[string]any{"type": "image_url", "imageUrl": map[string]any{"url": "blobref:image/png;" + blobHash}},
	}, "", false)
	if len(b.Children) != 2 || b.Children[0].Media == nil || b.Children[1].Type != "image" || b.Children[1].Media.SHA256 != blobHash {
		t.Fatalf("%+v", b.Children)
	}
}

func TestMessageImagesAreBlocks(t *testing.T) {
	blocks := contentBlocks([]json.RawMessage{[]byte(`{"type":"text","text":"look"}`), []byte(`{"type":"image_url","imageUrl":{"url":"data:image/png;base64,eHg="}}`)})
	if len(blocks) != 2 || blocks[1].Media == nil {
		t.Fatalf("%+v", blocks)
	}
}

func TestReaderResolvesBlobs(t *testing.T) {
	base := storeFixture(t)
	main := filepath.Join(base, "sessions", "wd_proj_abc", "session_p1", "agents", "main")
	writeFile(t, filepath.Join(main, "blobs", blobHash), "bytes")
	f, err := os.OpenFile(filepath.Join(main, "wire.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"message":{"message":{"role":"user","content":[{"type":"image_url","imageUrl":{"url":"blobref:image/png;` + blobHash + `"}}],"toolCalls":[]},"meta":{"source":"user","messageId":"m9"}},"type":"agent.message.appended","time":1,"kind":"event"}` + "\n")
	_ = f.Close()
	r, err := newTestStore(t, base).OpenSession(context.Background(), "session_p1")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close() //nolint:errcheck
	var found *thinkt.MediaRef
	for {
		e, err := r.ReadNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range e.ContentBlocks {
			if b.Media != nil {
				found = b.Media
			}
		}
	}
	if found == nil || found.URI != filepath.Join(main, "blobs", blobHash) || found.Size != 5 {
		t.Fatalf("%+v", found)
	}
}
