package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/nthtml"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func conversationFixture(t *testing.T) []thinkt.Entry {
	t.Helper()
	data, err := os.ReadFile("../conversation/testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []thinkt.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func viewerKey(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: []rune(text)[0], Text: text})
}

func TestConversationPagePreservesAnchorsAndRoleSpecificFilters(t *testing.T) {
	entries := conversationFixture(t)
	filters := NewRoleFilterSet()
	doc, result, err := renderConversationPage(context.Background(), entries, "session", 8, 800, filters, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := result.Document().TTY(context.Background(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Permission denied") {
		t.Fatal("tool-role result hidden by the unrelated other-role filter")
	}
	filters.Assistant = false
	filtered, result, err := renderConversationPage(context.Background(), entries, "session", 8, 480, filters, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err = result.Document().TTY(context.Background(), 60)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Can you make") || strings.Contains(text, "We can keep content") {
		t.Fatal("assistant text filter also hid user content or failed to hide assistant text")
	}
	for i, message := range doc.Messages {
		if message.ID != filtered.Messages[i].ID {
			t.Fatal("filter or width changed source identity")
		}
		if _, ok := result.AnchorBounds(message.ID); !ok {
			t.Fatalf("source anchor not retained: %s", message.ID)
		}
	}
	filters.Tools = false
	_, result, err = renderConversationPage(context.Background(), entries, "session", 8, 480, filters, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err = result.Document().TTY(context.Background(), 60)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "Permission denied") || strings.Contains(text, "Tool: Read") {
		t.Fatal("tools filter failed to hide a call or tool-role result")
	}
}

func TestConversationPageCollapseAndEmptyFilter(t *testing.T) {
	entries := conversationFixture(t)
	doc, _, err := renderConversationPage(context.Background(), entries, "session", 0, 640, NewRoleFilterSet(), nil)
	if err != nil {
		t.Fatal(err)
	}
	id := doc.Messages[1].Blocks[0].ID
	_, result, err := renderConversationPage(context.Background(), entries, "session", 0, 640, NewRoleFilterSet(), map[string]bool{id: true})
	if err != nil {
		t.Fatal(err)
	}
	text, err := result.Document().TTY(context.Background(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "The conversation already has structure") || !strings.Contains(text, "Thinking (collapsed)") {
		t.Fatal("collapsed state did not survive HTML-to-terminal conversion")
	}
	_, result, err = renderConversationPage(context.Background(), entries, "session", 0, 640, RoleFilterSet{}, nil)
	if err != nil {
		t.Fatalf("all filters off should be a valid empty page: %v", err)
	}
	text, err = result.Document().TTY(context.Background(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "No messages match") {
		t.Fatal("empty filters gave no feedback")
	}
}

func TestConversationViewerCollapseActiveMessage(t *testing.T) {
	entries := conversationFixture(t)
	m := loadedConversationViewer(t, &conversationTestSession{entries: entries, loaded: len(entries)})
	thinking := m.doc.Messages[1].Blocks[0].ID
	m.view.NavigateSource(m.doc.Messages[1].ID)
	_, cmd := m.Update(viewerKey("c"))
	if cmd == nil || !m.collapsed[thinking] {
		t.Fatal("collapse did not target the active message's thinking section")
	}
	msg := cmd().(conversationPageMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	if _, ok := m.view.Result().AnchorBounds(thinking); !ok {
		t.Fatal("collapsed block lost its header anchor")
	}
}

// This reader deliberately loads a small batch per LoadMore and records LoadAll
// so tests exercise the actual lazy/page boundary rather than a full session load.
type conversationTestSession struct {
	entries          []thinkt.Entry
	loaded           int
	loads, all       int
	closed           atomic.Int32
	started, release chan struct{}
}

func (s *conversationTestSession) Metadata() thinkt.SessionMeta {
	return thinkt.SessionMeta{ID: "session"}
}
func (s *conversationTestSession) Entries() []thinkt.Entry { return s.entries[:s.loaded] }
func (s *conversationTestSession) EntryCount() int         { return s.loaded }
func (s *conversationTestSession) HasMore() bool           { return s.loaded < len(s.entries) }
func (s *conversationTestSession) LoadMore(int) (int, error) {
	if s.started != nil {
		close(s.started)
		<-s.release
		s.started = nil
	}
	s.loads++
	previous := s.loaded
	s.loaded = min(len(s.entries), s.loaded+conversationPageSize)
	return s.loaded - previous, nil
}
func (s *conversationTestSession) LoadAll() error    { s.all++; s.loaded = len(s.entries); return nil }
func (s *conversationTestSession) Progress() float64 { return 0 }
func (s *conversationTestSession) Close() error      { s.closed.Add(1); return nil }

func loadedConversationViewer(t *testing.T, reader *conversationTestSession) *ConversationViewerModel {
	t.Helper()
	m := NewConversationViewerModel([]string{"fixture"}, nil)
	m.width, m.height = 80, 24
	m.session = ownConversationSession(m.ctx, reader)
	t.Cleanup(m.shutdown)
	msg := m.Init()().(conversationPageMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	return m
}

func TestConversationViewerLoadsAndNavigatesBoundedGroups(t *testing.T) {
	reader := &conversationTestSession{loaded: conversationPageSize}
	for i := 0; i < 25; i++ {
		reader.entries = append(reader.entries, thinkt.Entry{UUID: fmt.Sprint(i), Role: thinkt.RoleUser, Text: fmt.Sprintf("message %d", i)})
	}
	m := loadedConversationViewer(t, reader)
	if reader.loads != 0 || reader.all != 0 || m.end != conversationPageSize {
		t.Fatal("initial render read beyond its group")
	}
	first := m.doc.Messages[0].ID
	_, cmd := m.Update(viewerKey("]"))
	msg := cmd().(conversationPageMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	if m.start != 8 || m.end != 16 || reader.loads != 1 || reader.all != 0 {
		t.Fatal("next group did not load incrementally")
	}
	if m.doc.Messages[0].Origin.EntryIndex != 8 {
		t.Fatal("page lost original source offset")
	}
	_, cmd = m.Update(viewerKey("["))
	msg = cmd().(conversationPageMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	if m.doc.Messages[0].ID != first || reader.loads != 1 || m.start != 0 {
		t.Fatal("returning to a group changed anchors or reread source data")
	}
}

func TestConversationViewerSourceNavigationAndFilterRestore(t *testing.T) {
	entries := conversationFixture(t)
	m := loadedConversationViewer(t, &conversationTestSession{entries: entries, loaded: len(entries)})
	thinking := m.doc.Messages[1].Blocks[0].ID
	message := m.doc.Messages[1].ID
	m.view.Search("The conversation already has structure")
	if id, ok := m.view.SelectedSourceID(); !ok || id != thinking {
		t.Fatalf("search did not map to thinking source: %q", id)
	}
	_, cmd := m.Update(viewerKey("3"))
	if m.anchor != thinking {
		t.Fatalf("filter did not remember active source: %q", m.anchor)
	}
	msg := cmd().(conversationPageMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	if m.visibleAnchor(thinking) != message {
		t.Fatal("hidden block did not resolve to its enclosing message")
	}
	if _, ok := m.view.Result().AnchorBounds(thinking); ok {
		t.Fatal("filtered thinking retained a visible anchor")
	}
	if _, ok := m.view.Result().AnchorBounds(message); !ok {
		t.Fatal("enclosing message anchor disappeared")
	}
	_, result, err := renderConversationPage(m.ctx, entries, "session", 0, 480, m.filters, m.collapsed)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.AnchorBounds(message); !ok {
		t.Fatal("reflow changed the enclosing message's source anchor")
	}
}

func TestConversationViewerClosesReaderWhenResultIsDropped(t *testing.T) {
	reader := &conversationTestSession{entries: []thinkt.Entry{{Role: thinkt.RoleUser, Text: "hello"}}, loaded: 1}
	m := NewConversationViewerModel([]string{"fixture"}, nil)
	m.width = 80
	m.session = ownConversationSession(m.ctx, reader)
	_ = m.Init()() // Simulate a completed command whose result has not been delivered.
	m.shutdown()
	select {
	case <-m.session.done:
	case <-time.After(time.Second):
		t.Fatal("reader leaked after cancelled result delivery")
	}
	_ = m.session.Close()
	if reader.closed.Load() != 1 {
		t.Fatal("reader closed more than once")
	}
}

func TestConversationViewerCancellationWaitsForReader(t *testing.T) {
	reader := &conversationTestSession{entries: []thinkt.Entry{{Role: thinkt.RoleUser, Text: "hello"}}, started: make(chan struct{}), release: make(chan struct{})}
	m := NewConversationViewerModel([]string{"fixture"}, nil)
	m.width = 80
	m.session = ownConversationSession(m.ctx, reader)
	cmd := m.Init()
	finished := make(chan struct{})
	started := reader.started
	go func() { _ = cmd(); close(finished) }()
	<-started
	m.shutdown()
	if reader.closed.Load() != 0 {
		t.Fatal("closed a reader during LoadMore")
	}
	close(reader.release)
	<-finished
	select {
	case <-m.session.done:
	case <-time.After(time.Second):
		t.Fatal("cancelled reader was not released")
	}
	if reader.closed.Load() != 1 {
		t.Fatal("reader not closed exactly once")
	}
}

func TestConversationViewerBackAndQuitLifecycle(t *testing.T) {
	for _, standalone := range []bool{false, true} {
		reader := &conversationTestSession{entries: []thinkt.Entry{{Role: thinkt.RoleUser, Text: "hello"}}, loaded: 1}
		m := loadedConversationViewer(t, reader)
		m.standalone = standalone
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		if cmd == nil || !m.backRequested || !m.closing {
			t.Fatal("Escape did not begin cleanup/back navigation")
		}
		if reader.closed.Load() != 1 {
			t.Fatal("back navigation left the source reader open")
		}
		_, cmd = m.Update(conversationClosedMsg{m, true})
		msg := cmd()
		if standalone {
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Fatalf("standalone back = %T", msg)
			}
		} else {
			if _, ok := msg.(PopPageMsg); !ok {
				t.Fatalf("embedded back = %T", msg)
			}
		}
	}
}

func TestConversationViewerFactoryAndMessageOwnership(t *testing.T) {
	t.Setenv("THINKT_VIEWER", "html")
	model := newSessionViewer(nil, nil, false)
	m, ok := model.(*ConversationViewerModel)
	if !ok {
		t.Fatalf("HTML factory returned %T", model)
	}
	defer m.shutdown()
	if _, cmd := m.Update(nthtml.CopyMsg{Owner: &nthtml.Model{}, Text: "stale"}); cmd != nil {
		t.Fatal("accepted another widget's clipboard request")
	}
	t.Setenv("THINKT_VIEWER", "")
	if _, ok := newSessionViewer(nil, nil, false).(MultiViewerModel); !ok {
		t.Fatal("default viewer changed")
	}
}

func conversationPageFromCmd(t *testing.T, cmd tea.Cmd) conversationPageMsg {
	t.Helper()
	for _, msg := range runAllCmdMessages(cmd) {
		if page, ok := msg.(conversationPageMsg); ok {
			return page
		}
	}
	t.Fatal("no conversation page command")
	return conversationPageMsg{}
}

func TestConversationViewerCoalescesResizeAndRestoresAnchor(t *testing.T) {
	entries := conversationFixture(t)
	m := loadedConversationViewer(t, &conversationTestSession{entries: entries, loaded: len(entries)})
	m.view.Search("Permission denied")
	target, ok := m.view.SelectedSourceID()
	if !ok {
		t.Fatal("no source for search match")
	}
	_, first := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	if !m.busy {
		t.Fatal("resize did not schedule layout")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m.dirty {
		t.Fatal("second resize was not coalesced")
	}
	_, second := m.Update(conversationPageFromCmd(t, first))
	if !m.busy || m.dirty {
		t.Fatal("latest resize was not scheduled once")
	}
	m.Update(conversationPageFromCmd(t, second))
	if m.busy || m.width != 100 || m.anchor != target {
		t.Fatal("reflow lost final size or source anchor")
	}
	if m.view.Bounds().Min.Y != 1 || m.view.Bounds().Dx() != 100 {
		t.Fatal("widget bounds do not account for shell header")
	}
	if _, ok := m.view.Result().AnchorBounds(target); !ok {
		t.Fatal("restored source absent from final layout")
	}
}

func TestShellSelectsEmbeddedConversationViewer(t *testing.T) {
	t.Setenv("THINKT_VIEWER", "html")
	s := &Shell{stack: NewNavStack(), width: 100, height: 30}
	s.stack.items = append(s.stack.items, NavItem{Title: "sessions", Model: NewSessionPickerModel(nil, nil)})
	s.Update(SessionPickerResult{Selected: &thinkt.SessionMeta{ID: "session", FullPath: "fixture"}})
	item, ok := s.stack.Peek()
	if !ok {
		t.Fatal("session selection did not push a page")
	}
	m, ok := item.Model.(*ConversationViewerModel)
	if !ok || m.standalone {
		t.Fatalf("unexpected embedded page: %T", item.Model)
	}
	s.Close()
	if m.ctx.Err() == nil {
		t.Fatal("shell exit did not cancel conversation work")
	}
}
