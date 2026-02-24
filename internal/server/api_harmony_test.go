package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestAPI_GetProjects_ExcludesDeletedByDefault(t *testing.T) {
	registry := thinkt.NewRegistry()
	activePath := t.TempDir()
	deletedPath := filepath.Join(t.TempDir(), "deleted")

	registry.Register(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "active", Name: "active", Path: activePath, Source: thinkt.SourceClaude, LastModified: time.Now()},
			{ID: "deleted", Name: "deleted", Path: deletedPath, Source: thinkt.SourceClaude, LastModified: time.Now().Add(-time.Minute)},
		},
	})

	server := NewHTTPServer(registry, DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var out ProjectsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(out.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(out.Projects))
	}
	if out.Projects[0].ID != "active" {
		t.Fatalf("expected active project, got %s", out.Projects[0].ID)
	}
}

func TestAPI_GetSession_InvalidPaginationParams(t *testing.T) {
	registry := thinkt.NewRegistry()
	server := NewHTTPServer(registry, DefaultConfig())

	path := createTestClaudeSession(t, t.TempDir())
	escaped := url.PathEscape(path)

	tests := []struct {
		name      string
		query     string
		wantError string
	}{
		{name: "negative limit", query: "limit=-1", wantError: "invalid_limit"},
		{name: "negative offset", query: "offset=-1", wantError: "invalid_offset"},
		{name: "non-integer limit", query: "limit=abc", wantError: "invalid_limit"},
		{name: "non-integer offset", query: "offset=abc", wantError: "invalid_offset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+escaped+"?"+tt.query, nil)
			w := httptest.NewRecorder()
			server.router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", w.Code)
			}

			var out ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if out.Error != tt.wantError {
				t.Fatalf("expected error %q, got %q", tt.wantError, out.Error)
			}
		})
	}
}

func TestAPI_GetProjects_IncludeDeleted(t *testing.T) {
	registry := thinkt.NewRegistry()
	activePath := t.TempDir()
	deletedPath := filepath.Join(t.TempDir(), "deleted")

	registry.Register(&testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{ID: "active", Name: "active", Path: activePath, Source: thinkt.SourceClaude, LastModified: time.Now()},
			{ID: "deleted", Name: "deleted", Path: deletedPath, Source: thinkt.SourceClaude, LastModified: time.Now().Add(-time.Minute)},
		},
	})

	server := NewHTTPServer(registry, DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects?include_deleted=true", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var out ProjectsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(out.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(out.Projects))
	}
}

func TestAPI_GetSessionMetadata_SummaryOnlyPreviews(t *testing.T) {
	registry := thinkt.NewRegistry()
	server := NewHTTPServer(registry, DefaultConfig())

	path := createTestClaudeSession(t, t.TempDir())
	escaped := url.PathEscape(path)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+escaped+"/metadata?summary_only=true&limit=1", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var out SessionMetadataResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if out.Meta.Path != path {
		t.Fatalf("expected meta path %s, got %s", path, out.Meta.Path)
	}
	if out.Returned != 1 || len(out.EntrySummary) != 1 {
		t.Fatalf("expected one preview, returned=%d len=%d", out.Returned, len(out.EntrySummary))
	}
	if out.EntrySummary[0].Role != "user" {
		t.Fatalf("expected user preview, got %s", out.EntrySummary[0].Role)
	}
}
