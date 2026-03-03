package thinkt

import (
	"bytes"
	"io"
	"os"
)

// CountLines counts newline characters in a file without parsing.
// Designed to be called in a background goroutine for progress estimation.
func CountLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	count := 0
	for {
		n, err := f.Read(buf)
		if n > 0 {
			count += bytes.Count(buf[:n], []byte{'\n'})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
	}
	return count, nil
}
