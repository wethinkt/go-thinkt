package thinkt

import (
	"context"
)

// StoreFactory creates Store instances for a specific source.
// Each source (Kimi, Claude, etc.) implements this interface
// to provide its own discovery and creation logic.
type StoreFactory interface {
	// Source returns the source type this factory creates stores for.
	Source() Source

	// Create attempts to create a Store for this source.
	// Returns (store, nil) if the source is available and has data.
	// Returns (nil, nil) if the source is not available (no data).
	// Returns (nil, error) if there was an error checking availability.
	Create() (Store, error)

	// IsAvailable checks if the source has data without creating a full store.
	// This is a lightweight check for discovery purposes.
	IsAvailable() (bool, error)
}

// Discovery manages source detection and store creation.
type Discovery struct {
	factories []StoreFactory
}

// NewDiscovery creates a new discovery manager with the given factories.
func NewDiscovery(factories ...StoreFactory) *Discovery {
	return &Discovery{factories: factories}
}

// Register adds a factory to the discovery manager.
func (d *Discovery) Register(factory StoreFactory) {
	d.factories = append(d.factories, factory)
}

// Discover finds all available sources and returns a populated registry.
// Factories that also implement TeamStoreFactory will have their team
// stores created and registered automatically.
func (d *Discovery) Discover(ctx context.Context) (*StoreRegistry, error) {
	registry := NewRegistry()

	for _, factory := range d.factories {
		store, err := factory.Create()
		if err != nil {
			// Log error but continue with other sources
			continue
		}
		if store != nil {
			// Verify the store actually has data
			projects, err := store.ListProjects(ctx)
			if err == nil && len(projects) > 0 {
				registry.Register(store)
			}
		}

		// Check if this factory also supports teams
		if tsf, ok := factory.(TeamStoreFactory); ok {
			ts, err := tsf.CreateTeamStore()
			if err == nil && ts != nil {
				registry.RegisterTeamStore(ts)
			}
		}
	}

	return registry, nil
}

// DiscoverAvailable returns only the sources that have data.
func (d *Discovery) DiscoverAvailable(ctx context.Context) ([]SourceInfo, error) {
	var available []SourceInfo

	for _, factory := range d.factories {
		isAvail, err := factory.IsAvailable()
		if err != nil || !isAvail {
			continue
		}

		store, err := factory.Create()
		if err != nil || store == nil {
			continue
		}

		ws := store.Workspace()
		projects, _ := store.ListProjects(ctx)

		info := SourceInfo{
			Source:       factory.Source(),
			Name:         factory.Source().DisplayName(),
			Description:  factory.Source().Description(),
			Available:    true,
			WorkspaceID:  ws.ID,
			BasePath:     ws.BasePath,
			ProjectCount: len(projects),
		}
		available = append(available, info)
	}

	return available, nil
}

// Factories returns all registered factories.
func (d *Discovery) Factories() []StoreFactory {
	return d.factories
}

// BasePathFunc is a function type that returns the base path for a source.
// Used by FileSystemFactory for customization.
type BasePathFunc func() (string, error)

// FileSystemFactory is a generic factory for sources that store data in
// a filesystem directory (like ~/.kimi or ~/.claude).
type FileSystemFactory struct {
	source       Source
	basePathFn   BasePathFunc
	storeCreator func(basePath string) Store
}

// NewFileSystemFactory creates a new filesystem-based factory.
func NewFileSystemFactory(
	source Source,
	basePathFn BasePathFunc,
	storeCreator func(basePath string) Store,
) *FileSystemFactory {
	return &FileSystemFactory{
		source:       source,
		basePathFn:   basePathFn,
		storeCreator: storeCreator,
	}
}

// Source returns the source type.
func (f *FileSystemFactory) Source() Source {
	return f.source
}

// Create creates a store if the source is available.
func (f *FileSystemFactory) Create() (Store, error) {
	basePath, err := f.basePathFn()
	if err != nil {
		return nil, err
	}

	if basePath == "" {
		return nil, nil
	}

	return f.storeCreator(basePath), nil
}

// IsAvailable checks if the source directory exists and has data.
func (f *FileSystemFactory) IsAvailable() (bool, error) {
	basePath, err := f.basePathFn()
	if err != nil {
		return false, err
	}
	if basePath == "" {
		return false, nil
	}

	// Quick check: does the directory exist?
	store := f.storeCreator(basePath)
	ctx := context.Background()
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return false, nil // Not available, but not an error
	}

	return len(projects) > 0, nil
}
