package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// platformFileManager returns the default file manager app ID for the current OS.
func platformFileManager() string {
	switch runtime.GOOS {
	case "darwin":
		return "finder"
	case "windows":
		return "explorer"
	default:
		return "files"
	}
}

func TestHandleGetAllowedApps(t *testing.T) {
	// Create a test server
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServer(registry, config)

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/open-in/apps", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response AllowedAppsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have the platform file manager
	expectedApp := platformFileManager()
	var found bool
	for _, app := range response.Apps {
		if app.ID == expectedApp {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("%s should be in the allowed apps list", expectedApp)
	}
}

func TestHandleOpenIn_MissingApp(t *testing.T) {
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServer(registry, config)

	body := []byte(`{"path": "/some/path"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/open-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleOpenIn_MissingPath(t *testing.T) {
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServer(registry, config)

	body := []byte(`{"app": "` + platformFileManager() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/open-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleOpenIn_DisallowedApp(t *testing.T) {
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServer(registry, config)

	body := []byte(`{"app": "malicious_app", "path": "/some/path"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/open-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestHandleOpenIn_CrossOriginRejected(t *testing.T) {
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServerWithAuth(registry, config, AuthConfig{Mode: AuthModeNone})

	body := []byte(`{"app": "` + platformFileManager() + `", "path": "/some/path"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/open-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	var response ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Error != "forbidden" {
		t.Fatalf("expected error code %q, got %q", "forbidden", response.Error)
	}
}

func TestHandleOpenIn_SameOriginReachesAppValidation(t *testing.T) {
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServerWithAuth(registry, config, AuthConfig{Mode: AuthModeNone})

	body := []byte(`{"app": "malicious_app", "path": "/some/path"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/open-in", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	var response ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Error != "app_not_allowed" {
		t.Fatalf("expected error code %q, got %q", "app_not_allowed", response.Error)
	}
}
