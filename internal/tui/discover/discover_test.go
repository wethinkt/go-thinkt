package discover

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestNewModel(t *testing.T) {
	m := New(nil)

	if m.step != stepWelcome {
		t.Errorf("expected step=stepWelcome, got %d", m.step)
	}

	if m.result.Sources == nil {
		t.Error("expected Sources map to be initialized, got nil")
	}

	if m.result.Completed {
		t.Error("expected Completed=false for new model")
	}

	if m.accent == "" {
		t.Error("expected accent color to be set from theme")
	}
}

func TestNewModelWithFactories(t *testing.T) {
	factories := []thinkt.StoreFactory{}
	m := New(factories)

	if m.step != stepWelcome {
		t.Errorf("expected step=stepWelcome, got %d", m.step)
	}

	if m.factories == nil {
		t.Error("expected factories to be set")
	}
}

func TestGetResult(t *testing.T) {
	m := New(nil)
	m.result.Language = "zh-Hans"
	m.result.HomeDir = "/home/test/.thinkt"
	m.result.Sources["claude"] = true
	m.result.Sources["kimi"] = false
	m.result.Indexer = true
	m.result.Embeddings = false
	m.result.Completed = true

	r := m.GetResult()

	if r.Language != "zh-Hans" {
		t.Errorf("expected Language='zh-Hans', got %q", r.Language)
	}
	if r.HomeDir != "/home/test/.thinkt" {
		t.Errorf("expected HomeDir='/home/test/.thinkt', got %q", r.HomeDir)
	}
	if !r.Sources["claude"] {
		t.Error("expected Sources['claude']=true")
	}
	if r.Sources["kimi"] {
		t.Error("expected Sources['kimi']=false")
	}
	if !r.Indexer {
		t.Error("expected Indexer=true")
	}
	if r.Embeddings {
		t.Error("expected Embeddings=false")
	}
	if !r.Completed {
		t.Error("expected Completed=true")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := formatBytes(tc.input)
			if got != tc.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSourceDiscoveredMsg(t *testing.T) {
	m := New(nil)
	m.step = stepSourceApproval
	m.scanning = true
	m.sourceMode = sourceModeAll
	m.scanCh = make(chan tea.Msg, 1) // buffered so waitForScan cmd doesn't block test

	info := thinkt.DetailedSourceInfo{
		SourceInfo: thinkt.SourceInfo{
			Source: thinkt.SourceClaude,
			Name:   "Claude Code",
		},
		SessionCount: 42,
		TotalSize:    1048576,
	}

	msg := sourceDiscoveredMsg{info: info}
	updated, _ := m.Update(msg)
	um := updated.(Model)

	if len(um.sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(um.sources))
	}
	if um.sources[0].Info.SessionCount != 42 {
		t.Errorf("expected SessionCount=42, got %d", um.sources[0].Info.SessionCount)
	}
	if !um.sources[0].Approved {
		t.Error("expected source to be auto-approved in sourceModeAll")
	}
}

func TestScanCompleteMsg(t *testing.T) {
	m := New(nil)
	m.step = stepSourceApproval
	m.scanning = true

	msg := scanCompleteMsg{}
	updated, _ := m.Update(msg)
	um := updated.(Model)

	if um.scanning {
		t.Error("expected scanning=false after scanCompleteMsg")
	}
	if !um.scanDone {
		t.Error("expected scanDone=true after scanCompleteMsg")
	}
}

func TestHomeYHotkey(t *testing.T) {
	m := New(nil)
	m.step = stepHome

	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	updated, _ := m.Update(msg)
	um := updated.(Model)

	if um.step != stepSourceConsent {
		t.Errorf("expected step=stepSourceConsent after Y, got %d", um.step)
	}
}

func TestHomeNHotkey(t *testing.T) {
	m := New(nil)
	m.step = stepHome

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	updated, cmd := m.Update(msg)
	um := updated.(Model)

	if um.step != stepHome {
		t.Errorf("expected step to remain stepHome after N, got %d", um.step)
	}
	if um.result.Completed {
		t.Error("expected Completed=false after N")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd after N")
	}
}

func TestHomeVerticalToggle(t *testing.T) {
	m := New(nil)
	m.step = stepHome
	m.confirm = true

	// up/down should toggle
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	updated, _ := m.Update(msg)
	um := updated.(Model)

	if um.confirm {
		t.Error("expected confirm=false after down key")
	}

	msg = tea.KeyPressMsg{Code: tea.KeyUp}
	updated, _ = um.Update(msg)
	um = updated.(Model)

	if !um.confirm {
		t.Error("expected confirm=true after up key")
	}
}

func TestStepIndicatorHiddenOnSuggestions(t *testing.T) {
	m := New(nil)
	m.step = stepSuggestions
	indicator := m.stepIndicator()
	if indicator != "" {
		t.Error("expected empty step indicator for suggestions step")
	}
}

func TestStepIndicatorVisibleBeforeSuggestions(t *testing.T) {
	m := New(nil)
	m.step = stepEmbeddings
	if m.stepIndicator() == "" {
		t.Error("expected non-empty step indicator before suggestions step")
	}
}

func TestViewUsesInlinePrimaryScreen(t *testing.T) {
	m := New(nil)
	m.step = stepWelcome
	m.width = 120
	m.height = 40

	view := m.View()
	if view.AltScreen {
		t.Fatal("expected discover view to render in primary screen (AltScreen=false)")
	}
	if strings.TrimSpace(view.Content) == "" {
		t.Fatal("expected discover view to render non-empty inline content")
	}
}

func TestInlineWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  int
	}{
		{name: "unknown width uses max", width: 0, want: discoverMaxWidth},
		{name: "large terminal clamps max", width: 180, want: discoverMaxWidth},
		{name: "normal terminal subtracts margin", width: 80, want: 78},
		{name: "small terminal keeps available width", width: 40, want: 38},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil)
			m.width = tc.width
			if got := m.inlineWidth(); got != tc.want {
				t.Fatalf("inlineWidth() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSourceDiscoveryShowsStickyContext(t *testing.T) {
	m := New(nil)
	m.step = stepSourceConsent
	m.result.Language = "en"
	m.result.HomeDir = "/Users/evan/.thinkt"

	view := m.viewSourceConsent()
	if !strings.Contains(view, "Welcome to 🧠 thinkt") {
		t.Fatal("expected sticky context welcome line in source discovery step")
	}
	if !strings.Contains(view, "English (en)") {
		t.Fatal("expected sticky context language summary in source discovery step")
	}
	if !strings.Contains(view, "/Users/evan/.thinkt") {
		t.Fatal("expected sticky context home directory in source discovery step")
	}
	if !strings.Contains(view, "Source Discovery") {
		t.Fatal("expected source discovery header to be present")
	}
}

func TestSuggestionsEnterDoesNotClearStep(t *testing.T) {
	m := New(nil)
	m.step = stepSuggestions

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, cmd := m.Update(msg)
	um := updated.(Model)

	if um.step != stepSuggestions {
		t.Fatalf("expected step to remain stepSuggestions on finish, got %d", um.step)
	}
	if !um.result.Completed {
		t.Fatal("expected Completed=true after finishing setup")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd when finishing setup")
	}
}
