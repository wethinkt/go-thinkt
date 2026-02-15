package collect

import (
	"encoding/json"
	"net/http"
	"strconv"

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

// handleHealth returns a health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
