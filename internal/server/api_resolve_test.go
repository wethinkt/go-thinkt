package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func newResolveTestServer(stores ...thinkt.Store) *HTTPServer {
	registry := thinkt.NewRegistry()
	for _, s := range stores {
		registry.Register(s)
	}
	return NewHTTPServer(registry, DefaultConfig())
}

func TestHandleResolveSession_ValidPath(t *testing.T) {
	sessionPath := "/Users/test/project/session.jsonl"
	projectID := "/Users/test/project"

	store := &testStore{
		source: thinkt.SourceClaude,
		projects: []thinkt.Project{
			{
				ID:         projectID,
				Name:       "project",
				Path:       projectID,
				Source:     thinkt.SourceClaude,
				PathExists: true,
			},
		},
		sessions: map[string][]thinkt.SessionMeta{
			projectID: {
				{
					ID:          "sess-123",
					FullPath:    sessionPath,
					ProjectPath: projectID,
					Source:      thinkt.SourceClaude,
					WorkspaceID: "ws-1",
					ModifiedAt:  time.Now(),
				},
			},
		},
	}

	srv := newResolveTestServer(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/resolve?path="+url.QueryEscape(sessionPath), nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SessionResolveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.ProjectID != projectID {
		t.Errorf("ProjectID = %q, want %q", resp.ProjectID, projectID)
	}
	if resp.ProjectName != "project" {
		t.Errorf("ProjectName = %q, want %q", resp.ProjectName, "project")
	}
	if resp.ProjectSource != thinkt.SourceClaude {
		t.Errorf("ProjectSource = %q, want %q", resp.ProjectSource, thinkt.SourceClaude)
	}
	if resp.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want %q", resp.SessionID, "sess-123")
	}
	if resp.SessionPath != sessionPath {
		t.Errorf("SessionPath = %q, want %q", resp.SessionPath, sessionPath)
	}
	if resp.WorkspaceID != "ws-1" {
		t.Errorf("WorkspaceID = %q, want %q", resp.WorkspaceID, "ws-1")
	}
}

func TestHandleResolveSession_MissingPath(t *testing.T) {
	srv := newResolveTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/resolve", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Error != "missing_path" {
		t.Errorf("Error = %q, want %q", resp.Error, "missing_path")
	}
}

func TestHandleResolveSession_NotFound(t *testing.T) {
	srv := newResolveTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/resolve?path="+url.QueryEscape("/nonexistent/session.jsonl"), nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Error != "session_not_found" {
		t.Errorf("Error = %q, want %q", resp.Error, "session_not_found")
	}
}

func TestHandleResolveSession_CrossSource(t *testing.T) {
	sessionPath := "/Users/test/codex-project/session.jsonl"
	projectID := "/Users/test/codex-project"

	codexStore := &testStore{
		source: thinkt.SourceCodex,
		projects: []thinkt.Project{
			{ID: projectID, Name: "codex-project", Path: projectID, Source: thinkt.SourceCodex, PathExists: true},
		},
		sessions: map[string][]thinkt.SessionMeta{
			projectID: {
				{ID: "codex-sess", FullPath: sessionPath, ProjectPath: projectID, Source: thinkt.SourceCodex},
			},
		},
	}

	// Claude store has no matching session
	claudeStore := &testStore{
		source:   thinkt.SourceClaude,
		projects: []thinkt.Project{},
		sessions: map[string][]thinkt.SessionMeta{},
	}

	srv := newResolveTestServer(claudeStore, codexStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/resolve?path="+url.QueryEscape(sessionPath), nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SessionResolveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.ProjectSource != thinkt.SourceCodex {
		t.Errorf("ProjectSource = %q, want %q", resp.ProjectSource, thinkt.SourceCodex)
	}
	if resp.SessionID != "codex-sess" {
		t.Errorf("SessionID = %q, want %q", resp.SessionID, "codex-sess")
	}
	if resp.ProjectName != "codex-project" {
		t.Errorf("ProjectName = %q, want %q", resp.ProjectName, "codex-project")
	}
}

func TestHandleResolveSession_FallbackProjectID(t *testing.T) {
	// When GetProject returns nil, ProjectID should fall back to ProjectPath
	sessionPath := "/Users/test/proj/session.jsonl"
	projectPath := "/Users/test/proj"

	store := &testStore{
		source:   thinkt.SourceQwen,
		projects: []thinkt.Project{}, // no projects registered
		sessions: map[string][]thinkt.SessionMeta{
			projectPath: {
				{ID: "qwen-sess", FullPath: sessionPath, ProjectPath: projectPath, Source: thinkt.SourceQwen},
			},
		},
	}

	srv := newResolveTestServer(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/resolve?path="+url.QueryEscape(sessionPath), nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SessionResolveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	// ProjectID should be the raw ProjectPath since no Project matched
	if resp.ProjectID != projectPath {
		t.Errorf("ProjectID = %q, want %q", resp.ProjectID, projectPath)
	}
	if resp.ProjectName != "" {
		t.Errorf("ProjectName = %q, want empty", resp.ProjectName)
	}
}

func TestNormalizeSessionPathInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unix absolute path",
			input: "/tmp/session.jsonl",
			want:  "/tmp/session.jsonl",
		},
		{
			name:  "relative path gets rooted",
			input: "tmp/session.jsonl",
			want:  "/tmp/session.jsonl",
		},
		{
			name:  "windows drive path preserved",
			input: `C:\Users\test\session.jsonl`,
			want:  `C:\Users\test\session.jsonl`,
		},
		{
			name:  "windows UNC path preserved",
			input: `\\server\share\session.jsonl`,
			want:  `\\server\share\session.jsonl`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeSessionPathInput(tt.input); got != tt.want {
				t.Fatalf("normalizeSessionPathInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleResolveSession_WindowsPath(t *testing.T) {
	sessionPath := `C:\Users\test\project\session.jsonl`
	projectID := `C:\Users\test\project`

	store := &testStore{
		source: thinkt.SourceCodex,
		projects: []thinkt.Project{
			{
				ID:         projectID,
				Name:       "project",
				Path:       projectID,
				Source:     thinkt.SourceCodex,
				PathExists: true,
			},
		},
		sessions: map[string][]thinkt.SessionMeta{
			projectID: {
				{
					ID:          "sess-win",
					FullPath:    sessionPath,
					ProjectPath: projectID,
					Source:      thinkt.SourceCodex,
				},
			},
		},
	}

	srv := newResolveTestServer(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/resolve?path="+url.QueryEscape(sessionPath), nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SessionResolveResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.SessionPath != sessionPath {
		t.Fatalf("SessionPath = %q, want %q", resp.SessionPath, sessionPath)
	}
	if resp.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", resp.ProjectID, projectID)
	}
}

func TestHandleGetSession_NotFound(t *testing.T) {
	srv := newResolveTestServer(&testStore{
		source:   thinkt.SourceClaude,
		projects: nil,
		sessions: nil,
	})

	escaped := url.PathEscape("/nonexistent/session.jsonl")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+escaped, nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Error != "session_not_found" {
		t.Errorf("Error = %q, want %q", resp.Error, "session_not_found")
	}
}
