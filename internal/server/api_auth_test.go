package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDefaultAPIAuthConfig(t *testing.T) {
	// Test with no env var set
	os.Unsetenv("THINKT_API_TOKEN")
	config := DefaultAPIAuthConfig()

	if config.Mode != APIAuthModeNone {
		t.Errorf("DefaultAPIAuthConfig() Mode = %v, want APIAuthModeNone when env not set", config.Mode)
	}

	// Test with env var set
	os.Setenv("THINKT_API_TOKEN", "test-api-token")
	defer os.Unsetenv("THINKT_API_TOKEN")

	config2 := DefaultAPIAuthConfig()
	if config2.Mode != APIAuthModeEnvToken {
		t.Errorf("DefaultAPIAuthConfig() Mode = %v, want APIAuthModeEnvToken when env set", config2.Mode)
	}
}

func TestAPIAuthenticator_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   APIAuthConfig
		expected bool
	}{
		{"none", APIAuthConfig{Mode: APIAuthModeNone}, false},
		{"token", APIAuthConfig{Mode: APIAuthModeToken, Token: "test"}, true},
		{"env", APIAuthConfig{Mode: APIAuthModeEnvToken}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAPIAuthenticator(tt.config)
			if got := auth.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAPIAuthenticator_AuthenticateRequest(t *testing.T) {
	tests := []struct {
		name       string
		config     APIAuthConfig
		authHeader string
		wantStatus int
		wantAuth   bool
	}{
		{
			name:       "APIAuthModeNone allows all",
			config:     APIAuthConfig{Mode: APIAuthModeNone},
			authHeader: "",
			wantStatus: http.StatusOK,
			wantAuth:   true,
		},
		{
			name:       "APIAuthModeToken valid token",
			config:     APIAuthConfig{Mode: APIAuthModeToken, Token: "valid-token-123"},
			authHeader: "Bearer valid-token-123",
			wantStatus: http.StatusOK,
			wantAuth:   true,
		},
		{
			name:       "APIAuthModeToken invalid token",
			config:     APIAuthConfig{Mode: APIAuthModeToken, Token: "valid-token-123"},
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "APIAuthModeToken missing header",
			config:     APIAuthConfig{Mode: APIAuthModeToken, Token: "valid-token-123"},
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "APIAuthModeToken wrong format",
			config:     APIAuthConfig{Mode: APIAuthModeToken, Token: "valid-token-123"},
			authHeader: "Basic dXNlcjpwYXNz",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "APIAuthModeToken empty token",
			config:     APIAuthConfig{Mode: APIAuthModeToken, Token: ""},
			authHeader: "Bearer some-token",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAPIAuthenticator(tt.config)

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			gotAuth := auth.AuthenticateRequest(rr, req)

			if gotAuth != tt.wantAuth {
				t.Errorf("AuthenticateRequest() = %v, want %v", gotAuth, tt.wantAuth)
			}

			if rr.Code != tt.wantStatus {
				t.Errorf("AuthenticateRequest() status = %d, want %d", rr.Code, tt.wantStatus)
			}

			// Check for WWW-Authenticate header on 401 responses
			if rr.Code == http.StatusUnauthorized {
				wwwAuth := rr.Header().Get("WWW-Authenticate")
				if wwwAuth == "" {
					t.Error("Expected WWW-Authenticate header on 401 response")
				}
			}
		})
	}
}

func TestAPIAuthenticator_AuthModeEnvToken(t *testing.T) {
	// Set environment variable
	os.Setenv("THINKT_API_TOKEN", "env-api-token")
	defer os.Unsetenv("THINKT_API_TOKEN")

	config := APIAuthConfig{
		Mode:   APIAuthModeEnvToken,
		EnvVar: "THINKT_API_TOKEN",
	}
	auth := NewAPIAuthenticator(config)

	// Valid token from env
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer env-api-token")
	rr := httptest.NewRecorder()

	if !auth.AuthenticateRequest(rr, req) {
		t.Error("AuthenticateRequest() should succeed with valid env token")
	}

	// Invalid token
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token")
	rr2 := httptest.NewRecorder()

	if auth.AuthenticateRequest(rr2, req2) {
		t.Error("AuthenticateRequest() should fail with invalid token")
	}

	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", rr2.Code)
	}
}

func TestAPIAuthenticator_Middleware(t *testing.T) {
	config := APIAuthConfig{
		Mode:  APIAuthModeToken,
		Token: "secret-api-token",
	}
	auth := NewAPIAuthenticator(config)

	// Create a simple handler that returns 200
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Wrap with auth middleware
	wrapped := auth.Middleware(handler)

	// Test without auth (should fail)
	req1 := httptest.NewRequest("GET", "/test", nil)
	rr1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("Middleware() without auth status = %d, want %d", rr1.Code, http.StatusUnauthorized)
	}

	// Test with auth (should succeed)
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer secret-api-token")
	rr2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Middleware() with auth status = %d, want %d", rr2.Code, http.StatusOK)
	}

	if rr2.Body.String() != "success" {
		t.Errorf("Middleware() body = %q, want %q", rr2.Body.String(), "success")
	}
}

func TestAPIAuthenticator_OptionalMiddleware(t *testing.T) {
	// Test with auth disabled - should allow all
	authNone := NewAPIAuthenticator(APIAuthConfig{Mode: APIAuthModeNone})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedNone := authNone.OptionalMiddleware(handler)
	req1 := httptest.NewRequest("GET", "/test", nil)
	rr1 := httptest.NewRecorder()
	wrappedNone.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Errorf("OptionalMiddleware() with no auth status = %d, want %d", rr1.Code, http.StatusOK)
	}

	// Test with auth enabled - should require token
	authToken := NewAPIAuthenticator(APIAuthConfig{Mode: APIAuthModeToken, Token: "token"})
	wrappedToken := authToken.OptionalMiddleware(handler)

	// Without token should fail
	req2 := httptest.NewRequest("GET", "/test", nil)
	rr2 := httptest.NewRecorder()
	wrappedToken.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("OptionalMiddleware() with auth, no token status = %d, want %d", rr2.Code, http.StatusUnauthorized)
	}

	// With token should succeed
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("Authorization", "Bearer token")
	rr3 := httptest.NewRecorder()
	wrappedToken.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusOK {
		t.Errorf("OptionalMiddleware() with auth, valid token status = %d, want %d", rr3.Code, http.StatusOK)
	}
}
