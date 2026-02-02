package main

import (
	"context"
	"fmt"

	"github.com/wethinkt/go-thinkt/internal/sources/claude"
	"github.com/wethinkt/go-thinkt/internal/sources/copilot"
	"github.com/wethinkt/go-thinkt/internal/sources/gemini"
	"github.com/wethinkt/go-thinkt/internal/sources/kimi"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// createSourceRegistry creates a registry with all discovered sources.
func createSourceRegistry() *thinkt.StoreRegistry {
	// Create discovery with all source factories
	discovery := thinkt.NewDiscovery(
		kimi.Factory(),
		claude.Factory(),
		gemini.Factory(),
		copilot.Factory(),
	)

	ctx := context.Background()
	registry, err := discovery.Discover(ctx)
	if err != nil {
		// Return empty registry on error
		return thinkt.NewRegistry()
	}

	return registry
}

// getProjectsFromSources returns projects from the selected sources.
// If no sources specified, returns projects from all available sources.
func getProjectsFromSources(registry *thinkt.StoreRegistry, sources []string) ([]thinkt.Project, error) {
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
			return nil, fmt.Errorf("unknown source: %s (available: kimi, claude, gemini)", sourceName)
		}

		projects, err := store.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects from %s: %w", sourceName, err)
		}
		allProjects = append(allProjects, projects...)
	}

	return allProjects, nil
}

// getSessionsForProject returns sessions for a project from the selected sources.
// If no sources specified, searches all available sources.
func getSessionsForProject(registry *thinkt.StoreRegistry, projectID string, sources []string) ([]thinkt.SessionMeta, error) {
	ctx := context.Background()

	// If no sources specified, search all available sources
	if len(sources) == 0 {
		for _, store := range registry.All() {
			sessions, err := store.ListSessions(ctx, projectID)
			if err == nil && len(sessions) > 0 {
				return sessions, nil
			}
		}
		return []thinkt.SessionMeta{}, nil
	}

	// Validate and collect sessions from specified sources
	for _, sourceName := range sources {
		source := thinkt.Source(sourceName)
		store, ok := registry.Get(source)
		if !ok {
			return nil, fmt.Errorf("unknown source: %s (available: kimi, claude)", sourceName)
		}

		sessions, err := store.ListSessions(ctx, projectID)
		if err == nil && len(sessions) > 0 {
			return sessions, nil
		}
	}

	return []thinkt.SessionMeta{}, nil
}
