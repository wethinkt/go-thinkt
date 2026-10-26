package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/nthtml"
	"nimbleterminal.dev/nthtml/document"

	"github.com/wethinkt/go-thinkt/internal/conversation"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

const conversationPageSize = 8

// ConversationViewerModel hosts the shared HTML presentation in the existing
// shell. Only a bounded group of entries is rendered; readers stay incremental.
// nthtml owns text search, selection, links, and terminal graphics.
type ConversationViewerModel struct {
	paths         []string
	registry      *thinkt.StoreRegistry
	session       *conversationSession
	index         int
	start, end    int
	history       []int
	doc           *conversation.Document
	view          *nthtml.Model
	filters       RoleFilterSet
	collapsed     map[string]bool
	width, height int
	busy, dirty   bool
	notice        string
	ctx           context.Context
	cancel        context.CancelFunc
	closing       bool
	standalone    bool
	backRequested bool
	anchor        string
}

type conversationPageMsg struct {
	owner   *ConversationViewerModel
	session *conversationSession
	doc     *conversation.Document
	result  *document.Result
	end     int
	err     error
}

type conversationClosedMsg struct {
	owner *ConversationViewerModel
	back  bool
}

// A lease closes the reader exactly once, including when its result arrives
// after the page has been popped. Cancellation never races LoadMore or layout.
type conversationSession struct {
	thinkt.LazySession
	mu   sync.Mutex
	once sync.Once
	done chan struct{}
}

func ownConversationSession(ctx context.Context, reader thinkt.LazySession) *conversationSession {
	s := &conversationSession{LazySession: reader, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.done:
		}
	}()
	return s
}

func (s *conversationSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	s.once.Do(func() { err = s.LazySession.Close(); close(s.done) })
	return err
}

func NewConversationViewerModel(paths []string, registry *thinkt.StoreRegistry) *ConversationViewerModel {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConversationViewerModel{paths: append([]string(nil), paths...), registry: registry,
		filters: NewRoleFilterSet(), collapsed: make(map[string]bool), ctx: ctx, cancel: cancel}
}

func newSessionViewer(paths []string, registry *thinkt.StoreRegistry, standalone bool) tea.Model {
	if os.Getenv("THINKT_VIEWER") == "html" {
		m := NewConversationViewerModel(paths, registry)
		m.standalone = standalone
		return m
	}
	m := NewMultiViewerModelWithRegistry(paths, registry)
	m.standalone = standalone
	return m
}

func (m *ConversationViewerModel) Init() tea.Cmd { return m.requestPage() }

// requestPage serializes reader access and layout. Resize/filter events arriving
// during a render are coalesced, rather than racing mutable nthtml widgets.
func (m *ConversationViewerModel) requestPage() tea.Cmd {
	if m.closing || len(m.paths) == 0 {
		return nil
	}
	if m.busy {
		m.dirty = true
		return nil
	}
	m.busy = true
	m.notice = ""
	ctx, reader, path, registry := m.ctx, m.session, m.paths[m.index], m.registry
	start, width, filters := m.start, max(320, min(1600, m.width*8)), m.filters
	collapsed := make(map[string]bool, len(m.collapsed))
	for id, value := range m.collapsed {
		collapsed[id] = value
	}
	return func() tea.Msg {
		msg := conversationPageMsg{owner: m}
		if ctx.Err() != nil {
			return msg
		}
		if reader == nil {
			opened, err := OpenLazySessionWithRegistry(path, registry)
			if err != nil {
				msg.err = err
				return msg
			}
			reader = ownConversationSession(ctx, opened)
		}
		reader.mu.Lock()
		defer reader.mu.Unlock()
		if ctx.Err() != nil {
			return msg
		}
		msg.session = reader
		for reader.EntryCount() < start+conversationPageSize && reader.HasMore() {
			if ctx.Err() != nil {
				return msg
			}
			n, err := reader.LoadMore(32 * 1024)
			if err != nil {
				msg.err = err
				return msg
			}
			if n == 0 {
				break
			}
		}
		entries := reader.Entries()
		end := min(len(entries), start+conversationPageSize)
		if start > end {
			msg.err = fmt.Errorf("message range no longer available")
			return msg
		}
		meta := reader.Metadata()
		namespace := meta.ID
		if namespace == "" {
			namespace = path
		}
		for {
			msg.doc, msg.result, msg.err = renderConversationPage(ctx, entries[start:end], namespace, start, width, filters, collapsed)
			if msg.err == nil || end-start <= 1 || !errors.Is(msg.err, document.ErrLimit) {
				break
			}
			// Smaller windows preserve all source content while respecting renderer
			// byte, node, height and pixel budgets. A single oversized entry reports
			// its error instead of being truncated or silently skipped.
			end = start + max(1, (end-start)/2)
		}
		msg.end = end
		return msg
	}
}

