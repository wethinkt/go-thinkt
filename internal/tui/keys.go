package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit       key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	Enter      key.Binding
	OpenTracer key.Binding
}

var keys = keyMap{
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next column")),
	ShiftTab:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev column")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	OpenTracer: key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "open thinking-tracer")),
}
