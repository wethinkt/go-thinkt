// internal/thinkt/list_options.go
package thinkt

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

// WithEnrich enables background metadata enrichment. The callback is
// called after each batch of sessions is enriched, with the full
// refreshed session list for the project.
func WithEnrich(cb func(projectID string, sessions []SessionMeta)) ListSessionsOption {
	return func(cfg *ListSessionsConfig) {
		cfg.EnrichCallback = cb
	}
}