func renderConversationPage(ctx context.Context, entries []thinkt.Entry, sessionID string, offset, width int, filters RoleFilterSet, collapsed map[string]bool) (*conversation.Document, *document.Result, error) {
	doc, err := conversation.Build(entries, conversation.BuildOptions{SessionID: sessionID, EntryOffset: offset})
	if err != nil {
		return nil, nil, err
	}
	var html bytes.Buffer
	html.WriteString("<style>")
	html.WriteString(conversation.Stylesheet())
	html.WriteString("</style><body class=\"thinkt-page\">")
	visible := conversationFilters(doc, filters, collapsed)
	if err := conversation.WriteHTML(&html, doc, visible); err != nil {
		return nil, nil, err
	}
	hasVisible := false
	for _, message := range doc.Messages {
		if visible.HiddenRoles[message.Role] {
			continue
		}
		if len(message.Blocks) == 0 {
			hasVisible = true
		}
		for _, block := range message.Blocks {
			if !visible.HiddenBlocks[block.ID] {
				hasVisible = true
			}
		}
	}
	if !hasVisible {
		html.WriteString("<p>No messages match the current filters in this group.</p>")
	}
	html.WriteString("</body>")
	result, err := document.Render(ctx, &html, document.Options{Width: width, Limits: document.Limits{Bytes: 256 * 1024}})
	return doc, result, err
}

func conversationFilters(doc *conversation.Document, filters RoleFilterSet, collapsed map[string]bool) conversation.View {
	view := conversation.View{HiddenRoles: make(map[thinkt.Role]bool), HiddenBlocks: make(map[string]bool), Collapsed: collapsed}
	for _, message := range doc.Messages {
		switch message.Role {
		case thinkt.RoleUser:
			view.HiddenRoles[message.Role] = !filters.User
		case thinkt.RoleAssistant: // Its content types are filtered independently.
		case thinkt.RoleTool:
			view.HiddenRoles[message.Role] = !filters.Tools
		default:
			view.HiddenRoles[message.Role] = !filters.Other
		}
		for _, block := range message.Blocks {
			visible := true
			switch block.Kind {
			case "text":
				if message.Role == thinkt.RoleAssistant {
					visible = filters.Assistant
				}
			case "thinking":
				visible = filters.Thinking
			case "tool_use", "tool_result":
				visible = filters.Tools
			case "image", "document":
				visible = filters.Media
			default:
				visible = filters.Other
			}
			view.HiddenBlocks[block.ID] = !visible
		}
	}
	return view
}

func (m *ConversationViewerModel) rememberAnchor() {
	if m.view != nil {
		if id, ok := m.view.ActiveSourceID(); ok {
			m.anchor = id
		}
	}
}

