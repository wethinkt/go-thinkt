package thinkt

import (
	"context"
	"fmt"
	"strings"
)

// availableSourcesString returns a comma-separated list of all known sources
// for use in error messages.
func availableSourcesString() string {
	names := make([]string, len(AllSources))
	for i, s := range AllSources {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

// ProjectsFromSources returns projects from the named sources.
// If sources is empty, returns projects from all registered sources.
func (r *StoreRegistry) ProjectsFromSources(ctx context.Context, sources []string) ([]Project, error) {
	if len(sources) == 0 {
		return r.ListAllProjects(ctx)
	}

	var allProjects []Project
	for _, sourceName := range sources {
		source := Source(sourceName)
		store, ok := r.Get(source)
		if !ok {
			return nil, fmt.Errorf("unknown source: %s (available: %s)", sourceName, availableSourcesString())
		}

		projects, err := store.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects from %s: %w", sourceName, err)
		}
		allProjects = append(allProjects, projects...)
	}

	return allProjects, nil
}

// SessionsForProject returns sessions for a project across the named sources.
// If sources is empty, searches all registered sources.
// projectID can be a store-specific ID or a filesystem path; both are tried.
// Results are returned in no guaranteed order; callers that need sorting should sort themselves.
func (r *StoreRegistry) SessionsForProject(ctx context.Context, projectID string, sources []string) ([]SessionMeta, error) {
	// Determine which stores to search.
	var stores []Store
	if len(sources) == 0 {
		stores = r.All()
	} else {
		for _, sourceName := range sources {
			source := Source(sourceName)
			store, ok := r.Get(source)
			if !ok {
				return nil, fmt.Errorf("unknown source: %s (available: %s)", sourceName, availableSourcesString())
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
	var allSessions []SessionMeta
	for _, store := range stores {
		// Try the projectID directly first.
		sessions, err := store.ListSessions(ctx, projectID)
		if err == nil && len(sessions) > 0 {
			allSessions = append(allSessions, sessions...)
			continue
		}

		// Find this store's project by filesystem path and use its ID.
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
