package thinkt

import (
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
