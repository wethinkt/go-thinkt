package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// contentModel manages the content viewport (column 3).
type contentModel struct {
	viewport  viewport.Model
	session   *claude.Session
	width     int
	isPreview bool  // True if showing a preview (limited entries)
	fileSize  int64 // Size of the loaded file
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

func (m *contentModel) setSession(session *claude.Session, isPreview bool, fileSize int64) {
	m.session = session
	m.isPreview = isPreview
	m.fileSize = fileSize
	if session != nil {
		rendered := RenderSession(session, m.width)
		if isPreview && len(session.Entries) > 0 {
			sizeInfo := formatBytes(fileSize)
			rendered += fmt.Sprintf("\n\n--- Preview (%s, showing first %d entries) ---", sizeInfo, len(session.Entries))
		}
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
	return m.viewport.View()
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
