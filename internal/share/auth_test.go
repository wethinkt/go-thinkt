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

	creds := &Credentials{Token: "t", Username: "u", Endpoint: "https://share.wethinkt.com"}
	err := SaveCredentials(path, creds)
	if err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to be created")
	}
}

func TestEndpoint_UsesValidatedEnvOverride(t *testing.T) {
	t.Setenv("THINKT_SHARE_URL", "http://localhost:8784/share")

	got, err := Endpoint()
	if err != nil {
		t.Fatalf("Endpoint: %v", err)
	}
	if got != "http://localhost:8784/share" {
		t.Fatalf("Endpoint = %q, want %q", got, "http://localhost:8784/share")
	}
}

func TestEndpoint_RejectsInvalidEnvOverride(t *testing.T) {
	t.Setenv("THINKT_SHARE_URL", "http://example.com")

	if _, err := Endpoint(); err == nil {
		t.Fatal("expected Endpoint to reject non-local http share URL")
	}
}

func TestLoadCredentials_RejectsInvalidEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	data := []byte(`{"token":"t","username":"u","endpoint":"http://example.com"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := LoadCredentials(path); err == nil {
		t.Fatal("expected LoadCredentials to reject invalid endpoint")
	}
}
