package server

import (
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

// AuthMode represents the authentication mode.
type AuthMode int

const (
	// AuthModeNone disables authentication (local/development use only).
	AuthModeNone AuthMode = iota
	// AuthModeToken uses a static bearer token for authentication.
	AuthModeToken
	// AuthModeEnvToken reads the token from an environment variable.
	AuthModeEnvToken
)

// AuthConfig holds bearer token authentication configuration.
type AuthConfig struct {
	Mode   AuthMode
	Token  string // for AuthModeToken
	EnvVar string // for AuthModeEnvToken
	Realm  string // WWW-Authenticate realm (e.g. "thinkt-api", "thinkt-mcp")
}

// BearerAuthenticator handles bearer token authentication for HTTP requests.
type BearerAuthenticator struct {
	config AuthConfig
}

// NewBearerAuthenticator creates a new authenticator with the given configuration.
func NewBearerAuthenticator(config AuthConfig) *BearerAuthenticator {
	return &BearerAuthenticator{config: config}
}

// IsEnabled returns true if authentication is enabled.
func (a *BearerAuthenticator) IsEnabled() bool {
	return a.config.Mode != AuthModeNone
}

// AuthenticateRequest verifies the authentication for an HTTP request.
// Returns true if the request is authenticated, false otherwise.
func (a *BearerAuthenticator) AuthenticateRequest(w http.ResponseWriter, r *http.Request) bool {
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

func (a *BearerAuthenticator) authenticateToken(w http.ResponseWriter, r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")

	// Fall back to ?token= query param (for browser URL opening)
	if authHeader == "" {
		if qToken := r.URL.Query().Get("token"); qToken != "" {
			authHeader = "Bearer " + qToken
		}
	}

	if authHeader == "" {
		a.writeUnauthorized(w, "Missing Authorization header")
		return false
	}

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

	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) != 1 {
		tuilog.Log.Info("Authentication failed: invalid token", "realm", a.config.Realm, "remote", r.RemoteAddr)
		a.writeUnauthorized(w, "Invalid token")
		return false
	}

	return true
}

func (a *BearerAuthenticator) getExpectedToken() string {
	switch a.config.Mode {
	case AuthModeToken:
		return a.config.Token
	case AuthModeEnvToken:
		return os.Getenv(a.config.EnvVar)
	default:
		return ""
	}
}

func (a *BearerAuthenticator) writeUnauthorized(w http.ResponseWriter, message string) {
	realm := a.config.Realm
	if realm == "" {
		realm = "thinkt"
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, realm))
	writeError(w, http.StatusUnauthorized, "unauthorized", message)
}

// Middleware returns an HTTP middleware that enforces authentication.
func (a *BearerAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.AuthenticateRequest(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OptionalMiddleware returns middleware that authenticates if a token is configured,
// but allows requests through if no token is set.
func (a *BearerAuthenticator) OptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.config.Mode == AuthModeNone {
			next.ServeHTTP(w, r)
			return
		}
		if !a.AuthenticateRequest(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

///////////////////////////////////////////////////////////////////////////////
// Token generation
///////////////////////////////////////////////////////////////////////////////

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
