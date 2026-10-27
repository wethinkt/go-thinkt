package export

import (
	"io"

	"github.com/wethinkt/go-thinkt/internal/conversation"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func renderHTML(w io.Writer, entries []thinkt.Entry, opts Options) error {
	doc, err := conversation.Build(entries, conversation.BuildOptions{Title: opts.Title, SessionID: opts.SessionID})
	if err != nil {
		return err
	}
	view := conversation.View{}
	if opts.HTMLFilter != nil {
		// WritePage hides secondary entries by default; --system reveals them,
		// matching the Markdown and JSON exports.
		view.HiddenVisibility = map[thinkt.Visibility]bool{thinkt.VisibilitySecondary: !opts.HTMLFilter.IncludeSystem}
		view.HiddenRoles = make(map[thinkt.Role]bool)
		view.HiddenKinds = make(map[string]bool)
		for _, message := range doc.Messages {
			view.HiddenRoles[message.Role] = !opts.HTMLFilter.IncludesRole(message.Role)
			for _, block := range message.Blocks {
				view.HiddenKinds[block.Kind] = !opts.HTMLFilter.IncludesBlockType(block.Kind)
			}
		}
	}
	return conversation.WritePage(w, doc, view)
}
