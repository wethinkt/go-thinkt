package export

import (
	"encoding/json"
	"io"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// Options controls export rendering.
type Options struct {
	Title string
}

// ExportMarkdown writes entries as Markdown to w.
func ExportMarkdown(w io.Writer, entries []thinkt.Entry, opts Options) error {
	return renderMarkdown(w, entries, opts)
}

// ExportHTML writes entries as a self-contained HTML page to w.
func ExportHTML(w io.Writer, entries []thinkt.Entry, opts Options) error {
	return renderHTML(w, entries, opts)
}

// ExportJSON writes entries as a JSON array to w.
func ExportJSON(w io.Writer, entries []thinkt.Entry, opts Options) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}
