package rpc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

// mockHandler implements Handler for testing.
type mockHandler struct {
	syncCalled           bool
	searchCalled         bool
	semanticSearchCalled bool
	statsCalled          bool
	statusCalled         bool

	lastSearchParams         SearchParams
	lastSemanticSearchParams SemanticSearchParams
}

func (m *mockHandler) HandleSync(_ context.Context, params SyncParams, send func(Progress)) (*Response, error) {
	m.syncCalled = true
	// Send a couple of progress updates.
	send(Progress{Data: json.RawMessage(`{"done":1,"total":3}`)})
	send(Progress{Data: json.RawMessage(`{"done":2,"total":3}`)})
	return &Response{OK: true, Data: json.RawMessage(`{"synced":3}`)}, nil
}

func (m *mockHandler) HandleSearch(_ context.Context, params SearchParams) (*Response, error) {
	m.searchCalled = true
	m.lastSearchParams = params
	return &Response{OK: true, Data: json.RawMessage(`{"results":[]}`)}, nil
}

func (m *mockHandler) HandleSemanticSearch(_ context.Context, params SemanticSearchParams) (*Response, error) {
	m.semanticSearchCalled = true
	m.lastSemanticSearchParams = params
	return &Response{OK: true, Data: json.RawMessage(`{"results":[]}`)}, nil
}

func (m *mockHandler) HandleStats(_ context.Context) (*Response, error) {
	m.statsCalled = true
	return &Response{OK: true, Data: json.RawMessage(`{"sessions":42}`)}, nil
}

func (m *mockHandler) HandleConfigReload(_ context.Context) (*Response, error) {
	return &Response{OK: true, Data: json.RawMessage(`{"embedding_enabled":false}`)}, nil
}

func (m *mockHandler) HandleStatus(_ context.Context) (*Response, error) {
	m.statusCalled = true
	data, _ := json.Marshal(StatusData{
		State:         "idle",
		Model:         "test-model",
		ModelDim:      384,
		UptimeSeconds: 120,
		Watching:      true,
	})
	return &Response{OK: true, Data: data}, nil
}

func startTestServer(t *testing.T) (string, *mockHandler) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "test.sock")
	h := &mockHandler{}
	srv := NewServer(sock, h)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(srv.Stop)
	return sock, h
}

func TestStats(t *testing.T) {
	sock, h := startTestServer(t)

	resp, err := CallAt(sock, "stats", nil, nil)
	if err != nil {
		t.Fatalf("call stats: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response, got error: %s", resp.Error)
	}
	if !h.statsCalled {
		t.Fatal("expected stats handler to be called")
	}
	if string(resp.Data) != `{"sessions":42}` {
		t.Fatalf("unexpected data: %s", resp.Data)
	}
}

func TestSearch(t *testing.T) {
	sock, h := startTestServer(t)

	params := SearchParams{
		Query:   "hello world",
		Project: "myproject",
		Limit:   10,
	}
	resp, err := CallAt(sock, "search", params, nil)
	if err != nil {
		t.Fatalf("call search: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response, got error: %s", resp.Error)
	}
	if !h.searchCalled {
		t.Fatal("expected search handler to be called")
	}
	if h.lastSearchParams.Query != "hello world" {
		t.Fatalf("expected query 'hello world', got %q", h.lastSearchParams.Query)
	}
	if h.lastSearchParams.Project != "myproject" {
		t.Fatalf("expected project 'myproject', got %q", h.lastSearchParams.Project)
	}
}

func TestUnknownMethod(t *testing.T) {
	sock, _ := startTestServer(t)

	resp, err := CallAt(sock, "nonexistent", nil, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.OK {
		t.Fatal("expected error response for unknown method")
	}
	if resp.Error == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestSyncWithProgress(t *testing.T) {
	sock, h := startTestServer(t)

	var progressMessages []Progress
	resp, err := CallAt(sock, "sync", SyncParams{Force: true}, func(p Progress) {
		progressMessages = append(progressMessages, p)
	})
	if err != nil {
		t.Fatalf("call sync: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response, got error: %s", resp.Error)
	}
	if !h.syncCalled {
		t.Fatal("expected sync handler to be called")
	}
	if len(progressMessages) != 2 {
		t.Fatalf("expected 2 progress messages, got %d", len(progressMessages))
	}
}

func TestServerAvailable(t *testing.T) {
	sock, _ := startTestServer(t)

	if !ServerAvailableAt(sock) {
		t.Fatal("expected server to be available")
	}

	// Check a non-existent socket.
	if ServerAvailableAt("/tmp/nonexistent-test-sock-12345.sock") {
		t.Fatal("expected server to be unavailable for non-existent socket")
	}
}

func TestStatus(t *testing.T) {
	sock, h := startTestServer(t)

	resp, err := CallAt(sock, "status", nil, nil)
	if err != nil {
		t.Fatalf("call status: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response, got error: %s", resp.Error)
	}
	if !h.statusCalled {
		t.Fatal("expected status handler to be called")
	}

	var status StatusData
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if status.State != "idle" {
		t.Fatalf("expected state 'idle', got %q", status.State)
	}
	if status.Model != "test-model" {
		t.Fatalf("expected model 'test-model', got %q", status.Model)
	}
}

func TestSemanticSearch(t *testing.T) {
	sock, h := startTestServer(t)

	params := SemanticSearchParams{
		Query:      "find similar",
		Limit:      5,
		MaxDistance: 0.8,
	}
	resp, err := CallAt(sock, "semantic_search", params, nil)
	if err != nil {
		t.Fatalf("call semantic_search: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response, got error: %s", resp.Error)
	}
	if !h.semanticSearchCalled {
		t.Fatal("expected semantic_search handler to be called")
	}
	if h.lastSemanticSearchParams.Query != "find similar" {
		t.Fatalf("expected query 'find similar', got %q", h.lastSemanticSearchParams.Query)
	}
}
