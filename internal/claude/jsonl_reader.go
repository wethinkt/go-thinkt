package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// JSONLReader provides streaming access to JSONL files without loading
// the entire file into memory. It tracks position for resumable reading
// and supports seeking back to previously read positions.
type JSONLReader struct {
	path     string
	file     *os.File
	reader   *bufio.Reader
	position int64 // byte offset in file
	lineNum  int   // 1-indexed line number
	fileSize int64
	closed   bool
}

// NewJSONLReader opens a JSONL file for streaming reads.
// Call Close() when done to release the file handle.
func NewJSONLReader(path string) (*JSONLReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	return &JSONLReader{
		path:     path,
		file:     file,
		reader:   bufio.NewReaderSize(file, 64*1024), // 64KB buffer
		position: 0,
		lineNum:  0,
		fileSize: stat.Size(),
		closed:   false,
	}, nil
}

// Path returns the file path.
func (r *JSONLReader) Path() string {
	return r.path
}

// Position returns the current byte offset in the file.
func (r *JSONLReader) Position() int64 {
	return r.position
}

// LineNum returns the number of lines read so far (1-indexed after first read).
func (r *JSONLReader) LineNum() int {
	return r.lineNum
}

// FileSize returns the total file size in bytes.
func (r *JSONLReader) FileSize() int64 {
	return r.fileSize
}

// Progress returns the percentage of file read (0.0 to 1.0).
func (r *JSONLReader) Progress() float64 {
	if r.fileSize == 0 {
		return 1.0
	}
	return float64(r.position) / float64(r.fileSize)
}

// HasMore returns true if there's more content to read.
func (r *JSONLReader) HasMore() bool {
	return r.position < r.fileSize
}

// ReadLine reads the next line from the file.
// Returns the line bytes (without newline) and any error.
// Returns nil, io.EOF when end of file is reached.
func (r *JSONLReader) ReadLine() ([]byte, error) {
	if r.closed {
		return nil, errors.New("reader is closed")
	}

	line, err := r.reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}

	if len(line) == 0 && err == io.EOF {
		return nil, io.EOF
	}

	r.position += int64(len(line))
	r.lineNum++

	// Trim trailing newline
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	// Trim trailing carriage return (Windows)
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}

	// Return EOF if this was the last line
	if err == io.EOF {
		return line, io.EOF
	}

	return line, nil
}

// ReadLines reads up to n lines from the file.
// Returns the lines read and any error (io.EOF if end reached).
func (r *JSONLReader) ReadLines(n int) ([][]byte, error) {
	if n <= 0 {
		return nil, nil
	}

	lines := make([][]byte, 0, n)
	var lastErr error

	for range n {
		line, err := r.ReadLine()
		if err == io.EOF {
			if len(line) > 0 {
				lines = append(lines, line)
			}
			lastErr = io.EOF
			break
		}
		if err != nil {
			return lines, err
		}
		if len(line) > 0 { // skip empty lines
			lines = append(lines, line)
		}
	}

	return lines, lastErr
}

// ReadJSON reads the next line and unmarshals it into dest.
// Returns io.EOF when end of file is reached.
func (r *JSONLReader) ReadJSON(dest any) error {
	line, err := r.ReadLine()
	if err == io.EOF && len(line) == 0 {
		return io.EOF
	}
	if err != nil && err != io.EOF {
		return err
	}

	// Skip empty lines
	if len(line) == 0 {
		if err == io.EOF {
			return io.EOF
		}
		return r.ReadJSON(dest)
	}

	if unmarshalErr := json.Unmarshal(line, dest); unmarshalErr != nil {
		return unmarshalErr
	}

	return err // may be io.EOF
}

// ReadUntilBytes reads lines until the cumulative content size reaches maxBytes.
// Returns the lines read, total bytes of content, and any error.
// Useful for loading a "window" of content for display.
func (r *JSONLReader) ReadUntilBytes(maxBytes int) ([][]byte, int, error) {
	var lines [][]byte
	var totalBytes int
	var lastErr error

	for totalBytes < maxBytes {
		line, err := r.ReadLine()
		if err == io.EOF {
			if len(line) > 0 {
				lines = append(lines, line)
				totalBytes += len(line)
			}
			lastErr = io.EOF
			break
		}
		if err != nil {
			return lines, totalBytes, err
		}
		if len(line) > 0 {
			lines = append(lines, line)
			totalBytes += len(line)
		}
	}

	return lines, totalBytes, lastErr
}

// SeekTo moves to the specified byte position in the file.
// This resets the buffered reader, so it's relatively expensive.
func (r *JSONLReader) SeekTo(offset int64) error {
	if r.closed {
		return errors.New("reader is closed")
	}

	_, err := r.file.Seek(offset, io.SeekStart)
	if err != nil {
		return err
	}

	r.position = offset
	r.reader.Reset(r.file)
	// Note: lineNum becomes inaccurate after seeking to arbitrary position
	return nil
}

// Reset seeks back to the beginning of the file.
func (r *JSONLReader) Reset() error {
	err := r.SeekTo(0)
	if err != nil {
		return err
	}
	r.lineNum = 0
	return nil
}

// ReadAll reads all remaining lines from current position.
// Use with caution on large files.
func (r *JSONLReader) ReadAll() ([][]byte, error) {
	var lines [][]byte

	for {
		line, err := r.ReadLine()
		if err == io.EOF {
			if len(line) > 0 {
				lines = append(lines, line)
			}
			break
		}
		if err != nil {
			return lines, err
		}
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}

	return lines, nil
}

// Close closes the underlying file handle.
func (r *JSONLReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// Snapshot captures the current reader state for later resumption.
type ReaderSnapshot struct {
	Path     string
	Position int64
	LineNum  int
}

// Snapshot returns the current reader state.
func (r *JSONLReader) Snapshot() ReaderSnapshot {
	return ReaderSnapshot{
		Path:     r.path,
		Position: r.position,
		LineNum:  r.lineNum,
	}
}

// ResumeFrom creates a new reader starting from a snapshot position.
func ResumeFrom(snap ReaderSnapshot) (*JSONLReader, error) {
	reader, err := NewJSONLReader(snap.Path)
	if err != nil {
		return nil, err
	}

	if snap.Position > 0 {
		if err := reader.SeekTo(snap.Position); err != nil {
			reader.Close()
			return nil, err
		}
		reader.lineNum = snap.LineNum
	}

	return reader, nil
}
