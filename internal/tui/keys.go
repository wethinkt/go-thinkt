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

	// Filter toggles
	ToggleInput    key.Binding
	ToggleOutput   key.Binding
	ToggleTools    key.Binding
	ToggleThinking key.Binding
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

		// Filter toggles
		ToggleInput: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "toggle input"),
		),
		ToggleOutput: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "toggle output"),
		),
		ToggleTools: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "toggle tools"),
		),
		ToggleThinking: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "toggle thinking"),
		),
		ToggleOther: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "toggle other"),
		),
	}
}
