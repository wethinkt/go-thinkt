package tui

import "charm.land/bubbles/v2/key"

// listKeyMap defines key bindings for list views
type listKeyMap struct {
	up         key.Binding
	down       key.Binding
	selectItem key.Binding
	back       key.Binding
	quit       key.Binding
}

// viewerKeyMap defines key bindings for the session viewer
type viewerKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	PgUp   key.Binding
	PgDown key.Binding
	Home   key.Binding
	End    key.Binding
	Quit   key.Binding
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
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
