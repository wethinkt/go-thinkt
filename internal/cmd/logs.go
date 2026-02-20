package cmd

import (
	"fmt"
	"io"
	"os"
	"time"
)

// tailLogFile prints the last n lines from path, optionally following for new content.
func tailLogFile(path string, n int, follow bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log file not found: %s", path)
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Read last n lines using a ring buffer approach
	lines, err := readLastLines(f, n)
	if err != nil {
		return err
	}

	for _, line := range lines {
		fmt.Print(line)
	}

	if !follow {
		return nil
	}

	// Follow mode: poll for new content
	for {
		buf := make([]byte, 4096)
		nr, err := f.Read(buf)
		if nr > 0 {
			os.Stdout.Write(buf[:nr])
		}
		if err != nil && err != io.EOF {
			return err
		}
		if nr == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// readLastLines reads the last n lines from a file, returning them as strings
// (each including its trailing newline if present).
func readLastLines(f *os.File, n int) ([]string, error) {
	// Get file size
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	// Read entire file (log files are typically small)
	buf := make([]byte, size)
	if _, err := f.ReadAt(buf, 0); err != nil && err != io.EOF {
		return nil, err
	}

	// Split into lines, preserving newlines
	var lines []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == '\n' {
			lines = append(lines, string(buf[start:i+1]))
			start = i + 1
		}
	}
	// Handle last line without trailing newline
	if start < len(buf) {
		lines = append(lines, string(buf[start:])+"\n")
	}

	// Return last n lines
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	// Seek to end for follow mode
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return nil, err
	}

	return lines, nil
}
