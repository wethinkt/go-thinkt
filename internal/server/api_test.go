package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

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

	// Should have at least finder enabled
	var finderFound bool
	for _, app := range response.Apps {
		if app.ID == "finder" {
			finderFound = true
			break
		}
	}
	if !finderFound {
		t.Error("finder should be in the allowed apps list")
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

	body := []byte(`{"app": "finder"}`)
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
