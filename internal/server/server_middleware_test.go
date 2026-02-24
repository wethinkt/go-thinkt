package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRedactRequestForLogging(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost:8784/api/v1/sources?token=abc123&access_token=def456&q=hello", nil)

	redacted := redactRequestForLogging(req)
	if redacted == req {
		t.Fatal("redactRequestForLogging() should clone request when sensitive params are present")
	}

	query := redacted.URL.Query()
	if got := query.Get("token"); got != "[REDACTED]" {
		t.Fatalf("token query = %q, want [REDACTED]", got)
	}
	if got := query.Get("access_token"); got != "[REDACTED]" {
		t.Fatalf("access_token query = %q, want [REDACTED]", got)
	}
	if got := query.Get("q"); got != "hello" {
		t.Fatalf("q query = %q, want hello", got)
	}

	if strings.Contains(redacted.RequestURI, "abc123") || strings.Contains(redacted.RequestURI, "def456") {
		t.Fatalf("RequestURI should not include raw secret values: %s", redacted.RequestURI)
	}
}

func TestRedactRequestForLogging_NoSensitiveQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost:8784/api/v1/sources?q=hello", nil)

	redacted := redactRequestForLogging(req)
	if redacted != req {
		t.Fatal("redactRequestForLogging() should return the original request when no sensitive params are present")
	}
}

func TestSanitizeQueryForRedirect(t *testing.T) {
	got := sanitizeQueryForRedirect("token=abc123&foo=bar&access_token=def456")
	values, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}

	if values.Get("foo") != "bar" {
		t.Fatalf("foo query = %q, want bar", values.Get("foo"))
	}
	if values.Get("token") != "" {
		t.Fatalf("token query = %q, want empty", values.Get("token"))
	}
	if values.Get("access_token") != "" {
		t.Fatalf("access_token query = %q, want empty", values.Get("access_token"))
	}
}

func TestCORSMiddleware_Disabled(t *testing.T) {
	wrapped := corsMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest("GET", "/api/v1/sources", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

func TestCORSMiddleware_Enabled(t *testing.T) {
	nextCalled := false
	wrapped := corsMiddleware("https://example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	}))

	// Simple request should pass through to the handler.
	req := httptest.NewRequest("GET", "/api/v1/sources", nil)
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called for non-OPTIONS request")
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("Access-Control-Allow-Headers = %q, want %q", got, "Authorization, Content-Type")
	}

	// OPTIONS should short-circuit before calling next.
	nextCalled = false
	reqOptions := httptest.NewRequest("OPTIONS", "/api/v1/sources", nil)
	rrOptions := httptest.NewRecorder()
	wrapped.ServeHTTP(rrOptions, reqOptions)
	if nextCalled {
		t.Fatal("did not expect next handler to be called for OPTIONS request")
	}
	if rrOptions.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rrOptions.Code, http.StatusOK)
	}
}
