package discover

import (
	"testing"

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

func TestScanResultMsg(t *testing.T) {
	m := New(nil)
	m.scanning = true
	m.scanDone = false

	results := []thinkt.DetailedSourceInfo{
		{
			SourceInfo: thinkt.SourceInfo{
				Source: thinkt.SourceClaude,
				Name:   "Claude Code",
			},
			SessionCount: 42,
			TotalSize:    1048576,
		},
	}

	msg := scanResultMsg{results: results}
	updated, _ := m.Update(msg)
	um := updated.(Model)

	if um.scanning {
		t.Error("expected scanning=false after scanResultMsg")
	}
	if !um.scanDone {
		t.Error("expected scanDone=true after scanResultMsg")
	}
	if len(um.scanResults) != 1 {
		t.Errorf("expected 1 scan result, got %d", len(um.scanResults))
	}
	if um.scanResults[0].SessionCount != 42 {
		t.Errorf("expected SessionCount=42, got %d", um.scanResults[0].SessionCount)
	}
}
