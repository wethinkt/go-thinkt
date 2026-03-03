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