func (m *ConversationViewerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.closing {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.PasteMsg, tea.WindowSizeMsg,
			tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg, nthtml.CopyMsg, nthtml.LinkMsg:
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case conversationClosedMsg:
		if msg.owner != m {
			return m, nil
		}
		if msg.back && !m.standalone {
			return m, func() tea.Msg { return PopPageMsg{} }
		}
		return m, tea.Quit
	case conversationPageMsg:
		if msg.owner != m || m.closing {
			return m, nil
		}
		m.busy = false
		m.session = msg.session
		if msg.err != nil {
			m.notice = msg.err.Error()
			m.doc = nil
			if m.view != nil {
				cleanup := m.view.Close()
				m.view = nil
				return m, cleanup
			}
		} else {
			m.doc, m.end = msg.doc, msg.end
			var cmd tea.Cmd
			if m.view == nil {
				m.view = nthtml.New(msg.result)
				cmd = tea.Batch(m.view.Init(), m.view.SetLayoutMode(nthtml.LayoutTTY))
			} else {
				cmd = m.view.SetResult(msg.result)
			}
			bounds := m.setBounds()
			anchor := m.visibleAnchor(m.anchor)
			nav, _ := m.view.NavigateSource(anchor)
			if m.dirty {
				m.dirty = false
				return m, tea.Batch(cmd, bounds, nav, m.requestPage())
			}
			return m, tea.Batch(cmd, bounds, nav)
		}
		if m.dirty {
			m.dirty = false
			return m, m.requestPage()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.rememberAnchor()
		changed := m.width != msg.Width
		m.width, m.height = msg.Width, msg.Height
		bounds := m.setBounds()
		if changed && (m.session != nil || m.busy) {
			return m, tea.Batch(bounds, m.requestPage())
		}
		return m, bounds
	case nthtml.CopyMsg:
		if msg.Owner == m.view {
			return m, tea.SetClipboard(msg.Text)
		}
		return m, nil
	case nthtml.LinkMsg:
		if msg.Owner == m.view {
			m.notice = "External link: " + msg.Link.Href
		}
		return m, nil
	case tea.KeyPressMsg:
		if m.closing {
			return m, nil
		}
		if msg.String() == "ctrl+c" {
			return m, m.close(false)
		}
		// Ignore split terminal replies while the first document is loading.
		if m.view == nil && m.busy {
			return m, nil
		}
		if m.view != nil && m.view.SearchInputActive() {
			return m, m.view.Update(msg)
		}
		switch msg.String() {
		case "q":
			return m, m.close(false)
		case "esc":
			if m.view != nil {
				query, _, _ := m.view.SearchStatus()
				if query != "" || m.view.SelectedText() != "" {
					return m, m.view.Update(msg)
				}
			}
			return m, m.close(true)
		case "t":
			if m.view == nil {
				return m, nil
			}
			m.rememberAnchor()
			cmd := m.view.ToggleLayout()
			nav, _ := m.view.NavigateSource(m.anchor)
			return m, tea.Batch(cmd, nav)
		case "1", "2", "3", "4", "5", "6":
			m.rememberAnchor()
			switch msg.String() {
			case "1":
				m.filters.User = !m.filters.User
			case "2":
				m.filters.Assistant = !m.filters.Assistant
			case "3":
				m.filters.Thinking = !m.filters.Thinking
			case "4":
				m.filters.Tools = !m.filters.Tools
			case "5":
				m.filters.Media = !m.filters.Media
			case "6":
				m.filters.Other = !m.filters.Other
			}
			return m, m.requestPage()
		case "]":
			if m.busy || m.session == nil {
				return m, nil
			}
			if m.end < m.session.EntryCount() || m.session.HasMore() {
				m.history = append(m.history, m.start)
				m.start, m.anchor = m.end, ""
				return m, m.requestPage()
			}
		case "[":
			if m.busy || len(m.history) == 0 {
				return m, nil
			}
			m.start, m.anchor = m.history[len(m.history)-1], ""
			m.history = m.history[:len(m.history)-1]
			return m, m.requestPage()
		case "alt+left", "alt+right":
			if m.busy {
				return m, nil
			}
			next := m.index + 1
			if msg.String() == "alt+left" {
				next = m.index - 1
			}
			if next >= 0 && next < len(m.paths) {
				if m.session != nil {
					_ = m.session.Close()
				}
				m.session, m.index, m.start, m.end, m.history, m.anchor = nil, next, 0, 0, nil, ""
				return m, m.requestPage()
			}
		case "J", "K":
			return m, m.moveMessage(msg.String() == "K")
		case "c":
			if m.busy {
				return m, nil
			}
			m.rememberAnchor()
			if m.doc != nil {
				for _, message := range m.doc.Messages {
					for _, block := range message.Blocks {
						if (block.ID == m.anchor || message.ID == m.anchor) && (block.Kind == "thinking" || block.Kind == "tool_use" || block.Kind == "tool_result") {
							m.collapsed[block.ID] = !m.collapsed[block.ID]
							m.anchor = block.ID
							return m, m.requestPage()
						}
					}
				}
			}
		case "w":
			if m.session != nil && !m.busy {
				meta := m.session.Metadata()
				openInWeb(meta.ProjectPath, m.paths[m.index])
			}
			return m, nil
		}
	}
	if m.view != nil {
		return m, m.view.Update(msg)
	}
	return m, nil
}

// visibleAnchor keeps the enclosing message when its active block is filtered,
// otherwise chooses the nearest subsequent visible message, then a preceding one.
func (m *ConversationViewerModel) visibleAnchor(id string) string {
	if m.doc == nil {
		return ""
	}
	view := conversationFilters(m.doc, m.filters, m.collapsed)
	var candidates []string
	target := 0
	for i, message := range m.doc.Messages {
		owns := message.ID == id
		for _, block := range message.Blocks {
			if block.ID == id {
				owns = true
				if !view.HiddenRoles[message.Role] && !view.HiddenBlocks[id] {
					return id
				}
			}
		}
		if owns {
			target = i
		}
		visible := !view.HiddenRoles[message.Role] && len(message.Blocks) == 0
		for _, block := range message.Blocks {
			visible = visible || (!view.HiddenRoles[message.Role] && !view.HiddenBlocks[block.ID])
		}
		if visible {
			candidates = append(candidates, message.ID)
		} else {
			candidates = append(candidates, "")
		}
	}
	for i := target; i < len(candidates); i++ {
		if candidates[i] != "" {
			return candidates[i]
		}
	}
	for i := target - 1; i >= 0; i-- {
		if candidates[i] != "" {
			return candidates[i]
		}
	}
	return ""
}

func (m *ConversationViewerModel) moveMessage(back bool) tea.Cmd {
	if m.view == nil || m.doc == nil {
		return nil
	}
	m.rememberAnchor()
	current := -1
	for i, message := range m.doc.Messages {
		if message.ID == m.anchor {
			current = i
		}
		for _, block := range message.Blocks {
			if block.ID == m.anchor {
				current = i
			}
		}
	}
	delta := 1
	if back {
		delta = -1
		if current < 0 {
			current = len(m.doc.Messages)
		}
	}
	for i := current + delta; i >= 0 && i < len(m.doc.Messages); i += delta {
		id := m.doc.Messages[i].ID
		if m.visibleAnchor(id) == id {
			m.anchor = id
			cmd, _ := m.view.NavigateSource(id)
			return cmd
		}
	}
	return nil
}

func (m *ConversationViewerModel) setBounds() tea.Cmd {
	if m.view == nil {
		return nil
	}
	header := 0
	if m.standalone {
		header = 1
	}
	return m.view.SetBounds(0, 1, max(1, m.width), max(1, m.height-header-2))
}

// shutdown cancels in-flight work; the lease releases the reader after its last
// operation even if a completion message is delivered to a different page.
func (m *ConversationViewerModel) shutdown() {
	m.cancel()
	if !m.busy && m.session != nil {
		_ = m.session.Close()
		m.session = nil
	}
}

func (m *ConversationViewerModel) close(back bool) tea.Cmd {
	m.closing, m.backRequested = true, back
	m.shutdown()
	var cleanup tea.Cmd
	if m.view != nil {
		cleanup = m.view.Close()
	}
	return tea.Sequence(cleanup, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return conversationClosedMsg{m, back} }))
}

