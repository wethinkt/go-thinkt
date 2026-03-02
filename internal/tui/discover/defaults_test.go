package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/config"
)

func TestRunDefaultsCreatesConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("THINKT_HOME", tmp)

	result, err := RunDefaults(nil)
	if err != nil {
		t.Fatalf("RunDefaults: %v", err)
	}

	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if !result.Indexer {
		t.Error("expected Indexer=true")
	}
	if result.Embeddings {
		t.Error("expected Embeddings=false")
	}

	// Config file should exist
	configPath := filepath.Join(tmp, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid config JSON: %v", err)
	}

	if cfg.DiscoveredAt == nil {
		t.Error("expected DiscoveredAt to be set")
	}
	if !cfg.Indexer.Watch {
		t.Error("expected Indexer.Watch=true")
	}
	if cfg.Embedding.Enabled {
		t.Error("expected Embedding.Enabled=false")
	}
}

func TestSaveResult(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("THINKT_HOME", tmp)

	r := Result{
		Language:   "es",
		Indexer:    true,
		Embeddings: false,
		Sources:    map[string]bool{"claude": true, "kimi": false},
	}
	if err := SaveResult(r); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	if cfg.Language != "es" {
		t.Errorf("Language = %q, want %q", cfg.Language, "es")
	}
	if !cfg.Indexer.Watch {
		t.Errorf("Indexer.Watch = false, want true")
	}
	if cfg.Embedding.Enabled {
		t.Errorf("Embedding.Enabled = true, want false")
	}
	if cfg.DiscoveredAt == nil {
		t.Error("expected DiscoveredAt to be set")
	}

	// Check sources
	claudeSrc, ok := cfg.Sources["claude"]
	if !ok {
		t.Fatal("missing source 'claude'")
	}
	if !claudeSrc.Enabled {
		t.Error("expected claude source to be enabled")
	}

	kimiSrc, ok := cfg.Sources["kimi"]
	if !ok {
		t.Fatal("missing source 'kimi'")
	}
	if kimiSrc.Enabled {
		t.Error("expected kimi source to be disabled")
	}
}
