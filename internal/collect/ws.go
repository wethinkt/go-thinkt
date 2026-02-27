package collect

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

const (
	wsBackfillLimit = 50
)

// handleSessionWS upgrades to WebSocket and streams session entries in real-time.
// Auth: either Authorization header (handled by bearerAuth middleware) or ?ticket= query param.
func (s *Server) handleSessionWS(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "sessionID is required")
		return
	}

	// Check ticket auth (for browser clients that can't set headers on WS)
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		if !s.tickets.Redeem(ticket, sessionID) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid or expired ticket")
			return
		}
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS handled by middleware
	})
	if err != nil {
		tuilog.Log.Error("WebSocket accept failed", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx := r.Context()

	// Backfill: send recent entries
	afterParam := r.URL.Query().Get("after")
	var afterTime time.Time
	if afterParam != "" {
		afterTime, _ = time.Parse(time.RFC3339Nano, afterParam)
	}

	backfill, err := s.store.QueryEntries(ctx, sessionID, wsBackfillLimit, 0)
	if err != nil {
		tuilog.Log.Error("WS backfill query failed", "session_id", sessionID, "error", err)
	} else {
		for _, entry := range backfill {
			if !afterTime.IsZero() && !entry.Timestamp.After(afterTime) {
				continue
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				tuilog.Log.Debug("WS backfill write failed", "error", err)
				return
			}
		}
	}

	// Subscribe to live entries
	ch, unsub := s.pubsub.Subscribe(sessionID)
	defer unsub()

	wsConnectionsActive.Inc()
	defer wsConnectionsActive.Dec()
	tuilog.Log.Info("WebSocket client connected", "session_id", sessionID)

	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "server shutting down")
			return
		case entries, ok := <-ch:
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "subscription closed")
				return
			}
			for _, entry := range entries {
				data, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
					tuilog.Log.Debug("WS write failed", "session_id", sessionID, "error", err)
					return
				}
			}
		}
	}
}

// handleIssueTicket issues a WebSocket auth ticket for the given session.
// POST /v1/ws/ticket with body {"session_id": "..."}
func (s *Server) handleIssueTicket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body")
		return
	}
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "session_id is required")
		return
	}

	ticket := s.tickets.Issue(req.SessionID)
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket})
}
