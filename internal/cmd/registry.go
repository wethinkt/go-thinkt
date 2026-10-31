package cmd

import (
	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// CreateSourceRegistry creates a registry with all discovered sources.
// If config.Sources is present, it is treated as authoritative.
// If config.Sources is nil, all sources are considered enabled.
func CreateSourceRegistry() *thinkt.StoreRegistry {
	return sources.EnabledRegistry()
}

// CreateSourceRegistryFiltered creates a registry, optionally limited to the
// named sources. An empty or nil slice means "all sources".
func CreateSourceRegistryFiltered(allowed []string) *thinkt.StoreRegistry {
	return sources.RegistryFor(allowed)
}
