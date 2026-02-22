// Package server implements the HTTP and MCP server for thinkt.
package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/wethinkt/go-thinkt/internal/config"
	_ "github.com/wethinkt/go-thinkt/internal/server/docs" // swagger docs
	"github.com/wethinkt/go-thinkt/internal/thinkt"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Default port constants for thinkt servers.
const (
	// DefaultPortServer is the default port for 'thinkt server' (REST API).
	DefaultPortServer = 8784
	// DefaultPortLite is the default port for 'thinkt server lite'.
	DefaultPortLite = 8785
	// DefaultPortMCP is the default port for 'thinkt server mcp' over HTTP.
	DefaultPortMCP = 8786
)

// Config holds server configuration.
type Config struct {
	Port          int
	Host          string
	Quiet         bool                // suppress HTTP request logging
	HTTPLog       string              // path to HTTP access log file (empty = stdout, "-" = discard unless Quiet)
	CORSOrigin    string              // Access-Control-Allow-Origin value (default "*")
	StaticHandler http.Handler        // if nil, defaults to StaticLiteWebAppHandler()
	InstanceType  config.InstanceType // instance type for discovery file registration
	LogPath       string              // path to process log file (for instance registry)
	HTTPLogPath   string              // path to HTTP access log file (for instance registry)
	Token         string              // resolved auth token (stored in instance registry for discovery)
	httpLogFile   *os.File            // internal: opened log file handle
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Port: DefaultPortServer,
		Host: "localhost",
	}
}

// logWriter wraps an io.Writer to implement middleware.LoggerInterface.
type logWriter struct {
	w io.Writer
}

func (l *logWriter) Print(v ...interface{}) {
	// Ensure newline - chi's middleware doesn't add them
	fmt.Fprintln(l.w, v...)
}

func (l *logWriter) Println(v ...interface{}) {
	fmt.Fprintln(l.w, v...)
}

func (l *logWriter) Printf(format string, v ...interface{}) {
	fmt.Fprintf(l.w, format, v...)
}

// httpLogWriter returns the writer for HTTP access logs based on config.
// It returns nil if logging should be suppressed.
func (c *Config) httpLogWriter() (*logWriter, io.Closer, error) {
	if c.Quiet {
		return nil, nil, nil
	}
	if c.HTTPLog == "" {
		return &logWriter{w: os.Stdout}, nil, nil
	}
	if c.HTTPLog == "-" {
		return nil, nil, nil
	}
	f, err := os.OpenFile(c.HTTPLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("open http log: %w", err)
	}
	c.httpLogFile = f
	return &logWriter{w: f}, f, nil
}

// Close cleans up any opened resources.
func (c *Config) Close() {
	if c.httpLogFile != nil {
		c.httpLogFile.Close()
		c.httpLogFile = nil
	}
}

// HTTPServer serves the REST API.
type HTTPServer struct {
	registry      *thinkt.StoreRegistry
	teamStore     thinkt.TeamStore
	router        chi.Router
	config        Config
	pathValidator *thinkt.PathValidator
	authenticator *BearerAuthenticator
}

// NewHTTPServer creates a new HTTP server for the REST API.
func NewHTTPServer(registry *thinkt.StoreRegistry, config Config) *HTTPServer {
	return NewHTTPServerWithAuth(registry, config, DefaultAPIAuthConfig())
}

// NewHTTPServerWithAuth creates a new HTTP server with authentication.
func NewHTTPServerWithAuth(registry *thinkt.StoreRegistry, config Config, authConfig AuthConfig) *HTTPServer {
	s := &HTTPServer{
		registry:      registry,
		config:        config,
		pathValidator: thinkt.NewPathValidator(registry),
		authenticator: NewBearerAuthenticator(authConfig),
	}
	s.router = s.setupRouter()
	return s
}

// SetTeamStore sets the team store for team API endpoints.
func (s *HTTPServer) SetTeamStore(ts thinkt.TeamStore) {
	s.teamStore = ts
}

