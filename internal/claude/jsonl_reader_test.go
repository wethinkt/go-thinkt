package claude

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func createTestJSONL(t *testing.T, lines []string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test file: %v", err)
	}
	defer f.Close()

	for _, line := range lines {
		f.WriteString(line + "\n")
	}

	return path
}

func TestJSONLReader_ReadLine(t *testing.T) {
	lines := []string{
		`{"id": 1, "name": "first"}`,
		`{"id": 2, "name": "second"}`,
		`{"id": 3, "name": "third"}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	// Read all lines
	for i, expected := range lines {
		line, err := reader.ReadLine()
		if err != nil && err != io.EOF {
			t.Fatalf("ReadLine %d: %v", i, err)
		}
		if string(line) != expected {
			t.Errorf("line %d: got %q, want %q", i, string(line), expected)
		}
		if reader.LineNum() != i+1 {
			t.Errorf("LineNum after line %d: got %d, want %d", i, reader.LineNum(), i+1)
		}
	}

	// Next read should be EOF
	line, err := reader.ReadLine()
	if err != io.EOF {
		t.Errorf("expected EOF, got err=%v, line=%q", err, string(line))
	}
}

func TestJSONLReader_ReadLines(t *testing.T) {
	lines := []string{
		`{"id": 1}`,
		`{"id": 2}`,
		`{"id": 3}`,
		`{"id": 4}`,
		`{"id": 5}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	// Read first 2 lines
	batch, err := reader.ReadLines(2)
	if err != nil {
		t.Fatalf("ReadLines(2): %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("got %d lines, want 2", len(batch))
	}

	// Read next 2 lines
	batch, err = reader.ReadLines(2)
	if err != nil {
		t.Fatalf("ReadLines(2): %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("got %d lines, want 2", len(batch))
	}

	// Read remaining (only 1 left)
	batch, err = reader.ReadLines(5)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if len(batch) != 1 {
		t.Errorf("got %d lines, want 1", len(batch))
	}
}

func TestJSONLReader_ReadJSON(t *testing.T) {
	type testEntry struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	lines := []string{
		`{"id": 1, "name": "alice"}`,
		`{"id": 2, "name": "bob"}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	var entry testEntry
	err = reader.ReadJSON(&entry)
	if err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if entry.ID != 1 || entry.Name != "alice" {
		t.Errorf("got %+v, want {ID:1 Name:alice}", entry)
	}

	err = reader.ReadJSON(&entry)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadJSON: %v", err)
	}
	if entry.ID != 2 || entry.Name != "bob" {
		t.Errorf("got %+v, want {ID:2 Name:bob}", entry)
	}
}

func TestJSONLReader_Position(t *testing.T) {
	lines := []string{
		`{"id": 1}`,
		`{"id": 2}`,
		`{"id": 3}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	if reader.Position() != 0 {
		t.Errorf("initial position: got %d, want 0", reader.Position())
	}

	// Read first line - position should advance
	reader.ReadLine()
	pos1 := reader.Position()
	if pos1 == 0 {
		t.Error("position should advance after ReadLine")
	}

	// Read second line
	reader.ReadLine()
	pos2 := reader.Position()
	if pos2 <= pos1 {
		t.Errorf("position should increase: pos1=%d, pos2=%d", pos1, pos2)
	}
}

func TestJSONLReader_Seek(t *testing.T) {
	lines := []string{
		`{"id": 1}`,
		`{"id": 2}`,
		`{"id": 3}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	// Read first line and save position
	reader.ReadLine()
	posAfterFirst := reader.Position()

	// Read second line
	line2, _ := reader.ReadLine()

	// Seek back to after first line
	err = reader.SeekTo(posAfterFirst)
	if err != nil {
		t.Fatalf("Seek: %v", err)
	}

	// Should read second line again
	line2Again, _ := reader.ReadLine()
	if string(line2) != string(line2Again) {
		t.Errorf("after seek: got %q, want %q", string(line2Again), string(line2))
	}
}

func TestJSONLReader_Reset(t *testing.T) {
	lines := []string{
		`{"id": 1}`,
		`{"id": 2}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	// Read all lines
	reader.ReadAll()
	if reader.HasMore() {
		t.Error("HasMore should be false after ReadAll")
	}

	// Reset
	err = reader.Reset()
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if reader.Position() != 0 {
		t.Errorf("position after reset: got %d, want 0", reader.Position())
	}
	if reader.LineNum() != 0 {
		t.Errorf("lineNum after reset: got %d, want 0", reader.LineNum())
	}
	if !reader.HasMore() {
		t.Error("HasMore should be true after Reset")
	}

	// Should be able to read first line again
	line, _ := reader.ReadLine()
	if string(line) != lines[0] {
		t.Errorf("after reset: got %q, want %q", string(line), lines[0])
	}
}

func TestJSONLReader_ReadUntilBytes(t *testing.T) {
	// Create lines of known sizes
	lines := []string{
		`{"data": "aaaaaaaaaa"}`, // ~25 bytes
		`{"data": "bbbbbbbbbb"}`, // ~25 bytes
		`{"data": "cccccccccc"}`, // ~25 bytes
		`{"data": "dddddddddd"}`, // ~25 bytes
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	// Read until we have ~50 bytes (should get 2 lines)
	batch, totalBytes, err := reader.ReadUntilBytes(50)
	if err != nil {
		t.Fatalf("ReadUntilBytes: %v", err)
	}
	if len(batch) < 2 {
		t.Errorf("got %d lines, want at least 2", len(batch))
	}
	if totalBytes < 50 {
		t.Errorf("got %d bytes, want at least 50", totalBytes)
	}
}

func TestJSONLReader_Snapshot(t *testing.T) {
	lines := []string{
		`{"id": 1}`,
		`{"id": 2}`,
		`{"id": 3}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}

	// Read first line
	reader.ReadLine()

	// Take snapshot
	snap := reader.Snapshot()
	reader.Close()

	// Resume from snapshot
	reader2, err := ResumeFrom(snap)
	if err != nil {
		t.Fatalf("ResumeFrom: %v", err)
	}
	defer reader2.Close()

	// Should read second line
	line, _ := reader2.ReadLine()
	if string(line) != lines[1] {
		t.Errorf("after resume: got %q, want %q", string(line), lines[1])
	}
}

func TestJSONLReader_Progress(t *testing.T) {
	lines := []string{
		`{"id": 1}`,
		`{"id": 2}`,
	}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	if reader.Progress() != 0 {
		t.Errorf("initial progress: got %f, want 0", reader.Progress())
	}

	reader.ReadAll()

	if reader.Progress() != 1.0 {
		t.Errorf("final progress: got %f, want 1.0", reader.Progress())
	}
}

func TestJSONLReader_EmptyLines(t *testing.T) {
	// File with empty lines interspersed
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")

	content := `{"id": 1}

{"id": 2}

{"id": 3}
`
	os.WriteFile(path, []byte(content), 0644)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	// ReadLines should skip empty lines
	batch, err := reader.ReadLines(10)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if len(batch) != 3 {
		t.Errorf("got %d non-empty lines, want 3", len(batch))
	}
}

func TestJSONLReader_LargeLines(t *testing.T) {
	// Create a line larger than the default buffer
	largeData := make([]byte, 100*1024) // 100KB
	for i := range largeData {
		largeData[i] = 'x'
	}

	entry := map[string]string{"data": string(largeData)}
	lineBytes, _ := json.Marshal(entry)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jsonl")
	os.WriteFile(path, append(lineBytes, '\n'), 0644)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}
	defer reader.Close()

	line, err := reader.ReadLine()
	if err != nil && err != io.EOF {
		t.Fatalf("ReadLine: %v", err)
	}
	if len(line) != len(lineBytes) {
		t.Errorf("got %d bytes, want %d", len(line), len(lineBytes))
	}
}

func TestJSONLReader_ClosedReader(t *testing.T) {
	lines := []string{`{"id": 1}`}
	path := createTestJSONL(t, lines)

	reader, err := NewJSONLReader(path)
	if err != nil {
		t.Fatalf("NewJSONLReader: %v", err)
	}

	reader.Close()

	// Operations on closed reader should fail
	_, err = reader.ReadLine()
	if err == nil {
		t.Error("ReadLine on closed reader should fail")
	}

	err = reader.SeekTo(0)
	if err == nil {
		t.Error("Seek on closed reader should fail")
	}
}
