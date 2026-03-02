package discover

import (
	"context"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
	thinktI18n "github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// RunDefaults runs the discover wizard with all defaults accepted (--ok mode).
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

	result := Result{
		Language:   lang,
		HomeDir:    homeDir,
		Sources:    sources,
		Indexer:    true,
		Embeddings: false,
		Completed:  true,
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

	now := time.Now()
	cfg.DiscoveredAt = &now

	cfg.Sources = make(map[string]config.SourceConfig, len(result.Sources))
	for name, enabled := range result.Sources {
		cfg.Sources[name] = config.SourceConfig{Enabled: enabled}
	}

	return config.Save(cfg)
}
