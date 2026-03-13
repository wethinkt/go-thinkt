package share

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sessions" {
			t.Errorf("path = %q, want /api/sessions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing or wrong auth header")
		}
		_ = json.NewEncoder(w).Encode(SessionList{
			Sessions: []Session{{Slug: "abc-123", Title: "Test"}},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Slug != "abc-123" {
		t.Errorf("unexpected sessions: %+v", sessions)
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
		_ = json.NewEncoder(w).Encode(ExploreResponse{
			Sessions: []Session{{Slug: "pub-1", Title: "Public Session"}},
			Page:     1,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	resp, err := c.Explore("newest", "go", 1)
	if err != nil {
		t.Fatalf("Explore: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(resp.Sessions))
	}
}

func TestClient_GetProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/profile" {
			t.Errorf("path = %q, want /api/profile", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Profile{
			User:  ProfileUser{Name: "testuser", Email: "test@test.com"},
			Stats: ProfileStats{TotalSessions: 5},
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

func TestClient_DeleteSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/sessions/abc-123" {
			t.Errorf("path = %q, want /api/sessions/abc-123", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	err := c.DeleteSession("abc-123")
	if err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
}

func TestClient_ExploreTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/explore/tags" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(TagCloud{
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

func TestClient_GetSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sessions/slug-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Session{Slug: "slug-1", Title: "My Session"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	session, err := c.GetSession("slug-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Title != "My Session" {
		t.Errorf("title = %q", session.Title)
	}
}
