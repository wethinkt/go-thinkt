// Package server provides API server authentication.
package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// APIAuthMode represents the authentication mode for the API server.
type APIAuthMode int

const (
	// APIAuthModeNone disables authentication (local/development use only).
	APIAuthModeNone APIAuthMode = iota
	// APIAuthModeToken uses a static bearer token for authentication.
	APIAuthModeToken
	// APIAuthModeEnvToken reads the token from an environment variable.
	APIAuthModeEnvToken
)

// APIAuthConfig holds authentication configuration for the API server.
type APIAuthConfig struct {
	Mode   APIAuthMode
	Token  string // For APIAuthModeToken
	EnvVar string // For APIAuthModeEnvToken (default: THINKT_API_TOKEN)
}

// DefaultAPIAuthConfig returns a default configuration.
// By default, it uses the THINKT_API_TOKEN environment variable if set,
// otherwise runs without authentication (local development).
func DefaultAPIAuthConfig() APIAuthConfig {
	if token := os.Getenv("THINKT_API_TOKEN"); token != "" {
		return APIAuthConfig{
			Mode:   APIAuthModeEnvToken,
			EnvVar: "THINKT_API_TOKEN",
		}
	}
	return APIAuthConfig{Mode: APIAuthModeNone}
}

// APIAuthenticator handles authentication for API HTTP requests.
type APIAuthenticator struct {
	config APIAuthConfig
}

// NewAPIAuthenticator creates a new authenticator with the given configuration.
func NewAPIAuthenticator(config APIAuthConfig) *APIAuthenticator {
	return &APIAuthenticator{config: config}
}

// IsEnabled returns true if authentication is enabled.
func (a *APIAuthenticator) IsEnabled() bool {
	return a.config.Mode != APIAuthModeNone
}

// AuthenticateRequest verifies the authentication for an HTTP request.
// Returns true if the request is authenticated, false otherwise.
// When false is returned, the handler should write a 401 response and return.
func (a *APIAuthenticator) AuthenticateRequest(w http.ResponseWriter, r *http.Request) bool {
	switch a.config.Mode {
	case APIAuthModeNone:
		return true
	case APIAuthModeToken, APIAuthModeEnvToken:
		return a.authenticateToken(w, r)
	default:
		writeError(w, http.StatusInternalServerError, "auth_error", "Unknown authentication mode")
		return false
	}
}

// authenticateToken validates a Bearer token from the Authorization header.
func (a *APIAuthenticator) authenticateToken(w http.ResponseWriter, r *http.Request) bool {
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
		tuilog.Log.Info("API authentication failed: invalid token", "remote", r.RemoteAddr)
		a.writeUnauthorized(w, "Invalid token")
		return false
	}

	return true
}

// getExpectedToken returns the expected token based on the auth mode.
func (a *APIAuthenticator) getExpectedToken() string {
	switch a.config.Mode {
	case APIAuthModeToken:
		return a.config.Token
	case APIAuthModeEnvToken:
		if a.config.EnvVar == "" {
			return os.Getenv("THINKT_API_TOKEN")
		}
		return os.Getenv(a.config.EnvVar)
	default:
		return ""
	}
}

// writeUnauthorized writes a 401 Unauthorized response.
func (a *APIAuthenticator) writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="thinkt-api"`)
	writeError(w, http.StatusUnauthorized, "unauthorized", message)
}

// Middleware returns an HTTP middleware that enforces authentication.
func (a *APIAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.AuthenticateRequest(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OptionalMiddleware returns middleware that authenticates if a token is configured,
// but allows requests through if no token is set. This is useful for endpoints
// that can work in both authenticated and unauthenticated modes.
func (a *APIAuthenticator) OptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If auth is not configured, just pass through
		if a.config.Mode == APIAuthModeNone {
			next.ServeHTTP(w, r)
			return
		}

		// Auth is configured, require it
		if !a.AuthenticateRequest(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}
