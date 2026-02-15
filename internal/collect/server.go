package collect

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/wethinkt/go-thinkt/internal/config"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// InstanceCollector is the instance type for the collector process.
const InstanceCollector config.InstanceType = "collector"

// Server is the collector HTTP server that receives traces from exporters.
type Server struct {
	config    CollectorConfig
	store     TraceStore
	registry  *AgentRegistry
	router    chi.Router
	startedAt time.Time
}

// NewServer creates a new collector server. It opens the DuckDB store and
// sets up the HTTP router.
func NewServer(cfg CollectorConfig) (*Server, error) {
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	if cfg.Host == "" {
		cfg.Host = DefaultHost
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = DefaultBatchSize
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = DefaultFlushInterval
	}

	dbPath := cfg.DBPath
	if dbPath == "" {
		dir, err := config.Dir()
		if err != nil {
			return nil, fmt.Errorf("resolve config dir: %w", err)
		}
		dbPath = dir + "/collector.duckdb"
	}

	store, err := NewDuckDBStore(dbPath, cfg.BatchSize, cfg.FlushInterval)
	if err != nil {
		return nil, fmt.Errorf("open collector store: %w", err)
	}

	s := &Server{
		config:    cfg,
		store:     store,
		registry:  NewAgentRegistry(),
		startedAt: time.Now(),
	}
	s.router = s.setupRouter()
	return s, nil
}

// setupRouter configures the collector HTTP routes and middleware.
func (s *Server) setupRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	if !s.config.Quiet {
		r.Use(middleware.Logger)
	}

	// Bearer token auth
	if s.config.Token != "" {
		tuilog.Log.Info("Collector authentication enabled")
		r.Use(bearerAuth(s.config.Token))
	} else {
		tuilog.Log.Warn("Collector running without authentication - use --token to secure")
	}

	r.Route("/v1", func(r chi.Router) {
		r.Post("/traces", s.handleIngest)
		r.Get("/traces/search", s.handleSearchTraces)
		r.Get("/traces/stats", s.handleGetUsageStats)
		r.Post("/agents/register", s.handleRegisterAgent)
		r.Get("/agents", s.handleListAgents)
		r.Get("/collector/health", s.handleHealth)
	})

	return r
}

// ListenAndServe starts the collector server and blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Check for port conflicts
	if existing := config.FindInstanceByPort(s.config.Port); existing != nil {
		return fmt.Errorf("port %d is already in use by thinkt %s (PID %d, started %s)",
			s.config.Port, existing.Type, existing.PID, existing.StartedAt.Format(time.RFC3339))
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler: s.router,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// Update port if auto-assigned
	if s.config.Port == 0 {
		s.config.Port = ln.Addr().(*net.TCPAddr).Port
	}

	// Register instance for discovery
	inst := config.Instance{
		Type:      InstanceCollector,
		PID:       os.Getpid(),
		Port:      s.config.Port,
		Host:      s.config.Host,
		StartedAt: time.Now(),
	}
	if err := config.RegisterInstance(inst); err != nil {
		tuilog.Log.Warn("Failed to register collector instance", "error", err)
	}

	// Stale agent cleanup ticker
	go s.cleanStaleAgents(ctx)

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		config.UnregisterInstance(os.Getpid())
		s.store.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Collector server running at http://%s:%d\n", s.config.Host, s.config.Port)
	return srv.Serve(ln)
}

// Addr returns the server address string.
func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
}

// cleanStaleAgents periodically removes stale agents from the registry.
func (s *Server) cleanStaleAgents(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if removed := s.registry.CleanStale(StaleAgentThreshold); removed > 0 {
				tuilog.Log.Info("Cleaned stale agents", "removed", removed)
			}
		}
	}
}

// bearerAuth returns middleware that validates a bearer token using
// constant-time comparison to prevent timing attacks.
func bearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow health checks without auth
			if r.URL.Path == "/v1/collector/health" {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="thinkt-collector"`)
				writeError(w, http.StatusUnauthorized, "unauthorized", "Missing Authorization header")
				return
			}

			const prefix = "Bearer "
			if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid Authorization header format")
				return
			}

			if subtle.ConstantTimeCompare([]byte(auth[len(prefix):]), []byte(token)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware adds CORS headers for cross-origin requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ErrorResponse is an API error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, err string, msg string) {
	writeJSON(w, status, ErrorResponse{Error: err, Message: msg})
}