func (m *ConversationViewerModel) BackRequested() bool { return m.backRequested }

func (m *ConversationViewerModel) FilterStatus() string {
	return fmt.Sprintf("1 user:%t 2 assistant:%t 3 thinking:%t 4 tools:%t 5 media:%t 6 other:%t", m.filters.User, m.filters.Assistant, m.filters.Thinking, m.filters.Tools, m.filters.Media, m.filters.Other)
}

func viewerLabel(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
}

func (m *ConversationViewerModel) ViewContent() string {
	content := "Loading conversation…"
	if len(m.paths) == 0 {
		content = "No sessions selected."
	}
	if m.view != nil {
		content = m.view.View()
	} else if m.notice != "" {
		content = viewerLabel(m.notice)
	}
	var header string
	if m.standalone {
		header = "thinkt · conversation\n"
	}
	status := fmt.Sprintf("Session %d/%d · messages %d–%d", m.index+1, len(m.paths), m.start+1, m.end)
	if m.busy {
		status = fmt.Sprintf("Session %d/%d · loading messages from %d…", m.index+1, len(m.paths), m.start+1)
	}
	if m.notice != "" {
		status += " · " + viewerLabel(m.notice)
	}
	if m.view != nil {
		if m.view.SearchInputActive() {
			status = "/" + m.view.SearchInput()
		} else if query, current, total := m.view.SearchStatus(); query != "" {
			status += fmt.Sprintf(" · %q %d/%d (this group)", query, current, total)
		}
	}
	help := "[/] messages · J/K jump · t styled/TTY · c collapse · 1–6 filters · / search · Esc back · q quit"
	return header + content + "\n" + ansi.Truncate(status, max(1, m.width), "…") + "\n" + ansi.Truncate(help, max(1, m.width), "…")
}

func (m *ConversationViewerModel) configureMouseView(view *tea.View) {
	if !m.closing {
		view.MouseMode = tea.MouseModeCellMotion
	}
}

func (m *ConversationViewerModel) View() tea.View {
	view := tea.NewView(m.ViewContent())
	view.AltScreen = true
	m.configureMouseView(&view)
	return view
}
