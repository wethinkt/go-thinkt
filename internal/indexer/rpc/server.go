package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
)

// Handler defines the methods that an RPC server can dispatch to.
type Handler interface {
	HandleSync(ctx context.Context, params SyncParams, send func(Progress)) (*Response, error)
	HandleSearch(ctx context.Context, params SearchParams) (*Response, error)
	HandleSemanticSearch(ctx context.Context, params SemanticSearchParams) (*Response, error)
	HandleStats(ctx context.Context) (*Response, error)
	HandleStatus(ctx context.Context) (*Response, error)
	HandleConfigReload(ctx context.Context) (*Response, error)
}

// Server listens on a Unix domain socket and dispatches RPC requests to a Handler.
type Server struct {
	socketPath string
	handler    Handler
	listener   net.Listener
	wg         sync.WaitGroup
}

// NewServer creates a new RPC server that will listen on the given socket path.
func NewServer(socketPath string, handler Handler) *Server {
	return &Server{
		socketPath: socketPath,
		handler:    handler,
	}
}

// Start removes any stale socket file, begins listening on the Unix socket,
// and accepts connections in a background goroutine.
func (s *Server) Start() error {
	// Remove stale socket file if it exists.
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}
	s.listener = ln

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Listener was closed; stop accepting.
				return
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleConn(conn)
			}()
		}
	}()

	return nil
}

// Stop closes the listener, removes the socket file, and waits for all
// active connections to drain.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
	_ = os.Remove(s.socketPath)
	s.wg.Wait()
}

// handleConn reads a single JSON-line request from the connection, dispatches
// it to the appropriate handler method, and writes back the response.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			slog.Debug("rpc: failed to read request", "error", err)
		}
		return
	}

	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeJSON(conn, Response{OK: false, Error: "invalid request: " + err.Error()})
		return
	}

	ctx := context.Background()

	var resp *Response
	var handlerErr error

	switch req.Method {
	case "sync":
		var params SyncParams
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeJSON(conn, Response{OK: false, Error: "invalid sync params: " + err.Error()})
				return
			}
		}
		send := func(p Progress) {
			p.Progress = true
			writeJSON(conn, p)
		}
		resp, handlerErr = s.handler.HandleSync(ctx, params, send)

	case "search":
		var params SearchParams
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeJSON(conn, Response{OK: false, Error: "invalid search params: " + err.Error()})
				return
			}
		}
		resp, handlerErr = s.handler.HandleSearch(ctx, params)

	case "semantic_search":
		var params SemanticSearchParams
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeJSON(conn, Response{OK: false, Error: "invalid semantic_search params: " + err.Error()})
				return
			}
		}
		resp, handlerErr = s.handler.HandleSemanticSearch(ctx, params)

	case "stats":
		resp, handlerErr = s.handler.HandleStats(ctx)

	case "status":
		resp, handlerErr = s.handler.HandleStatus(ctx)

	case "config_reload":
		resp, handlerErr = s.handler.HandleConfigReload(ctx)

	default:
		writeJSON(conn, Response{OK: false, Error: fmt.Sprintf("unknown method: %q", req.Method)})
		return
	}

	if handlerErr != nil {
		writeJSON(conn, Response{OK: false, Error: handlerErr.Error()})
		return
	}
	if resp != nil {
		writeJSON(conn, *resp)
	}
}

// writeJSON marshals v as JSON and writes it to w followed by a newline.
func writeJSON(w net.Conn, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("rpc: failed to marshal response", "error", err)
		return
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		slog.Debug("rpc: failed to write response", "error", err)
	}
}
