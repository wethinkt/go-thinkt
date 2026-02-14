package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type sizeProbeModel struct {
	lastWidth  int
	lastHeight int
	seenSize   bool
}

func (m *sizeProbeModel) Init() tea.Cmd { return nil }

func (m *sizeProbeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.lastWidth = ws.Width
		m.lastHeight = ws.Height
		m.seenSize = true
	}
	return m, nil
}

func (m *sizeProbeModel) View() tea.View {
	return tea.NewView("")
}

func runAllCmdMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, runAllCmdMessages(sub)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestShellPopPageRebroadcastsFullWindowSize(t *testing.T) {
	revealed := &sizeProbeModel{}
	top := &sizeProbeModel{}

	s := &Shell{
		width:   120,
		height:  40,
		stack:   NewNavStack(),
		loading: false,
	}
	s.stack.items = append(s.stack.items,
		NavItem{Title: "revealed", Model: revealed},
		NavItem{Title: "top", Model: top},
	)

	model, cmd := s.Update(PopPageMsg{})
	shell, ok := model.(*Shell)
	if !ok {
		t.Fatalf("expected *Shell model, got %T", model)
	}
	if len(shell.stack.items) != 1 {
		t.Fatalf("expected one page after pop, got %d", len(shell.stack.items))
	}

	msgs := runAllCmdMessages(cmd)
	if len(msgs) == 0 {
		t.Fatal("expected rebroadcast command message, got none")
	}

	var rawSize *tea.WindowSizeMsg
	for _, msg := range msgs {
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			copy := ws
			rawSize = &copy
		}
	}
	if rawSize == nil {
		t.Fatalf("expected a WindowSizeMsg in command batch, got %#v", msgs)
	}
	if rawSize.Width != 120 || rawSize.Height != 40 {
		t.Fatalf("expected full window size 120x40 from rebroadcast, got %dx%d", rawSize.Width, rawSize.Height)
	}

	for _, msg := range msgs {
		model, _ = shell.Update(msg)
		shell = model.(*Shell)
	}

	if !revealed.seenSize {
		t.Fatal("revealed page did not receive size update")
	}
	// Shell has a one-line header, so child page should receive height-1.
	if revealed.lastWidth != 120 || revealed.lastHeight != 39 {
		t.Fatalf("expected revealed child size 120x39, got %dx%d", revealed.lastWidth, revealed.lastHeight)
	}
}

