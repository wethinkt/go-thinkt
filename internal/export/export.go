package export

import (
	"io"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Options controls what gets included in the export.
type Options struct {
	Title              string
	IncludeThinking    bool
	IncludeToolUse     bool
	IncludeToolResults bool
	IncludeMedia       bool
	IncludeSystem      bool
}

// ExportMarkdown writes entries as Markdown to w.
func ExportMarkdown(w io.Writer, entries []thinkt.Entry, opts Options) error {
	return renderMarkdown(w, entries, opts)
}

// ExportHTML writes entries as a self-contained HTML page to w.
func ExportHTML(w io.Writer, entries []thinkt.Entry, opts Options) error {
	return renderHTML(w, entries, opts)
}
