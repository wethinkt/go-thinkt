package collect

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// handleIngest processes POST /v1/traces requests from exporters.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body")
		return
	}

	dropped, err := NormalizeRequest(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if dropped > 0 {
		tuilog.Log.Info("Dropped invalid entries during normalization",
			"session_id", req.SessionID, "dropped", dropped)
	}

	if len(req.Entries) == 0 {
		writeJSON(w, http.StatusOK, IngestResponse{Accepted: 0, Message: "all entries dropped during validation"})
		return
	}

	// Update agent heartbeat and trace count
	s.registry.Heartbeat(req.InstanceID)
	s.registry.IncrementTraceCount(req.InstanceID, int64(len(req.Entries)))

	if err := s.store.IngestBatch(r.Context(), req); err != nil {
		tuilog.Log.Error("Failed to ingest batch",
			"session_id", req.SessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "ingest_error", "Failed to store traces")
		return
	}

	tuilog.Log.Info("Ingested traces",
		"session_id", req.SessionID,
		"source", req.Source,
		"entries", len(req.Entries))

	writeJSON(w, http.StatusOK, IngestResponse{
		Accepted: len(req.Entries),
	})
}

// handleSearchTraces handles GET /v1/traces/search.
func (s *Server) handleSearchTraces(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "q parameter is required")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	results, err := s.store.SearchTraces(r.Context(), query, limit)
	if err != nil {
		tuilog.Log.Error("Failed to search traces", "query", query, "error", err)
		writeError(w, http.StatusInternalServerError, "search_error", "Failed to search traces")
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// handleRegisterAgent processes POST /v1/agents/register requests.
func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var reg AgentRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body")
		return
	}

	if reg.InstanceID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "instance_id is required")
		return
	}
	if reg.Platform == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "platform is required")
		return
	}

	info := s.registry.Register(reg)
	tuilog.Log.Info("Agent registered",
		"instance_id", reg.InstanceID,
		"platform", reg.Platform,
		"hostname", reg.Hostname)

	writeJSON(w, http.StatusOK, info)
}

// handleListAgents returns all registered agents.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.registry.List()
	writeJSON(w, http.StatusOK, agents)
}

// handleGetUsageStats returns collector usage statistics.
func (s *Server) handleGetUsageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetUsageStats(r.Context())
	if err != nil {
		tuilog.Log.Error("Failed to get stats", "error", err)
		writeError(w, http.StatusInternalServerError, "stats_error", "Failed to retrieve statistics")
		return
	}

	total, active := s.registry.Count()
	stats.TotalAgents = total
	stats.ActiveAgents = active

	writeJSON(w, http.StatusOK, stats)
}

// handleSessionActivity processes POST /v1/sessions/activity requests from exporters.
func (s *Server) handleSessionActivity(w http.ResponseWriter, r *http.Request) {
	var event SessionActivityEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body")
		return
	}

	if event.SessionID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "session_id is required")
		return
	}
	if event.Event == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "event is required")
		return
	}

	validEvents := map[string]bool{"session_start": true, "session_active": true, "session_end": true}
	if !validEvents[event.Event] {
		writeError(w, http.StatusBadRequest, "validation_error", "event must be session_start, session_active, or session_end")
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	if err := s.store.RecordSessionActivity(r.Context(), event); err != nil {
		tuilog.Log.Error("Failed to record session activity",
			"session_id", event.SessionID, "event", event.Event, "error", err)
		writeError(w, http.StatusInternalServerError, "activity_error", "Failed to record session activity")
		return
	}

	tuilog.Log.Info("Session activity recorded",
		"session_id", event.SessionID, "event", event.Event,
		"source", event.Source)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleActiveSessions handles GET /v1/sessions/active.
func (s *Server) handleActiveSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.QueryActiveSessions(r.Context())
	if err != nil {
		tuilog.Log.Error("Failed to query active sessions", "error", err)
		writeError(w, http.StatusInternalServerError, "query_error", "Failed to query active sessions")
		return
	}

	if sessions == nil {
		sessions = []SessionSummary{}
	}

	writeJSON(w, http.StatusOK, sessions)
}

// handleHealth returns a health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
