package server

import (
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
