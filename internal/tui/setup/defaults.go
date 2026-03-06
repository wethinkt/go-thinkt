package setup

import (
	"context"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/indexer/embedding"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// RunDefaults runs the setup wizard with all defaults accepted (--ok mode).
// It detects the locale, scans for sources, enables the indexer, disables
// embeddings, saves the config, and returns the Result.
func RunDefaults(factories []thinkt.StoreFactory) (Result, error) {
	lang := thinktI18n.ResolveLocale("")

	homeDir, err := config.Dir()
	if err != nil {
		return Result{}, err
	}

	// Scan for available sources — approve all found
	d := thinkt.NewDiscovery(factories...)
	detailed, err := d.DiscoverDetailed(context.Background(), nil)
	if err != nil {
		return Result{}, err
	}

	sources := make(map[string]bool, len(detailed))
	for _, info := range detailed {
		sources[string(info.Source)] = true
	}

	// Auto-detect terminal from environment
	apps := config.DefaultApps()
	terminal := config.DetectTerminal(apps)
	if terminal == "" {
		terminal = "terminal"
	}

	result := Result{
		Language:       lang,
		HomeDir:        homeDir,
		Sources:        sources,
		Terminal:       terminal,
		Indexer:        true,
		Embeddings:     false,
		EmbeddingModel: embedding.DefaultModelID,
		Completed:      true,
	}

	if err := SaveResult(result); err != nil {
		return Result{}, err
	}

	return result, nil
}

// SaveResult writes the wizard result to config.json.
func SaveResult(result Result) error {
	cfg := config.Default()
	cfg.Language = result.Language
	cfg.Indexer.Watch = result.Indexer
	cfg.Embedding.Enabled = result.Embeddings
	if result.EmbeddingModel != "" {
		cfg.Embedding.Model = result.EmbeddingModel
	}

	now := time.Now()
	cfg.DiscoveredAt = &now

	cfg.Terminal = result.Terminal

	cfg.Sources = make(map[string]config.SourceConfig, len(result.Sources))
	for name, enabled := range result.Sources {
		cfg.Sources[name] = config.SourceConfig{Enabled: enabled}
	}

	// Apply app enabled/disabled preferences from setup
	if result.Apps != nil {
		for i := range cfg.AllowedApps {
			if enabled, ok := result.Apps[cfg.AllowedApps[i].ID]; ok {
				cfg.AllowedApps[i].Enabled = enabled
			}
		}
	}

	return config.Save(cfg)
}
