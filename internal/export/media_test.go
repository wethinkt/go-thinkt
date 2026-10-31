package export

import (
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestMediaLineNamesLocation(t *testing.T) {
	var b strings.Builder
	writeMediaLine(&b, thinkt.MediaBlockURI("image/png", "/b/x.png"))
	if !strings.Contains(b.String(), "Image: image/png (/b/x.png)") {
		t.Fatalf("%q", b.String())
	}
	b.Reset()
	writeMediaLine(&b, thinkt.MediaBlock("image/png", "eHg="))
	if strings.Contains(b.String(), "(") {
		t.Fatalf("%q", b.String())
	}
}
