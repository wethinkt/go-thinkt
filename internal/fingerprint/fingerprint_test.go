package fingerprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeFingerprint(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "a1b2c3d4e5f67890abcd ef1234567890",
			expected: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
		{
			input:    "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
			expected: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
		{
			input:    "22414C5F-CFA8-5D30-8EB4-F1A9C49D355C",
			expected: "22414c5f-cfa8-5d30-8eb4-f1a9c49d355c",
		},
		{
			// Short input gets hashed (SHA-256 based)
			input:    "short",
			expected: "f9b0078b-5df5-96d2-ea19-010c001bbd00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeFingerprint(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeFingerprint(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatAsUUID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "a1b2c3d4e5f67890abcdef1234567890",
			expected: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
		{
			input:    "SHORT",
			expected: "short000-0000-0000-0000-000000000000",
		},
		{
			input:    "thisisaverylongstringthatexceeds32chars",
			expected: "thisisav-eryl-ongs-trin-gthatexceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatAsUUID(tt.input)
			if got != tt.expected {
				t.Errorf("formatAsUUID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetMachineIDPath(t *testing.T) {
	path, err := getMachineIDPath()
	if err != nil {
		t.Fatalf("getMachineIDPath() error = %v", err)
	}

	if !strings.HasSuffix(path, ".thinkt/machine_id") {
		t.Errorf("getMachineIDPath() = %q, want suffix '.thinkt/machine_id'", path)
	}
}

func TestCachedFingerprint(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Override the machine ID path
	origPath := getMachineIDPath
	getMachineIDPath = func() (string, error) {
		return filepath.Join(tmpDir, "machine_id"), nil
	}
	defer func() { getMachineIDPath = origPath }()

	// Test with no cache
	info := getCachedFingerprint()
	if info.Fingerprint != "" {
		t.Errorf("getCachedFingerprint() with empty cache = %v, want empty", info)
	}

	// Test with cached value
	testFP := "test-fingerprint-1234"
	if err := os.WriteFile(filepath.Join(tmpDir, "machine_id"), []byte(testFP+"\n"), 0600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	info = getCachedFingerprint()
	if info.Fingerprint == "" {
		t.Error("getCachedFingerprint() with valid cache = empty, want fingerprint")
	}
	if info.Source != "thinkt-generated" {
		t.Errorf("getCachedFingerprint().Source = %q, want 'thinkt-generated'", info.Source)
	}
}

func TestGenerateAndCacheFingerprint(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Override the machine ID path
	origPath := getMachineIDPath
	getMachineIDPath = func() (string, error) {
		return filepath.Join(tmpDir, "machine_id"), nil
	}
	defer func() { getMachineIDPath = origPath }()

	// Generate fingerprint
	info, err := generateAndCacheFingerprint()
	if err != nil {
		t.Fatalf("generateAndCacheFingerprint() error = %v", err)
	}

	if info.Fingerprint == "" {
		t.Error("generateAndCacheFingerprint().Fingerprint = empty")
	}
	if info.Source != "thinkt-generated" {
		t.Errorf("generateAndCacheFingerprint().Source = %q, want 'thinkt-generated'", info.Source)
	}

	// Verify file was created
	if _, err := os.Stat(filepath.Join(tmpDir, "machine_id")); err != nil {
		t.Errorf("machine_id file not created: %v", err)
	}

	// Verify we can read it back
	info2 := getCachedFingerprint()
	if info2.Fingerprint != info.Fingerprint {
		t.Errorf("Cached fingerprint %q != original %q", info2.Fingerprint, info.Fingerprint)
	}
}

func TestGet(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Override the machine ID path
	origPath := getMachineIDPath
	getMachineIDPath = func() (string, error) {
		return filepath.Join(tmpDir, "machine_id"), nil
	}
	defer func() { getMachineIDPath = origPath }()

	// Get fingerprint (should generate since no system ID)
	info, err := Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if info.Fingerprint == "" {
		t.Error("Get().Fingerprint = empty")
	}

	// Should be in UUID format
	if len(info.Fingerprint) != 36 {
		t.Errorf("Get().Fingerprint length = %d, want 36 (UUID format)", len(info.Fingerprint))
	}

	// Get again (should return cached)
	info2, err := Get()
	if err != nil {
		t.Fatalf("Get() (second call) error = %v", err)
	}

	if info2.Fingerprint != info.Fingerprint {
		t.Errorf("Get() returned different fingerprint: %q vs %q", info2.Fingerprint, info.Fingerprint)
	}
}

func TestGetFingerprint(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Override the machine ID path
	origPath := getMachineIDPath
	getMachineIDPath = func() (string, error) {
		return filepath.Join(tmpDir, "machine_id"), nil
	}
	defer func() { getMachineIDPath = origPath }()

	fp, err := GetFingerprint()
	if err != nil {
		t.Fatalf("GetFingerprint() error = %v", err)
	}

	if fp == "" {
		t.Error("GetFingerprint() = empty")
	}

	if len(fp) != 36 {
		t.Errorf("GetFingerprint() length = %d, want 36", len(fp))
	}
}

func TestInfoString(t *testing.T) {
	info := Info{
		Fingerprint: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Source:      "test-source",
		Path:        "/test/path",
	}

	s := info.String()
	if !strings.Contains(s, "a1b2c3d4-e5f6-7890-abcd-ef1234567890") {
		t.Error("Info.String() missing fingerprint")
	}
	if !strings.Contains(s, "test-source") {
		t.Error("Info.String() missing source")
	}
	if !strings.Contains(s, "/test/path") {
		t.Error("Info.String() missing path")
	}
}
