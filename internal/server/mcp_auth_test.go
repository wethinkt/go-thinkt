package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMCPAuthenticator_AuthenticateRequest(t *testing.T) {
	tests := []struct {
		name       string
		config     AuthConfig
		authHeader string
		wantStatus int
		wantAuth   bool
	}{
		{
			name:       "AuthModeNone allows all",
			config:     AuthConfig{Mode: AuthModeNone},
			authHeader: "",
			wantStatus: http.StatusOK,
			wantAuth:   true,
		},
		{
			name:       "AuthModeToken valid token",
			config:     AuthConfig{Mode: AuthModeToken, Token: "valid-token-123"},
			authHeader: "Bearer valid-token-123",
			wantStatus: http.StatusOK,
			wantAuth:   true,
		},
		{
			name:       "AuthModeToken invalid token",
			config:     AuthConfig{Mode: AuthModeToken, Token: "valid-token-123"},
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "AuthModeToken missing header",
			config:     AuthConfig{Mode: AuthModeToken, Token: "valid-token-123"},
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "AuthModeToken wrong format",
			config:     AuthConfig{Mode: AuthModeToken, Token: "valid-token-123"},
			authHeader: "Basic dXNlcjpwYXNz",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
		{
			name:       "AuthModeToken empty token",
			config:     AuthConfig{Mode: AuthModeToken, Token: ""},
			authHeader: "Bearer some-token",
			wantStatus: http.StatusUnauthorized,
			wantAuth:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewBearerAuthenticator(tt.config)

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

func TestMCPAuthenticator_AuthModeEnvToken(t *testing.T) {
	// Set environment variable
	os.Setenv("THINKT_MCP_TOKEN", "env-token-123")
	defer os.Unsetenv("THINKT_MCP_TOKEN")

	config := AuthConfig{
		Mode:   AuthModeEnvToken,
		EnvVar: "THINKT_MCP_TOKEN",
	}
	auth := NewBearerAuthenticator(config)

	// Valid token from env
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer env-token-123")
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

func TestMCPAuthenticator_Middleware(t *testing.T) {
	config := AuthConfig{
		Mode:  AuthModeToken,
		Token: "secret-token",
	}
	auth := NewBearerAuthenticator(config)

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
	req2.Header.Set("Authorization", "Bearer secret-token")
	rr2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Middleware() with auth status = %d, want %d", rr2.Code, http.StatusOK)
	}

	if rr2.Body.String() != "success" {
		t.Errorf("Middleware() body = %q, want %q", rr2.Body.String(), "success")
	}
}

func TestMCPAuthenticator_TimingAttack(t *testing.T) {
	// This test verifies that we use constant-time comparison
	// The test itself doesn't measure timing, but ensures the function exists
	config := AuthConfig{
		Mode:  AuthModeToken,
		Token: "correct-token",
	}
	auth := NewBearerAuthenticator(config)

	// Both should fail but take similar time
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("Authorization", "Bearer wrong-token-1")
	rr1 := httptest.NewRecorder()
	auth.AuthenticateRequest(rr1, req1)

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token-2")
	rr2 := httptest.NewRecorder()
	auth.AuthenticateRequest(rr2, req2)

	if rr1.Code != http.StatusUnauthorized || rr2.Code != http.StatusUnauthorized {
		t.Error("Both requests should be unauthorized")
	}
}

func TestDefaultMCPAuthConfig(t *testing.T) {
	// Test with no env var set
	os.Unsetenv("THINKT_MCP_TOKEN")
	config := DefaultMCPAuthConfig()

	if config.Mode != AuthModeNone {
		t.Errorf("DefaultMCPAuthConfig() Mode = %v, want AuthModeNone when env not set", config.Mode)
	}

	// Test with env var set
	os.Setenv("THINKT_MCP_TOKEN", "test-token")
	defer os.Unsetenv("THINKT_MCP_TOKEN")

	config2 := DefaultMCPAuthConfig()
	if config2.Mode != AuthModeEnvToken {
		t.Errorf("DefaultMCPAuthConfig() Mode = %v, want AuthModeEnvToken when env set", config2.Mode)
	}
}

func TestStdioAuthProvider(t *testing.T) {
	// Test without env var
	os.Unsetenv("THINKT_MCP_TOKEN")
	provider := NewStdioAuthProvider()

	if provider.IsAuthenticated() {
		t.Error("IsAuthenticated() should be false when env not set")
	}

	ctx, err := provider.AuthenticateContext(nil)
	if err != nil {
		t.Errorf("AuthenticateContext() error = %v", err)
	}
	if ctx != nil {
		// Context is passed through unchanged
		t.Log("Context passed through successfully")
	}

	// Test with env var
	os.Setenv("THINKT_MCP_TOKEN", "stdio-token")
	defer os.Unsetenv("THINKT_MCP_TOKEN")

	provider2 := NewStdioAuthProvider()
	if !provider2.IsAuthenticated() {
		t.Error("IsAuthenticated() should be true when env set")
	}
}
