package tui

import "charm.land/bubbles/v2/key"

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
			key.WithHelp("↑/k", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
		PgUp: key.NewBinding(
			key.WithKeys("pgup", "b"),
			key.WithHelp("pgup", "page up"),
		),
		PgDown: key.NewBinding(
			key.WithKeys("pgdown", " "),
			key.WithHelp("pgdn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g", "go to top"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G", "go to bottom"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),

		// Search
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),

		// Actions
		OpenWeb: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "open web"),
		),

		// Filter toggles
		ToggleInput: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "toggle user"),
		),
		ToggleOutput: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "toggle assistant"),
		),
		ToggleThinking: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "toggle thinking"),
		),
		ToggleTools: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "toggle tools"),
		),
		ToggleMedia: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "toggle media"),
		),
		ToggleOther: key.NewBinding(
			key.WithKeys("6"),
			key.WithHelp("6", "toggle other"),
		),
	}
}
