package thinkt

import (
	"context"
)

// MultiStore provides a unified view across multiple stores.
type MultiStore struct {
	registry *StoreRegistry
}

// NewMultiStore creates a new multi-store view.
func NewMultiStore(registry *StoreRegistry) *MultiStore {
	return &MultiStore{registry: registry}
}

// ListAllProjects returns projects from all stores.
func (m *MultiStore) ListAllProjects(ctx context.Context) ([]Project, error) {
	return m.registry.ListAllProjects(ctx)
}

// ListProjects returns projects from a specific source.
func (m *MultiStore) ListProjects(ctx context.Context, source Source) ([]Project, error) {
	store, ok := m.registry.Get(source)
	if !ok {
		return nil, nil
	}
	return store.ListProjects(ctx)
}

// GetStore returns a store by source.
func (m *MultiStore) GetStore(source Source) (Store, bool) {
	return m.registry.Get(source)
}

// AllSources returns all available source types.
func (m *MultiStore) AllSources() []Source {
	return m.registry.Sources()
}

// AvailableSources returns sources that have data.
func (m *MultiStore) AvailableSources(ctx context.Context) []Source {
	return m.registry.AvailableSources(ctx)
}

// SourceStatus returns detailed status for all sources.
func (m *MultiStore) SourceStatus(ctx context.Context) []SourceInfo {
	return m.registry.SourceStatus(ctx)
}
