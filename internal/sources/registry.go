package sources

import (
	"context"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// EnabledRegistry discovers the stores of the sources the config enables.
// A config with a sources map is authoritative; with none, every source is
// enabled.
func EnabledRegistry() *thinkt.StoreRegistry {
	cfg, _ := config.Load()
	if cfg.Sources != nil {
		enabled := cfg.EnabledSources()
		if len(enabled) == 0 {
			return thinkt.NewRegistry()
		}
		return RegistryFor(enabled)
	}
	return RegistryFor(nil)
}

// RegistryFor discovers the stores of the named sources; an empty list
// means every source.
func RegistryFor(allowed []string) *thinkt.StoreRegistry {
	factories := AllFactories()
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
	registry, err := thinkt.NewDiscovery(factories...).Discover(context.Background())
	if err != nil {
		return thinkt.NewRegistry()
	}
	return registry
}

// Restrict returns a registry holding only the stores of reg whose source
// is in keep; an empty keep returns reg.
func Restrict(reg *thinkt.StoreRegistry, keep map[thinkt.Source]bool) *thinkt.StoreRegistry {
	if len(keep) == 0 {
		return reg
	}
	out := thinkt.NewRegistry()
	for _, s := range reg.All() {
		if keep[s.Source()] {
			out.Register(s)
		}
	}
	return out
}
