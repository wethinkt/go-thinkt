package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

type docStore struct {
	thinkt.Store
	docs []thinkt.AgentDocument
}

func (d docStore) SessionDocuments(context.Context, string) ([]thinkt.AgentDocument, error) {
	return d.docs, nil
}

func getDocuments(t *testing.T, store thinkt.Store, path string) DocumentsResponse {
	t.Helper()
	registry := thinkt.NewRegistry()
	registry.Register(store)
	server := NewHTTPServer(registry, DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+url.PathEscape(path)+"/documents", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out DocumentsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAPI_GetSessionDocuments(t *testing.T) {
	path := createTestClaudeSession(t, t.TempDir())
	docs := []thinkt.AgentDocument{{Name: "plan.md", Kind: thinkt.DocPlan, Final: "x"}}
	out := getDocuments(t, docStore{newSessionFixtureStore(path), docs}, path)
	if len(out.Documents) != 1 || out.Documents[0].Name != "plan.md" {
		t.Fatalf("%+v", out)
	}
}

func TestAPI_GetSessionDocuments_StoreWithoutDocuments(t *testing.T) {
	path := createTestClaudeSession(t, t.TempDir())
	if out := getDocuments(t, newSessionFixtureStore(path), path); out.Documents == nil || len(out.Documents) != 0 {
		t.Fatalf("%+v", out)
	}
}
