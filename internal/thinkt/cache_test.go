package thinkt

import (
	"errors"
	"sync"
	"sync/atomic"
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

func TestStoreCacheLoadProjectsDedupesConcurrentMisses(t *testing.T) {
	var c StoreCache
	var loaderCalls atomic.Int32

	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			projects, err := c.LoadProjects(func() ([]Project, error) {
				loaderCalls.Add(1)
				time.Sleep(20 * time.Millisecond)
				return []Project{{Path: "/dedup"}}, nil
			})
			if err == nil && (len(projects) != 1 || projects[0].Path != "/dedup") {
				err = errors.New("unexpected projects result")
			}
			errs <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected load error: %v", err)
		}
	}

	if got := loaderCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 loader call, got %d", got)
	}
}

func TestStoreCacheLoadSessionsDedupesPerProjectID(t *testing.T) {
	var c StoreCache
	var loaderCalls atomic.Int32

	const workers = 12
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sessions, err := c.LoadSessions("proj1", func() ([]SessionMeta, error) {
				loaderCalls.Add(1)
				time.Sleep(20 * time.Millisecond)
				return []SessionMeta{{ID: "s1"}}, nil
			})
			if err == nil && (len(sessions) != 1 || sessions[0].ID != "s1") {
				err = errors.New("unexpected sessions result")
			}
			errs <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected load error: %v", err)
		}
	}

	if got := loaderCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 loader call, got %d", got)
	}
}

func TestStoreCacheLoadSessionsDifferentProjectIDsDoNotBlockEachOther(t *testing.T) {
	var c StoreCache
	var p1Calls atomic.Int32
	var p2Calls atomic.Int32

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = c.LoadSessions("proj1", func() ([]SessionMeta, error) {
			p1Calls.Add(1)
			time.Sleep(20 * time.Millisecond)
			return []SessionMeta{{ID: "s1"}}, nil
		})
	}()
	go func() {
		defer wg.Done()
		_, _ = c.LoadSessions("proj2", func() ([]SessionMeta, error) {
			p2Calls.Add(1)
			time.Sleep(20 * time.Millisecond)
			return []SessionMeta{{ID: "s2"}}, nil
		})
	}()

	wg.Wait()

	if got := p1Calls.Load(); got != 1 {
		t.Fatalf("expected proj1 loader to run once, got %d", got)
	}
	if got := p2Calls.Load(); got != 1 {
		t.Fatalf("expected proj2 loader to run once, got %d", got)
	}
}

func TestStoreCacheLoadProjectsErrorRetriesOnNextCall(t *testing.T) {
	var c StoreCache
	var loaderCalls atomic.Int32

	loader := func() ([]Project, error) {
		call := loaderCalls.Add(1)
		if call == 1 {
			return nil, errors.New("temporary failure")
		}
		return []Project{{Path: "/ok"}}, nil
	}

	if _, err := c.LoadProjects(loader); err == nil {
		t.Fatal("expected first load to fail")
	}
	projects, err := c.LoadProjects(loader)
	if err != nil {
		t.Fatalf("expected second load to succeed, got %v", err)
	}
	if len(projects) != 1 || projects[0].Path != "/ok" {
		t.Fatalf("unexpected projects result: %+v", projects)
	}
	if got := loaderCalls.Load(); got != 2 {
		t.Fatalf("expected two loader calls, got %d", got)
	}
}
