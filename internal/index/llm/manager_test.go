package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModelPath(t *testing.T) {
	path, err := ModelPath("nomic-embed-text-v1.5")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute path, got %s", path)
	}
	if filepath.Base(path) != "nomic-embed-text-v1.5.Q8_0.gguf" {
		t.Fatalf("unexpected filename: %s", filepath.Base(path))
	}
}

func TestModelPathUnknown(t *testing.T) {
	_, err := ModelPath("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestIsModelDownloaded(t *testing.T) {
	dir := t.TempDir()
	fakeModel := filepath.Join(dir, "test.gguf")
	if err := os.WriteFile(fakeModel, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsModelDownloaded(fakeModel) {
		t.Fatal("expected true for existing file")
	}
	if IsModelDownloaded(filepath.Join(dir, "nonexistent.gguf")) {
		t.Fatal("expected false for nonexistent file")
	}
}
