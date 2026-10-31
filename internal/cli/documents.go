package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// FormatDocuments prints one line per document, with its summary below.
func FormatDocuments(w io.Writer, docs []thinkt.AgentDocument) {
	if len(docs) == 0 {
		fmt.Fprintln(w, "No documents.")
		return
	}
	for _, d := range docs {
		fmt.Fprintf(w, "%-32s %-12s %d versions\n", d.Name, d.Kind, len(d.Versions))
		if d.Summary != "" {
			fmt.Fprintf(w, "  %s\n", d.Summary)
		}
	}
}

// DocumentText returns a document's final text, or version when it is not 0.
// name matches with or without the .md extension.
func DocumentText(docs []thinkt.AgentDocument, name string, version int) (string, error) {
	for _, d := range docs {
		if d.Name != name && strings.TrimSuffix(d.Name, ".md") != name {
			continue
		}
		if version == 0 {
			return d.Final, nil
		}
		for _, v := range d.Versions {
			if v.Number == version {
				return v.Content, nil
			}
		}
		return "", fmt.Errorf("document %s has no version %d", name, version)
	}
	return "", fmt.Errorf("no document %q", name)
}
