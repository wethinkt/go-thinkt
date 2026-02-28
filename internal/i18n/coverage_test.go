package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestTranslationCoverage scans Go source files for message IDs used in
// T/Tf/Tn calls and checks each non-English locale file for missing
// translations. It logs coverage percentages but does not fail, since
// partial translations are expected.
func TestTranslationCoverage(t *testing.T) {
	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("finding project root: %v", err)
	}

	// 1. Collect all message IDs from source.
	ids := collectMessageIDs(t, root)
	if len(ids) == 0 {
		t.Fatal("found 0 message IDs in source — regex may be broken")
	}

	sortedIDs := sortedKeys(ids)
	t.Logf("Found %d message IDs in source", len(sortedIDs))

	// 2. Check each non-English locale file.
	localeDir := filepath.Join(root, "internal", "i18n", "locales")
	entries, err := os.ReadDir(localeDir)
	if err != nil {
		t.Fatalf("reading locales dir: %v", err)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") || name == "en.toml" {
			continue
		}

		translated := parseTomlKeys(t, filepath.Join(localeDir, name))

		var missing []string
		for _, id := range sortedIDs {
			if !translated[id] {
				missing = append(missing, id)
			}
		}

		covered := len(sortedIDs) - len(missing)
		pct := float64(covered) / float64(len(sortedIDs)) * 100
		t.Logf("%s: %d/%d translated (%.1f%%)", name, covered, len(sortedIDs), pct)

		if len(missing) > 0 {
			t.Logf("%s: %d missing keys:", name, len(missing))
			for _, k := range missing {
				t.Logf("  - %s", k)
			}
		}
	}
}

// messageIDPattern matches T("key"), Tf("key"), Tn("key") calls,
// with an optional thinktI18n. or i18n. package prefix.
// messageIDPattern matches T("key.id"), Tf("key.id"), Tn("key.id") calls,
// with an optional thinktI18n. or i18n. package prefix.
// Keys must have at least two dot-separated segments (e.g., "cmd.root.short")
// to exclude partial dynamic keys like T("cmd."+name+".short").
var messageIDPattern = regexp.MustCompile(
	`(?:thinktI18n|i18n)\.T[fn]?\("([a-zA-Z][a-zA-Z0-9]*(?:\.[a-zA-Z][a-zA-Z0-9]*)+)"` +
		`|` +
		`\bT[fn]?\("([a-zA-Z][a-zA-Z0-9]*(?:\.[a-zA-Z][a-zA-Z0-9]*)+)"`,
)

func collectMessageIDs(t *testing.T, root string) map[string]bool {
	t.Helper()
	ids := make(map[string]bool)

	for _, dir := range []string{"internal", "cmd"} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", path, err)
			}

			for _, m := range messageIDPattern.FindAllSubmatch(data, -1) {
				// One of the two capture groups will be non-empty.
				id := string(m[1])
				if id == "" {
					id = string(m[2])
				}
				ids[id] = true
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
	}
	return ids
}

// parseTomlKeys extracts [section.key] headers from a TOML locale file.
func parseTomlKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	keys := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[[") {
			// Extract key from [key.name]
			key := strings.Trim(line, "[] ")
			if key != "" {
				keys[key] = true
			}
		}
	}
	return keys
}

func findProjectRoot() (string, error) {
	// Start from this file's directory and walk up looking for go.mod.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
