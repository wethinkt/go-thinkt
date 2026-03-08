// internal/thinkt/list_options.go
package thinkt

// ---------------------------------------------------------------------------
// ListSessions options
// ---------------------------------------------------------------------------

// ListSessionsOption configures optional behavior for ListSessions.
type ListSessionsOption func(*ListSessionsConfig)

// ListSessionsConfig holds resolved options for ListSessions.
type ListSessionsConfig struct {
	EnrichCallback func(projectID string, sessions []SessionMeta)
}

// ResolveListOptions applies option functions and returns the config.
func ResolveListOptions(opts []ListSessionsOption) ListSessionsConfig {
	var cfg ListSessionsConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithEnrich requests metadata enrichment for listed sessions. The callback
// receives the full refreshed session list for the project. Invocation
// semantics are store-dependent: stores that require expensive per-session
// parsing (Claude, Kimi) fire the callback asynchronously in a background
// goroutine, while stores that eagerly populate metadata during listing
// (Gemini, Copilot, Codex, Qwen) invoke it synchronously before returning.
// Callers must handle both cases.
func WithEnrich(cb func(projectID string, sessions []SessionMeta)) ListSessionsOption {
	return func(cfg *ListSessionsConfig) {
		cfg.EnrichCallback = cb
	}
}

// ---------------------------------------------------------------------------
// ListProjects options (used by StoreRegistry.ListAllProjects)
// ---------------------------------------------------------------------------

// ListProjectsOption configures optional behavior for ListAllProjects.
type ListProjectsOption func(*ListProjectsConfig)

// ListProjectsConfig holds resolved options for ListAllProjects.
type ListProjectsConfig struct {
	Sources        []string // Filter to these sources (empty = all)
	IncludeDeleted bool     // Include projects where PathExists=false
	Limit          int      // Max results (0 = no limit)
	Offset         int      // Skip this many results
}

// ResolveListProjectsOptions applies option functions and returns the config.
func ResolveListProjectsOptions(opts []ListProjectsOption) ListProjectsConfig {
	var cfg ListProjectsConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithSources filters projects to the given source names.
func WithSources(sources ...string) ListProjectsOption {
	return func(cfg *ListProjectsConfig) {
		cfg.Sources = sources
	}
}

// WithIncludeDeleted includes projects whose path no longer exists on disk.
func WithIncludeDeleted(b bool) ListProjectsOption {
	return func(cfg *ListProjectsConfig) {
		cfg.IncludeDeleted = b
	}
}

// WithLimit sets the maximum number of results to return.
func WithLimit(n int) ListProjectsOption {
	return func(cfg *ListProjectsConfig) {
		cfg.Limit = n
	}
}

// WithOffset sets the number of results to skip.
func WithOffset(n int) ListProjectsOption {
	return func(cfg *ListProjectsConfig) {
		cfg.Offset = n
	}
}
