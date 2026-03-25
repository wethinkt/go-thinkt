package cmd

import (
	"context"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// CreateSourceRegistry creates a registry with all discovered sources.
// If config.Sources is present, it is treated as authoritative.
// If config.Sources is nil, all sources are considered enabled.
func CreateSourceRegistry() *thinkt.StoreRegistry {
	cfg, _ := config.Load()
	allowed, constrained := configuredEnabledSources(cfg)
	if constrained && len(allowed) == 0 {
		return thinkt.NewRegistry()
	}
	return CreateSourceRegistryFiltered(allowed)
}

// configuredEnabledSources returns the enabled source names derived from config,
// along with whether source selection is explicitly constrained.
func configuredEnabledSources(cfg config.Config) ([]string, bool) {
	if cfg.Sources != nil {
		enabled := cfg.EnabledSources()
		if enabled == nil {
			return []string{}, true
		}
		return enabled, true
	}
	return nil, false
}

// CreateSourceRegistryFiltered creates a registry, optionally limited to the
// named sources. An empty or nil slice means "all sources".
func CreateSourceRegistryFiltered(allowed []string) *thinkt.StoreRegistry {
	factories := sources.AllFactories()
	if len(allowed) > 0 {
		set := make(map[string]bool, len(allowed))
		for _, s := range allowed {
			set[s] = true
		}
		var filtered []thinkt.StoreFactory
		for _, f := range factories {
			if set[string(f.Source())] {
				filtered = append(filtered, f)
				delete(set, string(f.Source()))
			}
		}
		for name := range set {
			tuilog.Log.Warn("unknown source in config", "source", name)
		}
		factories = filtered
	}

	discovery := thinkt.NewDiscovery(factories...)
	ctx := context.Background()
	registry, err := discovery.Discover(ctx)
	if err != nil {
		return thinkt.NewRegistry()
	}

	return registry
}

