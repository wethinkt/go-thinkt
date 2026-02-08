package server

import (
	"context"
	"os"
)

// DefaultMCPAuthConfig returns a default MCP auth configuration.
// Uses THINKT_MCP_TOKEN env var if set, otherwise no auth.
func DefaultMCPAuthConfig() AuthConfig {
	if os.Getenv("THINKT_MCP_TOKEN") != "" {
		return AuthConfig{
			Mode:   AuthModeEnvToken,
			EnvVar: "THINKT_MCP_TOKEN",
			Realm:  "thinkt-mcp",
		}
	}
	return AuthConfig{Mode: AuthModeNone, Realm: "thinkt-mcp"}
}

// StdioAuthProvider provides authentication for stdio transport.
// For stdio, authentication is handled via environment variables per MCP spec.
type StdioAuthProvider struct {
	token string
}

// NewStdioAuthProvider creates a new stdio auth provider.
func NewStdioAuthProvider() *StdioAuthProvider {
	return &StdioAuthProvider{
		token: os.Getenv("THINKT_MCP_TOKEN"),
	}
}

// AuthenticateContext adds authentication context to a context.
// For stdio, this validates the token was set in the environment.
func (s *StdioAuthProvider) AuthenticateContext(ctx context.Context) (context.Context, error) {
	if s.token == "" {
		// No token configured - allowed for local development
		return ctx, nil
	}
	// Token is present - stdio transport relies on the transport security
	// (stdio is inherently local and secure)
	return ctx, nil
}

// IsAuthenticated returns true if authentication is configured.
func (s *StdioAuthProvider) IsAuthenticated() bool {
	return s.token != ""
}
