package embedding

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

const DefaultBinary = "thinkt-embed-apple"

type EmbedRequest struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type EmbedResponse struct {
	ID        string    `json:"id"`
	Embedding []float32 `json:"embedding"`
	Dim       int       `json:"dim"`
}

// Client manages a long-running embedding subprocess.
// The model loads once and stays resident for all embed calls.
type Client struct {
	binary string

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
}

// NewClient creates a client using the default binary name.
func NewClient() (*Client, error) {
	return NewClientWithBinary(DefaultBinary)
}

// NewClientWithBinary creates a client with a specific binary path.
func NewClientWithBinary(binary string) (*Client, error) {
	path := findBinary(binary)
	if path == "" {
		return nil, fmt.Errorf("embedding binary not found: %s", binary)
	}
	return &Client{binary: path}, nil
}

// findBinary looks for the named binary next to the current executable,
// then falls back to PATH lookup.
func findBinary(name string) string {
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(sibling); err == nil {
			return sibling
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

// ensureRunning starts the subprocess if it isn't already running.
func (c *Client) ensureRunning() error {
	if c.cmd != nil && c.cmd.Process != nil {
		return nil
	}

	cmd := exec.Command(c.binary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", c.binary, err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	c.cmd = cmd
	c.stdin = stdin
	c.scanner = scanner
	return nil
}

// EmbedBatch sends items to the persistent subprocess and reads responses
// one at a time. The subprocess stays running for future calls.
func (c *Client) EmbedBatch(ctx context.Context, items []EmbedRequest) ([]EmbedResponse, error) {
	if len(items) == 0 {
		return nil, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureRunning(); err != nil {
		return nil, err
	}

	enc := json.NewEncoder(c.stdin)
	var results []EmbedResponse

	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			// Process may have died — reset and return what we have
			c.kill()
			return results, fmt.Errorf("write request: %w", err)
		}

		if !c.scanner.Scan() {
			c.kill()
			return results, fmt.Errorf("unexpected EOF from %s", c.binary)
		}

		var resp EmbedResponse
		if err := json.Unmarshal(c.scanner.Bytes(), &resp); err != nil {
			continue // skip malformed
		}
		results = append(results, resp)
	}

	return results, nil
}

// Close shuts down the subprocess.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.kill()
}

func (c *Client) kill() error {
	if c.cmd == nil {
		return nil
	}
	c.stdin.Close()
	err := c.cmd.Wait()
	c.cmd = nil
	c.stdin = nil
	c.scanner = nil
	return err
}

// Available returns true if the embedding binary can be found.
func Available() bool {
	return findBinary(DefaultBinary) != ""
}
