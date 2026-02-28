package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/wethinkt/go-thinkt/internal/config"
)

// DefaultSocketPath returns the default Unix socket path for the indexer server.
func DefaultSocketPath() string {
	configDir, err := config.Dir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "indexer.sock")
}

// ServerAvailable checks whether a server is listening on the default socket.
func ServerAvailable() bool {
	return ServerAvailableAt(DefaultSocketPath())
}

// ServerAvailableAt checks whether a server is listening on the given socket path.
func ServerAvailableAt(socketPath string) bool {
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Call connects to the socket at DefaultSocketPath, sends a request for the
// given method with params, and reads back responses. Progress messages are
// delivered to progressFn (if non-nil) before the final response is returned.
func Call(method string, params any, progressFn func(Progress)) (*Response, error) {
	return CallAt(DefaultSocketPath(), method, params, progressFn)
}

// CallAt connects to the socket at the given path, sends a request, and reads
// back responses.
func CallAt(socketPath string, method string, params any, progressFn func(Progress)) (*Response, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", socketPath, err)
	}
	defer conn.Close()

	// Build the request.
	var rawParams json.RawMessage
	if params != nil {
		rawParams, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	req := Request{
		Method: method,
		Params: rawParams,
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	reqData = append(reqData, '\n')
	if _, err := conn.Write(reqData); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read responses line by line.
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Check if this is a progress message by looking for "progress":true.
		// We do a quick probe to decide which type to unmarshal into.
		var probe struct {
			Progress bool `json:"progress"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil, fmt.Errorf("unmarshal response line: %w", err)
		}

		if probe.Progress {
			if progressFn != nil {
				var p Progress
				if err := json.Unmarshal(line, &p); err != nil {
					return nil, fmt.Errorf("unmarshal progress: %w", err)
				}
				progressFn(p)
			}
			continue
		}

		// Final response.
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
		return &resp, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return nil, fmt.Errorf("connection closed without response")
}
