package cmd

import (
	"testing"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestIsSourceDisabled_UsesSourcesMap(t *testing.T) {
	cfg := config.Config{
		Sources: map[string]config.SourceConfig{
			"claude": {Enabled: true},
			"kimi":   {Enabled: false},
		},
	}

	if isSourceDisabled(cfg, "claude") {
		t.Fatal("expected claude to be enabled from Sources map")
	}
	if !isSourceDisabled(cfg, "kimi") {
		t.Fatal("expected kimi to be disabled from Sources map")
	}
	if !isSourceDisabled(cfg, "copilot") {
		t.Fatal("expected missing source entry to be treated as disabled in Sources map mode")
	}
}

func TestIsSourceDisabled_AllEnabledWhenSourcesUnset(t *testing.T) {
	cfg := config.Config{}
	if isSourceDisabled(cfg, "claude") {
		t.Fatal("expected claude to be enabled when Sources is unset")
	}
	if isSourceDisabled(cfg, "qwen") {
		t.Fatal("expected qwen to be enabled when Sources is unset")
	}
}

func TestSetSourceEnabled_UpdatesSourcesMapConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("THINKT_HOME", tmp)

	cfg := config.Default()
	cfg.Sources = map[string]config.SourceConfig{
		"claude": {Enabled: true},
		"kimi":   {Enabled: false},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	outputJSON = false
	if err := setSourceEnabled("kimi", true); err != nil {
		t.Fatalf("setSourceEnabled: %v", err)
	}

	updated, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !updated.Sources["kimi"].Enabled {
		t.Fatal("expected kimi to be enabled in Sources map")
	}
}

func TestSetSourceEnabled_InitializesSourcesMapWhenUnset(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("THINKT_HOME", tmp)

	cfg := config.Default()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	outputJSON = false
	if err := setSourceEnabled("qwen", false); err != nil {
		t.Fatalf("setSourceEnabled: %v", err)
	}

	updated, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if updated.Sources == nil {
		t.Fatal("expected Sources map to be initialized")
	}
	if updated.Sources["qwen"].Enabled {
		t.Fatal("expected qwen to be disabled in initialized Sources map")
	}
	if !updated.Sources["claude"].Enabled {
		t.Fatal("expected other sources to remain enabled in initialized Sources map")
	}
}

func TestSetAllSourcesEnabled_UpdatesSourcesMapConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("THINKT_HOME", tmp)

	cfg := config.Default()
	cfg.Sources = map[string]config.SourceConfig{
		"claude": {Enabled: true},
		"kimi":   {Enabled: true},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	outputJSON = false
	if err := setAllSourcesEnabled(false); err != nil {
		t.Fatalf("setAllSourcesEnabled(false): %v", err)
	}

	updated, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	for _, s := range thinkt.AllSources {
		if updated.Sources[string(s)].Enabled {
			t.Fatalf("expected source %q to be disabled", s)
		}
	}
}
