package share

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListTraces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces" {
			t.Errorf("path = %q, want /api/traces", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong auth header")
		}
		json.NewEncoder(w).Encode(TraceList{
			Traces: []Trace{{Slug: "abc-123", Title: "Test"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	traces, err := c.ListTraces()
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	if len(traces) != 1 || traces[0].Slug != "abc-123" {
		t.Errorf("unexpected traces: %+v", traces)
	}
}

func TestClient_Explore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/explore" {
			t.Errorf("path = %q, want /api/explore", r.URL.Path)
		}
		if r.URL.Query().Get("sort") != "newest" {
			t.Errorf("sort = %q, want newest", r.URL.Query().Get("sort"))
		}
		if r.URL.Query().Get("tag") != "go" {
			t.Errorf("tag = %q, want go", r.URL.Query().Get("tag"))
		}
		json.NewEncoder(w).Encode(ExploreResponse{
			Traces: []Trace{{Slug: "pub-1", Title: "Public Trace"}},
			Page:   1,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	resp, err := c.Explore("newest", "go", 1)
	if err != nil {
		t.Fatalf("Explore: %v", err)
	}
	if len(resp.Traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(resp.Traces))
	}
}

func TestClient_GetProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/profile" {
			t.Errorf("path = %q, want /api/profile", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Profile{
			User:  ProfileUser{Name: "testuser", Email: "test@test.com"},
			Stats: ProfileStats{TotalTraces: 5},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	profile, err := c.GetProfile()
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile.User.Name != "testuser" {
		t.Errorf("name = %q, want testuser", profile.User.Name)
	}
}

func TestClient_DeleteTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/traces/abc-123" {
			t.Errorf("path = %q, want /api/traces/abc-123", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	err := c.DeleteTrace("abc-123")
	if err != nil {
		t.Fatalf("DeleteTrace: %v", err)
	}
}

func TestClient_ExploreTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/explore/tags" {
			t.Errorf("path = %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(TagCloud{
			Tags: []TagCount{{Tag: "go", Count: 10}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	tags, err := c.ExploreTags()
	if err != nil {
		t.Fatalf("ExploreTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Tag != "go" {
		t.Errorf("unexpected tags: %+v", tags)
	}
}

func TestClient_GetTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces/slug-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Trace{Slug: "slug-1", Title: "My Trace"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	trace, err := c.GetTrace("slug-1")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if trace.Title != "My Trace" {
		t.Errorf("title = %q", trace.Title)
	}
}
