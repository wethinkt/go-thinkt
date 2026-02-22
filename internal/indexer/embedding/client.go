package embedding

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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

type Client struct {
	binary string
}

// NewClient creates a client using the default binary name.
// Returns an error if the binary is not found in PATH.
func NewClient() (*Client, error) {
	return NewClientWithBinary(DefaultBinary)
}

// NewClientWithBinary creates a client with a specific binary path.
func NewClientWithBinary(binary string) (*Client, error) {
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("embedding binary not found: %s: %w", binary, err)
	}
	return &Client{binary: path}, nil
}

// EmbedBatch sends a batch of text items to the embedding binary and returns
// the embedding vectors. Spawns one subprocess for the entire batch.
func (c *Client) EmbedBatch(ctx context.Context, items []EmbedRequest) ([]EmbedResponse, error) {
	if len(items) == 0 {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, c.binary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", c.binary, err)
	}

	// Write all requests to stdin
	enc := json.NewEncoder(stdin)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			stdin.Close()
			cmd.Wait()
			return nil, fmt.Errorf("write request: %w", err)
		}
	}
	stdin.Close()

	// Read all responses from stdout
	var results []EmbedResponse
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line
	for scanner.Scan() {
		var resp EmbedResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			continue // skip malformed lines
		}
		results = append(results, resp)
	}

	if err := cmd.Wait(); err != nil {
		return results, fmt.Errorf("%s exited with error: %w", c.binary, err)
	}
	return results, nil
}

// Available returns true if the embedding binary is found in PATH.
func Available() bool {
	_, err := exec.LookPath(DefaultBinary)
	return err == nil
}
