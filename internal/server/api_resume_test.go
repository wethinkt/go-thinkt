package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func newNoAuthTestServer() *HTTPServer {
	return NewHTTPServerWithAuth(
		thinkt.NewRegistry(),
		DefaultConfig(),
		AuthConfig{Mode: AuthModeNone},
	)
}

func TestHandleResumeSession_GetExecActionRejected(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/foo/resume?action=exec", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
	if got := w.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("expected Allow header %q, got %q", http.MethodPost, got)
	}
}

func TestHandleResumeSessionExec_PostCrossOriginRejected(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/foo/resume", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestHandleResumeSessionExec_PostSameOriginAllowed(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/foo/resume", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// No test data is loaded, so same-origin requests should reach the resolver and return 404.
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleResumeSessionExec_PostResumePrefixRoute(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/resume/foo", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestHandleResumeSessionExec_PostNoOriginHeadersAllowed(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/foo/resume", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	// Non-browser clients without Origin/Referer are allowed through CSRF gate.
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleResumeSessionExec_PostRefererCrossOriginRejected(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/foo/resume", nil)
	req.Header.Set("Referer", "https://evil.example/path")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestHandleResumeSessionExec_PostRefererSameOriginAllowed(t *testing.T) {
	server := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/foo/resume", nil)
	req.Header.Set("Referer", "http://example.com/app")
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestResumeSession_NonexistentPathReturnsSessionNotFound(t *testing.T) {
	srv := newNoAuthTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/nonexistent/path/resume", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "session_not_found" {
		t.Fatalf("expected error code %q, got %q", "session_not_found", resp.Error)
	}
}

func TestResumeSession_UnsupportedSourceReturnsResumeNotSupported(t *testing.T) {
	reg := thinkt.NewRegistry()
	// Register a store that does NOT implement SessionResumer.
	reg.Register(&fakeStoreNoResume{
		source: "fake",
		metas: map[string]*thinkt.SessionMeta{
			"/tmp/fake-session.jsonl": {
				Source:   "fake",
				FullPath: "/tmp/fake-session.jsonl",
			},
		},
	})

	srv := NewHTTPServerWithAuth(reg, DefaultConfig(), AuthConfig{Mode: AuthModeNone})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/%2Ftmp%2Ffake-session.jsonl/resume", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "resume_not_supported" {
		t.Fatalf("expected error code %q, got %q", "resume_not_supported", resp.Error)
	}
}

// fakeStoreNoResume implements thinkt.Store but NOT thinkt.SessionResumer,
// so GetResumer returns false for its source.
type fakeStoreNoResume struct {
	source thinkt.Source
	metas  map[string]*thinkt.SessionMeta
}

func (f *fakeStoreNoResume) Source() thinkt.Source         { return f.source }
func (f *fakeStoreNoResume) Workspace() thinkt.Workspace   { return thinkt.Workspace{} }
func (f *fakeStoreNoResume) ListProjects(context.Context) ([]thinkt.Project, error) {
	return nil, nil
}
func (f *fakeStoreNoResume) GetProject(context.Context, string) (*thinkt.Project, error) {
	return nil, nil
}
func (f *fakeStoreNoResume) ListSessions(context.Context, string) ([]thinkt.SessionMeta, error) {
	return nil, nil
}
func (f *fakeStoreNoResume) GetSessionMeta(_ context.Context, id string) (*thinkt.SessionMeta, error) {
	m := f.metas[id]
	if m == nil {
		return nil, nil
	}
	cp := *m
	return &cp, nil
}
func (f *fakeStoreNoResume) LoadSession(context.Context, string) (*thinkt.Session, error) {
	return nil, nil
}
func (f *fakeStoreNoResume) OpenSession(context.Context, string) (thinkt.SessionReader, error) {
	return nil, nil
}
func (f *fakeStoreNoResume) WatchConfig() thinkt.WatchConfig { return thinkt.DefaultWatchConfig() }
