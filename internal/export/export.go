package export

import (
	"encoding/json"
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

// ExportJSON writes entries as a JSON array to w.
func ExportJSON(w io.Writer, entries []thinkt.Entry, opts Options) error {
	filtered := filterEntries(entries, opts)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(filtered)
}

// filterEntries applies Options filters to entries.
func filterEntries(entries []thinkt.Entry, opts Options) []thinkt.Entry {
	var out []thinkt.Entry
	for _, entry := range entries {
		if !shouldIncludeEntry(entry.Role, opts) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
