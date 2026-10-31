package conversation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/yuin/goldmark"
)

func TestMediaLocationReachesHTML(t *testing.T) {
	block, err := buildBlock(thinkt.MediaBlockURI("image/png", "/b/<x>.png"), "b1", Origin{})
	if err != nil {
		t.Fatal(err)
	}
	if block.MediaURI != "/b/<x>.png" {
		t.Fatalf("%+v", block)
	}
	var out bytes.Buffer
	if err := writeBlock(&out, goldmark.New(), block, View{}, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/b/&lt;x&gt;.png") {
		t.Fatalf("%s", out.String())
	}
}
