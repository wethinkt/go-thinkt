// Package server implements the HTTP and MCP server for thinkt serve.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/thinkt"
)

// ErrInvalidConfiguration is returned when server options conflict.
var ErrInvalidConfiguration = errors.New("invalid server configuration")

// Mode specifies how the server should run.
type Mode int

const (
	// ModeHTTPOnly serves only the REST API over HTTP.
	ModeHTTPOnly Mode = iota
	// ModeMCPStdio runs only MCP over stdin/stdout.
	ModeMCPStdio
	// ModeMCPHTTP serves only MCP over HTTP (SSE transport).
	ModeMCPHTTP
	// ModeCombined serves both REST API and MCP over HTTP on the same port.
	ModeCombined
)

// Config holds server configuration.
type Config struct {
	Mode Mode
	Port int
	Host string
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Mode: ModeCombined,
		Port: 7433,
		Host: "localhost",
	}
}

// HTTPServer serves the REST API.
type HTTPServer struct {
	registry *thinkt.StoreRegistry
	router   chi.Router
	config   Config
}

// NewHTTPServer creates a new HTTP server for the REST API.
func NewHTTPServer(registry *thinkt.StoreRegistry, config Config) *HTTPServer {
	s := &HTTPServer{
		registry: registry,
		config:   config,
	}
	s.router = s.setupRouter()
	return s
}

// setupRouter configures all routes.
func (s *HTTPServer) setupRouter() chi.Router {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/sources", s.handleGetSources)
		r.Get("/projects", s.handleGetProjects)
		r.Get("/projects/{projectID}/sessions", s.handleGetProjectSessions)
		r.Get("/sessions/*", s.handleGetSession)
	})

	// Root handler (placeholder for webapp)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Thinkt</title></head>
<body>
<h1>Thinkt Server</h1>
<p>API available at <a href="/api/v1/sources">/api/v1/sources</a></p>
</body>
</html>`))
	})

	return r
}

// Router returns the chi router for combining with other servers.
func (s *HTTPServer) Router() chi.Router {
	return s.router
}

// Addr returns the server address.
func (s *HTTPServer) Addr() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

// ListenAndServe starts the HTTP server.
func (s *HTTPServer) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr(),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Update port if it was auto-assigned
	if s.config.Port == 0 {
		s.config.Port = ln.Addr().(*net.TCPAddr).Port
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("HTTP server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}

// corsMiddleware adds CORS headers for local development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CombinedServer serves both REST API and MCP on the same port.
type CombinedServer struct {
	httpServer *HTTPServer
	mcpServer  *MCPServer
	router     chi.Router
	config     Config
}

// NewCombinedServer creates a server that serves both REST API and MCP over HTTP.
func NewCombinedServer(registry *thinkt.StoreRegistry, config Config) *CombinedServer {
	httpServer := NewHTTPServer(registry, config)
	mcpServer := NewMCPServer(registry)

	// Create combined router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	// Mount HTTP API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/sources", httpServer.handleGetSources)
		r.Get("/projects", httpServer.handleGetProjects)
		r.Get("/projects/{projectID}/sessions", httpServer.handleGetProjectSessions)
		r.Get("/sessions/*", httpServer.handleGetSession)
	})

	// MCP endpoint placeholder (for SSE transport in future)
	// r.Mount("/mcp", mcpHTTPHandler(mcpServer))

	// Root handler
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Thinkt</title></head>
<body>
<h1>Thinkt Server</h1>
<p>REST API: <a href="/api/v1/sources">/api/v1/sources</a></p>
<p>MCP: Available via stdio transport (use --mcp-only)</p>
</body>
</html>`))
	})

	return &CombinedServer{
		httpServer: httpServer,
		mcpServer:  mcpServer,
		router:     r,
		config:     config,
	}
}

// Addr returns the server address.
func (s *CombinedServer) Addr() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

// ListenAndServe starts the combined server.
func (s *CombinedServer) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr(),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	if s.config.Port == 0 {
		s.config.Port = ln.Addr().(*net.TCPAddr).Port
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}

// Server is the legacy interface for backwards compatibility.
// Use NewHTTPServer, NewMCPServer, or NewCombinedServer directly.
type Server struct {
	registry *thinkt.StoreRegistry
	router   chi.Router
	port     int
	host     string
}

// Option configures the server.
type Option func(*Server)

// WithPort sets the server port.
func WithPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// WithHost sets the server host.
func WithHost(host string) Option {
	return func(s *Server) {
		s.host = host
	}
}

// New creates a new Server (legacy interface - creates CombinedServer behavior).
func New(registry *thinkt.StoreRegistry, opts ...Option) *Server {
	s := &Server{
		registry: registry,
		port:     7433,
		host:     "localhost",
	}

	for _, opt := range opts {
		opt(s)
	}

	// Create HTTP server and use its router
	config := Config{
		Mode: ModeCombined,
		Port: s.port,
		Host: s.host,
	}
	httpServer := NewHTTPServer(registry, config)
	s.router = httpServer.Router()

	return s
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.Addr(),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	if s.port == 0 {
		s.port = ln.Addr().(*net.TCPAddr).Port
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}
