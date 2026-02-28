package tui

import (
	"charm.land/bubbles/v2/key"

	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
)

// viewerKeyMap defines key bindings for the session viewer
type viewerKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	PgUp   key.Binding
	PgDown key.Binding
	Home   key.Binding
	End    key.Binding
	Quit   key.Binding
	Back   key.Binding

	// Search
	Search    key.Binding
	NextMatch key.Binding
	PrevMatch key.Binding

	// Actions
	OpenWeb key.Binding

	// Filter toggles
	ToggleInput    key.Binding
	ToggleOutput   key.Binding
	ToggleThinking key.Binding
	ToggleTools    key.Binding
	ToggleMedia    key.Binding
	ToggleOther    key.Binding
}

// defaultViewerKeyMap returns the default key bindings for the viewer
func defaultViewerKeyMap() viewerKeyMap {
	return viewerKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", thinktI18n.T("tui.help.scrollUp", "scroll up")),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", thinktI18n.T("tui.help.scrollDown", "scroll down")),
		),
		PgUp: key.NewBinding(
			key.WithKeys("pgup", "b"),
			key.WithHelp("pgup", thinktI18n.T("tui.help.pageUp", "page up")),
		),
		PgDown: key.NewBinding(
			key.WithKeys("pgdown", " "),
			key.WithHelp("pgdn", thinktI18n.T("tui.help.pageDown", "page down")),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g", thinktI18n.T("tui.help.goToTop", "go to top")),
		),
		End: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G", thinktI18n.T("tui.help.goToBottom", "go to bottom")),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", thinktI18n.T("tui.help.quit", "quit")),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", thinktI18n.T("tui.help.back", "back")),
		),

		// Search
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", thinktI18n.T("tui.help.search", "search")),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", thinktI18n.T("tui.help.nextMatch", "next match")),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", thinktI18n.T("tui.help.prevMatch", "prev match")),
		),

		// Actions
		OpenWeb: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", thinktI18n.T("tui.help.openWeb", "open web")),
		),

		// Filter toggles
		ToggleInput: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", thinktI18n.T("tui.help.toggleUser", "toggle user")),
		),
		ToggleOutput: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", thinktI18n.T("tui.help.toggleAssistant", "toggle assistant")),
		),
		ToggleThinking: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", thinktI18n.T("tui.help.toggleThinking", "toggle thinking")),
		),
		ToggleTools: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", thinktI18n.T("tui.help.toggleTools", "toggle tools")),
		),
		ToggleMedia: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", thinktI18n.T("tui.help.toggleMedia", "toggle media")),
		),
		ToggleOther: key.NewBinding(
			key.WithKeys("6"),
			key.WithHelp("6", thinktI18n.T("tui.help.toggleOther", "toggle other")),
		),
	}
}
