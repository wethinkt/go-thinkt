package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/i18n"
	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestHandleGetLanguages(t *testing.T) {
	// Initialize i18n
	i18n.Init("en")

	// Create a test server
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServer(registry, config)

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/languages", nil)
	w := httptest.NewRecorder()

	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response LanguagesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Active != "en" {
		t.Errorf("expected active language 'en', got %q", response.Active)
	}

	// Should have at least English, Chinese, and Spanish
	expectedTags := map[string]bool{
		"en":      false,
		"zh-Hans": false,
		"es":      false,
	}

	for _, lang := range response.Languages {
		if _, ok := expectedTags[lang.Tag]; ok {
			expectedTags[lang.Tag] = true
		}
	}

	for tag, found := range expectedTags {
		if !found {
			t.Errorf("expected language tag %q not found in response", tag)
		}
	}
}
