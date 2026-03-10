package share

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	creds := &Credentials{
		Token:    "test-token-123",
		Username: "testuser",
		Endpoint: "https://share.wethinkt.com",
	}

	err := SaveCredentials(path, creds)
	if err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	loaded, err := LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	if loaded.Token != creds.Token {
		t.Errorf("token = %q, want %q", loaded.Token, creds.Token)
	}
	if loaded.Username != creds.Username {
		t.Errorf("username = %q, want %q", loaded.Username, creds.Username)
	}
	if loaded.Endpoint != creds.Endpoint {
		t.Errorf("endpoint = %q, want %q", loaded.Endpoint, creds.Endpoint)
	}
}

func TestLoadCredentials_NotFound(t *testing.T) {
	_, err := LoadCredentials("/nonexistent/auth.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSaveCredentials_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "auth.json")

	creds := &Credentials{Token: "t", Username: "u", Endpoint: "e"}
	err := SaveCredentials(path, creds)
	if err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to be created")
	}
}
