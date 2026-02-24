package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindBinaryNearExecutable_PrefersSiblingBinary(t *testing.T) {
	dir := t.TempDir()
	sibling := filepath.Join(dir, indexerBinaryName)
	if err := os.WriteFile(sibling, []byte(""), 0644); err != nil {
		t.Fatalf("write sibling binary: %v", err)
	}

	got := findBinaryNearExecutable(
		indexerBinaryName,
		"linux",
		func() (string, error) { return filepath.Join(dir, "thinkt"), nil },
		os.Stat,
		func(string) (string, error) { return "/usr/local/bin/thinkt-indexer", nil },
	)

	if got != sibling {
		t.Fatalf("expected sibling %q, got %q", sibling, got)
	}
}

func TestFindBinaryNearExecutable_FallsBackToPATH(t *testing.T) {
	dir := t.TempDir()
	pathResult := "/usr/local/bin/thinkt-indexer"

	got := findBinaryNearExecutable(
		indexerBinaryName,
		"linux",
		func() (string, error) { return filepath.Join(dir, "thinkt"), nil },
		os.Stat,
		func(string) (string, error) { return pathResult, nil },
	)

	if got != pathResult {
		t.Fatalf("expected PATH result %q, got %q", pathResult, got)
	}
}

func TestFindBinaryNearExecutable_ReturnsEmptyWhenNotFound(t *testing.T) {
	dir := t.TempDir()

	got := findBinaryNearExecutable(
		indexerBinaryName,
		"linux",
		func() (string, error) { return filepath.Join(dir, "thinkt"), nil },
		os.Stat,
		func(string) (string, error) { return "", errors.New("not found") },
	)

	if got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestFindBinaryNearExecutable_UsesPATHWhenExecutableLookupFails(t *testing.T) {
	pathResult := "/usr/local/bin/thinkt-indexer"

	got := findBinaryNearExecutable(
		indexerBinaryName,
		"linux",
		func() (string, error) { return "", errors.New("exec path unavailable") },
		os.Stat,
		func(string) (string, error) { return pathResult, nil },
	)

	if got != pathResult {
		t.Fatalf("expected PATH result %q, got %q", pathResult, got)
	}
}

func TestFindBinaryNearExecutable_WindowsPrefersSiblingExe(t *testing.T) {
	dir := t.TempDir()
	sibling := filepath.Join(dir, indexerBinaryName+".exe")
	if err := os.WriteFile(sibling, []byte(""), 0644); err != nil {
		t.Fatalf("write sibling binary: %v", err)
	}

	got := findBinaryNearExecutable(
		indexerBinaryName,
		"windows",
		func() (string, error) { return filepath.Join(dir, "thinkt.exe"), nil },
		os.Stat,
		func(string) (string, error) { return `C:\bin\thinkt-indexer.exe`, nil },
	)

	if got != sibling {
		t.Fatalf("expected sibling %q, got %q", sibling, got)
	}
}

func TestFindBinaryNearExecutable_WindowsFallsBackToExeInPATH(t *testing.T) {
	dir := t.TempDir()
	pathResult := `C:\bin\thinkt-indexer.exe`

	got := findBinaryNearExecutable(
		indexerBinaryName,
		"windows",
		func() (string, error) { return filepath.Join(dir, "thinkt.exe"), nil },
		os.Stat,
		func(name string) (string, error) {
			if name == indexerBinaryName {
				return "", errors.New("not found")
			}
			if name == indexerBinaryName+".exe" {
				return pathResult, nil
			}
			return "", errors.New("unexpected binary name")
		},
	)

	if got != pathResult {
		t.Fatalf("expected PATH result %q, got %q", pathResult, got)
	}
}