// setupRouter configures all routes.
func (s *HTTPServer) setupRouter() chi.Router {
	r := chi.NewRouter()

	// HTTP access logging middleware
	logWriter, _, err := s.config.httpLogWriter()
	if err != nil {
		// Log error to stderr but continue (non-fatal)
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	if logWriter != nil {
		r.Use(middleware.RequestLogger(&middleware.DefaultLogFormatter{
			Logger:  logWriter,
			NoColor: true,
		}))
	}

	// Middleware
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware(s.config.CORSOrigin))

	// Log authentication status
	if s.authenticator.IsEnabled() {
		tuilog.Log.Info("API server authentication enabled")
	} else {
		tuilog.Log.Warn("API server running without authentication - use THINKT_API_TOKEN or --token to secure")
	}

	// API routes (auth middleware applied here, not globally, so static assets are unprotected)
	r.Route("/api/v1", func(r chi.Router) {
		if s.authenticator.IsEnabled() {
			r.Use(s.authenticator.Middleware)
		}
		r.Get("/sources", s.handleGetSources)
		r.Get("/projects", s.handleGetProjects)
		r.Get("/projects/{source}/{projectID}/sessions", s.handleGetProjectSessionsBySource)
		r.Get("/projects/{projectID}/sessions", s.handleGetProjectSessions)
		r.Get("/sessions/resume/*", s.handleResumeSession)
		r.Get("/sessions/*", s.handleGetSession)

		// Open-in endpoints
		r.Post("/open-in", s.handleOpenIn)
		r.Get("/open-in/apps", s.handleGetAllowedApps)

		// Themes endpoint
		r.Get("/themes", s.handleGetThemes)

		// Indexer endpoints (search/stats)
		r.Get("/search", s.handleSearchSessions)
		r.Get("/stats", s.handleGetStats)
		r.Get("/indexer/health", s.handleIndexerHealth)

		// Team endpoints
		r.Route("/teams", func(r chi.Router) {
			r.Use(s.requireTeamStore)
			r.Get("/", s.handleGetTeams)
			r.Get("/{teamName}", s.handleGetTeam)
			r.Get("/{teamName}/tasks", s.handleGetTeamTasks)
			r.Get("/{teamName}/members/{memberName}/messages", s.handleGetTeamMemberMessages)
		})
	})

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Lite webapp at /lite — redirect /lite to /lite/ so relative URLs resolve correctly
	r.Get("/lite", func(w http.ResponseWriter, req *http.Request) {
		target := "/lite/"
		if req.URL.RawQuery != "" {
			target += "?" + req.URL.RawQuery
		}
		http.Redirect(w, req, target, http.StatusMovedPermanently)
	})
	r.Mount("/lite/", http.StripPrefix("/lite", StaticLiteWebAppHandler()))

	// Serve full webapp for all other routes
	if s.config.StaticHandler != nil {
		r.Handle("/*", s.config.StaticHandler)
	} else {
		r.Handle("/*", StaticWebAppHandler())
	}

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
	// Check for port conflicts via instance discovery
	if s.config.Port != 0 {
		if existing := config.FindInstanceByPort(s.config.Port); existing != nil {
			return fmt.Errorf("port %d is already in use by thinkt %s (PID %d, started %s)",
				s.config.Port, existing.Type, existing.PID, existing.StartedAt.Format(time.RFC3339))
		}
	}

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

	// Register instance for discovery
	instType := s.config.InstanceType
	if instType == "" {
		instType = config.InstanceServer
	}
	inst := config.Instance{
		Type:        instType,
		PID:         os.Getpid(),
		Port:        s.config.Port,
		Host:        s.config.Host,
		LogPath:     s.config.LogPath,
		HTTPLogPath: s.config.HTTPLogPath,
		Token:       s.config.Token,
		StartedAt:   time.Now(),
	}
	if err := config.RegisterInstance(inst); err != nil {
		tuilog.Log.Warn("Failed to register instance", "error", err)
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		_ = config.UnregisterInstance(os.Getpid()) // Ignore error, cleanup is best-effort
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx) // Ignore error, shutdown errors are logged by server
	}()

	fmt.Printf("HTTP server running at http://%s\n", s.Addr())
	return srv.Serve(ln)
}

// corsMiddleware returns middleware that adds CORS headers.
func corsMiddleware(origin string) func(http.Handler) http.Handler {
	if origin == "" {
		origin = "*"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
