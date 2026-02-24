package thinkt

import (
	"errors"
	"testing"
	"time"
)

func TestStoreCacheNoTTL(t *testing.T) {
	var c StoreCache

	// Without TTL, cache never expires
	c.SetProjects([]Project{{Path: "/a"}}, nil)
	projects, err, ok := c.GetProjects()
	if !ok {
		t.Fatal("expected cached")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 || projects[0].Path != "/a" {
		t.Errorf("unexpected projects: %v", projects)
	}
}

func TestStoreCacheTTLExpiry(t *testing.T) {
	var c StoreCache
	c.SetTTL(10 * time.Millisecond)

	c.SetProjects([]Project{{Path: "/a"}}, nil)

	// Immediately should be cached
	_, _, ok := c.GetProjects()
	if !ok {
		t.Fatal("expected cached immediately after set")
	}

	// Wait for TTL to expire
	time.Sleep(15 * time.Millisecond)

	_, _, ok = c.GetProjects()
	if ok {
		t.Fatal("expected cache miss after TTL expiry")
	}

	// Re-set should work
	c.SetProjects([]Project{{Path: "/b"}}, nil)
	projects, _, ok := c.GetProjects()
	if !ok {
		t.Fatal("expected cached after re-set")
	}
	if len(projects) != 1 || projects[0].Path != "/b" {
		t.Errorf("unexpected projects after re-set: %v", projects)
	}
}

func TestStoreCacheSessionTTL(t *testing.T) {
	var c StoreCache
	c.SetTTL(10 * time.Millisecond)

	c.SetSessions("proj1", []SessionMeta{{ID: "s1"}}, nil)

	// Immediately should be cached
	_, _, ok := c.GetSessions("proj1")
	if !ok {
		t.Fatal("expected session cached")
	}

	time.Sleep(15 * time.Millisecond)

	_, _, ok = c.GetSessions("proj1")
	if ok {
		t.Fatal("expected session cache miss after TTL expiry")
	}
}

func TestStoreCacheProjectsDefensiveCopies(t *testing.T) {
	var c StoreCache

	input := []Project{{Path: "/a"}}
	c.SetProjects(input, nil)

	// Mutating input after SetProjects must not affect cache.
	input[0].Path = "/mutated-input"

	projects, _, ok := c.GetProjects()
	if !ok {
		t.Fatal("expected cached projects")
	}
	if projects[0].Path != "/a" {
		t.Fatalf("cache should keep original value, got %q", projects[0].Path)
	}

	// Mutating returned slice must not affect cache.
	projects[0].Path = "/mutated-output"
	projects2, _, ok := c.GetProjects()
	if !ok {
		t.Fatal("expected cached projects")
	}
	if projects2[0].Path != "/a" {
		t.Fatalf("cache should return immutable snapshot, got %q", projects2[0].Path)
	}
}

func TestStoreCacheSessionsDefensiveCopies(t *testing.T) {
	var c StoreCache

	input := []SessionMeta{{ID: "s1"}}
	c.SetSessions("proj1", input, nil)

	// Mutating input after SetSessions must not affect cache.
	input[0].ID = "mutated-input"

	sessions, _, ok := c.GetSessions("proj1")
	if !ok {
		t.Fatal("expected cached sessions")
	}
	if sessions[0].ID != "s1" {
		t.Fatalf("cache should keep original value, got %q", sessions[0].ID)
	}

	// Mutating returned slice must not affect cache.
	sessions[0].ID = "mutated-output"
	sessions2, _, ok := c.GetSessions("proj1")
	if !ok {
		t.Fatal("expected cached sessions")
	}
	if sessions2[0].ID != "s1" {
		t.Fatalf("cache should return immutable snapshot, got %q", sessions2[0].ID)
	}
}

func TestStoreCacheInvalidationMethods(t *testing.T) {
	var c StoreCache
	c.SetProjects([]Project{{Path: "/a"}}, nil)
	c.SetSessions("proj1", []SessionMeta{{ID: "s1"}}, nil)
	c.SetSessions("proj2", []SessionMeta{{ID: "s2"}}, nil)

	c.InvalidateProjects()
	if _, _, ok := c.GetProjects(); ok {
		t.Fatal("expected project cache miss after invalidation")
	}
	if _, _, ok := c.GetSessions("proj1"); !ok {
		t.Fatal("expected sessions cache to remain after project invalidation")
	}

	c.InvalidateSessions("proj1")
	if _, _, ok := c.GetSessions("proj1"); ok {
		t.Fatal("expected proj1 sessions cache miss after invalidation")
	}
	if _, _, ok := c.GetSessions("proj2"); !ok {
		t.Fatal("expected other project sessions cache to remain")
	}

	c.Clear()
	if _, _, ok := c.GetSessions("proj2"); ok {
		t.Fatal("expected all sessions cache miss after Clear")
	}
}

func TestStoreCacheProjectsErrorNotCached(t *testing.T) {
	var c StoreCache

	c.SetProjects(nil, errors.New("temporary failure"))
	projects, err, ok := c.GetProjects()
	if ok {
		t.Fatal("expected project cache miss after error set")
	}
	if err != nil {
		t.Fatalf("expected no cached error, got %v", err)
	}
	if projects != nil {
		t.Fatalf("expected nil projects on miss, got %v", projects)
	}
}

func TestStoreCacheSessionsErrorNotCached(t *testing.T) {
	var c StoreCache

	c.SetSessions("proj1", nil, errors.New("temporary failure"))
	sessions, err, ok := c.GetSessions("proj1")
	if ok {
		t.Fatal("expected session cache miss after error set")
	}
	if err != nil {
		t.Fatalf("expected no cached error, got %v", err)
	}
	if sessions != nil {
		t.Fatalf("expected nil sessions on miss, got %v", sessions)
	}
}
