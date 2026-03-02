package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestMetricsEndpoint_ExposedWithoutAuth(t *testing.T) {
	registry := thinkt.NewRegistry()
	config := DefaultConfig()
	server := NewHTTPServerWithAuth(registry, config, AuthConfig{
		Mode:  AuthModeToken,
		Token: "secret-token",
		Realm: "thinkt-api",
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected prometheus text content type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "thinkt_api_http_requests_total") {
		t.Fatalf("expected api metrics in output, got body prefix: %q", truncateForTest(body, 120))
	}
}

func truncateForTest(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func TestPrefixPrometheusMetricNames(t *testing.T) {
	in := strings.Join([]string{
		"# HELP go_goroutines Number of goroutines.",
		"# TYPE go_goroutines gauge",
		"go_goroutines 12",
		"process_cpu_seconds_total 1.5",
		"http_requests_total{method=\"GET\"} 3",
		"",
	}, "\n")

	out := prefixPrometheusMetricNames(in, "thinkt_indexer_")

	if !strings.Contains(out, "# HELP thinkt_indexer_go_goroutines Number of goroutines.") {
		t.Fatalf("missing prefixed HELP line: %q", out)
	}
	if !strings.Contains(out, "# TYPE thinkt_indexer_go_goroutines gauge") {
		t.Fatalf("missing prefixed TYPE line: %q", out)
	}
	if !strings.Contains(out, "thinkt_indexer_go_goroutines 12") {
		t.Fatalf("missing prefixed sample line: %q", out)
	}
	if !strings.Contains(out, "thinkt_indexer_http_requests_total{method=\"GET\"} 3") {
		t.Fatalf("missing prefixed labeled sample line: %q", out)
	}
}
