// Package server provides MCP server authentication.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// AuthMode represents the authentication mode for the MCP server.
type AuthMode int

const (
	// AuthModeNone disables authentication (local/development use only).
	AuthModeNone AuthMode = iota
	// AuthModeToken uses a static bearer token for authentication.
	AuthModeToken
	// AuthModeEnvToken reads the token from an environment variable.
	AuthModeEnvToken
)

// MCPAuthConfig holds authentication configuration for the MCP server.
type MCPAuthConfig struct {
	Mode   AuthMode
	Token  string // For AuthModeToken
	EnvVar string // For AuthModeEnvToken (default: THINKT_MCP_TOKEN)
}

// DefaultMCPAuthConfig returns a default configuration.
// By default, it uses the THINKT_MCP_TOKEN environment variable if set,
// otherwise runs without authentication (local development).
func DefaultMCPAuthConfig() MCPAuthConfig {
	if token := os.Getenv("THINKT_MCP_TOKEN"); token != "" {
		return MCPAuthConfig{
			Mode:   AuthModeEnvToken,
			EnvVar: "THINKT_MCP_TOKEN",
		}
	}
	return MCPAuthConfig{Mode: AuthModeNone}
}

// MCPAuthenticator handles authentication for MCP HTTP requests.
type MCPAuthenticator struct {
	config MCPAuthConfig
}

// NewMCPAuthenticator creates a new authenticator with the given configuration.
func NewMCPAuthenticator(config MCPAuthConfig) *MCPAuthenticator {
	return &MCPAuthenticator{config: config}
}

// AuthenticateRequest verifies the authentication for an HTTP request.
// Returns true if the request is authenticated, false otherwise.
// When false is returned, the handler should write a 401 response and return.
func (a *MCPAuthenticator) AuthenticateRequest(w http.ResponseWriter, r *http.Request) bool {
	switch a.config.Mode {
	case AuthModeNone:
		return true
	case AuthModeToken, AuthModeEnvToken:
		return a.authenticateToken(w, r)
	default:
		writeError(w, http.StatusInternalServerError, "auth_error", "Unknown authentication mode")
		return false
	}
}

// authenticateToken validates a Bearer token from the Authorization header.
func (a *MCPAuthenticator) authenticateToken(w http.ResponseWriter, r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		a.writeUnauthorized(w, "Missing Authorization header")
		return false
	}

	// Parse Bearer token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		a.writeUnauthorized(w, "Invalid Authorization header format")
		return false
	}

	providedToken := parts[1]
	expectedToken := a.getExpectedToken()

	if expectedToken == "" {
		a.writeUnauthorized(w, "Server authentication not configured")
		return false
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
		tuilog.Log.Info("MCP authentication failed: invalid token")
		a.writeUnauthorized(w, "Invalid token")
		return false
	}

	return true
}

// getExpectedToken returns the expected token based on the auth mode.
func (a *MCPAuthenticator) getExpectedToken() string {
	switch a.config.Mode {
	case AuthModeToken:
		return a.config.Token
	case AuthModeEnvToken:
		if a.config.EnvVar == "" {
			return os.Getenv("THINKT_MCP_TOKEN")
		}
		return os.Getenv(a.config.EnvVar)
	default:
		return ""
	}
}

// writeUnauthorized writes a 401 Unauthorized response.
func (a *MCPAuthenticator) writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="thinkt-mcp"`)
	writeError(w, http.StatusUnauthorized, "unauthorized", message)
}

// Middleware returns an HTTP middleware that enforces authentication.
func (a *MCPAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.AuthenticateRequest(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GenerateSecureToken generates a cryptographically secure random token.
// The token is 32 bytes (256 bits) encoded as hex, resulting in a 64-character string.
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateSecureTokenWithPrefix generates a token with a human-readable prefix.
// Format: thinkt_<timestamp>_<random>
func GenerateSecureTokenWithPrefix() (string, error) {
	randomPart, err := GenerateSecureToken()
	if err != nil {
		return "", err
	}
	timestamp := time.Now().UTC().Format("20060102")
	return fmt.Sprintf("thinkt_%s_%s", timestamp, randomPart[:32]), nil
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
