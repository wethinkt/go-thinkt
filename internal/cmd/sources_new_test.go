package cmd

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// fakeFactory is a StoreFactory whose availability is fixed.
type fakeFactory struct {
	source    thinkt.Source
	available bool
}

func (f fakeFactory) Source() thinkt.Source         { return f.source }
func (f fakeFactory) Create() (thinkt.Store, error) { return nil, nil }
func (f fakeFactory) IsAvailable() (bool, error)    { return f.available, nil }

var testFactories = []thinkt.StoreFactory{
	fakeFactory{thinkt.SourceClaude, true},
	fakeFactory{thinkt.SourceAntigravity, true},
	fakeFactory{thinkt.SourceQwen, false},
}

func TestNewSourcesAreAvailableAndUnconfigured(t *testing.T) {
	cfg := config.Config{Sources: map[string]config.SourceConfig{"claude": {Enabled: true}}}
	if got := newSources(cfg, testFactories); !slices.Equal(got, []string{"antigravity"}) {
		t.Fatalf("%v", got)
	}
	// Without a sources map every source is enabled: nothing is new.
	if got := newSources(config.Config{}, testFactories); len(got) != 0 {
		t.Fatalf("nil map: %v", got)
	}
	// An explicitly disabled source is not new.
	cfg.Sources["antigravity"] = config.SourceConfig{Enabled: false}
	if got := newSources(cfg, testFactories); len(got) != 0 {
		t.Fatalf("disabled: %v", got)
	}
}

func TestAnnounceNewSourcesOnce(t *testing.T) {
	cfg := config.Config{Sources: map[string]config.SourceConfig{"claude": {Enabled: true}}}
	var buf bytes.Buffer
	if !announceNewSources(&buf, &cfg, testFactories) {
		t.Fatal("first announcement reported no change")
	}
	out := buf.String()
	if !strings.Contains(out, "antigravity") || !strings.Contains(out, "thinkt config sources enable antigravity") {
		t.Fatalf("%q", out)
	}
	if !slices.Equal(cfg.NotifiedSources, []string{"antigravity"}) {
		t.Fatalf("notified %v", cfg.NotifiedSources)
	}
	if _, ok := cfg.Sources["antigravity"]; ok {
		t.Fatal("announcement changed the source's enabled setting")
	}
	buf.Reset()
	if announceNewSources(&buf, &cfg, testFactories) || buf.Len() != 0 {
		t.Fatalf("second announcement: %q", buf.String())
	}
}

func TestSourceEnabledLabelMarksNew(t *testing.T) {
	cfg := config.Config{Sources: map[string]config.SourceConfig{"claude": {Enabled: true}, "kimi": {Enabled: false}}}
	for name, want := range map[string]string{"claude": "yes", "kimi": "no", "antigravity": "new"} {
		if got := sourceEnabledLabel(cfg, name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if got := sourceEnabledLabel(config.Config{}, "antigravity"); got != "yes" {
		t.Errorf("nil map: %q", got)
	}
}

func TestNewSourcesHintNamesEnableCommand(t *testing.T) {
	cfg := config.Config{Sources: map[string]config.SourceConfig{"claude": {Enabled: true}}}
	srcs := []thinkt.SourceInfo{
		{Source: thinkt.SourceClaude, Name: "claude", Available: true},
		{Source: thinkt.SourceAntigravity, Name: "antigravity", Available: true},
		{Source: thinkt.SourceQwen, Name: "qwen", Available: false},
	}
	var buf bytes.Buffer
	writeNewSourcesHint(&buf, cfg, srcs)
	if out := buf.String(); !strings.Contains(out, "thinkt config sources enable antigravity") || strings.Contains(out, "qwen") || strings.Contains(out, "claude") {
		t.Fatalf("%q", out)
	}
	buf.Reset()
	writeNewSourcesHint(&buf, config.Config{}, srcs)
	if buf.Len() != 0 {
		t.Fatalf("nil map: %q", buf.String())
	}
}

func TestUnregisteredSourcesAddsAvailableOnly(t *testing.T) {
	srcs := []thinkt.SourceInfo{{Source: thinkt.SourceClaude, Name: "claude", Available: true}}
	got := unregisteredSources(srcs, testFactories)
	if len(got) != 1 || got[0].Source != thinkt.SourceAntigravity || got[0].Name != "antigravity" || !got[0].Available {
		t.Fatalf("%+v", got)
	}
}

func TestSourcesCommandsSkipTheNotice(t *testing.T) {
	if !isSourcesCommand(sourcesListCmd) || !isSourcesCommand(sourcesEnableCmd) || isSourcesCommand(rootCmd) || isSourcesCommand(sessionsCmd) {
		t.Fatal("isSourcesCommand misclassifies")
	}
}
