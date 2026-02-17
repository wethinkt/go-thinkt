package cmd

import (
	"context"
	"fmt"

	"github.com/wethinkt/go-thinkt/internal/sources"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// CreateSourceRegistry creates a registry with all discovered sources.
func CreateSourceRegistry() *thinkt.StoreRegistry {
	discovery := thinkt.NewDiscovery(sources.AllFactories()...)

	ctx := context.Background()
	registry, err := discovery.Discover(ctx)
	if err != nil {
		// Return empty registry on error
		return thinkt.NewRegistry()
	}

	return registry
}

// GetProjectsFromSources returns projects from the selected sources.
// If no sources specified, returns projects from all available sources.
func GetProjectsFromSources(registry *thinkt.StoreRegistry, sources []string) ([]thinkt.Project, error) {
	ctx := context.Background()

	// If no sources specified, use all available sources
	if len(sources) == 0 {
		return registry.ListAllProjects(ctx)
	}

	// Validate and collect projects from specified sources
	var allProjects []thinkt.Project
	for _, sourceName := range sources {
		source := thinkt.Source(sourceName)
		store, ok := registry.Get(source)
		if !ok {
			return nil, fmt.Errorf("unknown source: %s (available: claude, kimi, gemini, copilot, codex, qwen)", sourceName)
		}

		projects, err := store.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects from %s: %w", sourceName, err)
		}
		allProjects = append(allProjects, projects...)
	}

	return allProjects, nil
}

// GetSessionsForProject returns sessions for a project from the selected sources.
// If no sources specified, searches all available sources.
// The projectID can be a store-specific ID or a filesystem path; both are tried.
func GetSessionsForProject(registry *thinkt.StoreRegistry, projectID string, sources []string) ([]thinkt.SessionMeta, error) {
	ctx := context.Background()

	// Determine which stores to search
	var stores []thinkt.Store
	if len(sources) == 0 {
		stores = registry.All()
	} else {
		for _, sourceName := range sources {
			source := thinkt.Source(sourceName)
			store, ok := registry.Get(source)
			if !ok {
				return nil, fmt.Errorf("unknown source: %s (available: claude, kimi, gemini, copilot, codex, qwen)", sourceName)
			}
			stores = append(stores, store)
		}
	}

	// Resolve projectID to a filesystem path by checking all stores.
	// projectID might be a store-specific ID (e.g., ~/.claude/projects/...)
	// or a filesystem path (e.g., /Users/evan/myproject).
	resolvedPath := projectID
	for _, store := range stores {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		for _, p := range projects {
			if p.ID == projectID {
				resolvedPath = p.Path
				break
			}
		}
		if resolvedPath != projectID {
			break
		}
	}

	// Collect sessions from all stores that have a project matching the resolved path.
	var allSessions []thinkt.SessionMeta
	for _, store := range stores {
		// Try the projectID directly first
		sessions, err := store.ListSessions(ctx, projectID)
		if err == nil && len(sessions) > 0 {
			allSessions = append(allSessions, sessions...)
			continue
		}

		// Find this store's project by filesystem path and use its ID
		projects, err := store.ListProjects(ctx)
		if err != nil {
			continue
		}
		for _, p := range projects {
			if p.Path == resolvedPath {
				sessions, err := store.ListSessions(ctx, p.ID)
				if err == nil && len(sessions) > 0 {
					allSessions = append(allSessions, sessions...)
				}
				break
			}
		}
	}

	return allSessions, nil
}
