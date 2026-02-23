package rpc

import "encoding/json"

// Request is a JSON-over-newline RPC request.
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is a final RPC response.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Progress is a streaming progress update.
type Progress struct {
	Progress bool            `json:"progress"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// SyncParams for the sync method.
type SyncParams struct {
	Force bool `json:"force,omitempty"`
}

// SearchParams for the search method.
type SearchParams struct {
	Query           string `json:"query"`
	Project         string `json:"project,omitempty"`
	Source          string `json:"source,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	LimitPerSession int    `json:"limit_per_session,omitempty"`
	CaseSensitive   bool   `json:"case_sensitive,omitempty"`
	Regex           bool   `json:"regex,omitempty"`
}

// SemanticSearchParams for the semantic_search method.
type SemanticSearchParams struct {
	Query      string  `json:"query"`
	Project    string  `json:"project,omitempty"`
	Source     string  `json:"source,omitempty"`
	Limit      int     `json:"limit,omitempty"`
	MaxDistance float64 `json:"max_distance,omitempty"`
}

// StatusData returned by the status method.
type StatusData struct {
	State         string        `json:"state"` // "idle", "syncing", "embedding"
	SyncProgress  *ProgressInfo `json:"sync_progress,omitempty"`
	EmbedProgress *ProgressInfo `json:"embed_progress,omitempty"`
	Model         string        `json:"model"`
	ModelDim      int           `json:"model_dim"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	Watching      bool          `json:"watching"`
}

// ProgressInfo represents progress for a long-running operation.
type ProgressInfo struct {
	Done        int    `json:"done"`
	Total       int    `json:"total"`
	SessionID   string `json:"session_id,omitempty"`
	Message     string `json:"message,omitempty"`
	ChunksDone  int    `json:"chunks_done,omitempty"`
	ChunksTotal int    `json:"chunks_total,omitempty"`
	Entries     int    `json:"entries,omitempty"`
}
