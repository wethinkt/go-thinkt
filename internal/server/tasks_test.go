package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

func TestAPI_GetSessionTasks(t *testing.T) {
	path := createTestClaudeSession(t, t.TempDir())
	registry := thinkt.NewRegistry()
	registry.Register(newSessionFixtureStore(path))
	server := NewHTTPServer(registry, DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+url.PathEscape(path)+"/tasks", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out TasksResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Tasks == nil {
		t.Fatalf("%s", w.Body.String())
	}
}

func TestAPI_GetSessionTasks_UnknownSession(t *testing.T) {
	server := NewHTTPServer(thinkt.NewRegistry(), DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+url.PathEscape("/no/such/s.jsonl")+"/tasks", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d", w.Code)
	}
}
