package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// contentModel manages the content viewport (column 3).
type contentModel struct {
	viewport viewport.Model
	session  *claude.Session
	loading  bool
	width    int
}

func newContentModel() contentModel {
	vp := viewport.New()
	return contentModel{viewport: vp}
}

func (m *contentModel) setSize(w, h int) {
	m.width = w
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h)
}

func (m *contentModel) setSession(session *claude.Session) {
	m.session = session
	m.loading = false
	if session != nil {
		rendered := RenderSession(session, m.width)
		m.viewport.SetContent(rendered)
		m.viewport.GotoTop()
	} else {
		m.viewport.SetContent("Select a session to view")
	}
}

func (m contentModel) update(msg tea.Msg) (contentModel, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m contentModel) view() string {
	if m.loading {
		return "Loading..."
	}
	return m.viewport.View()
}
