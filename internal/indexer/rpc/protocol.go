package rpc

import (
	"encoding/json"
	"fmt"

	"github.com/wethinkt/go-thinkt/internal/indexer/search"
)

// RPC method names.
const (
	MethodIndexSync      = "index_sync"
	MethodEmbedSync      = "embed_sync"
	MethodSearch         = "search"
	MethodSemanticSearch = "semantic_search"
	MethodStats          = "stats"
	MethodStatus         = "status"
	MethodConfigReload   = "config_reload"
)

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

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

// OKResponse marshals v into a successful Response.
func OKResponse(v any) (*Response, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal response data: %w", err)
	}
	return &Response{OK: true, Data: data}, nil
}

// Progress is a streaming progress update.
type Progress struct {
	Progress bool            `json:"progress"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// ProgressFrom marshals v into a Progress message.
func ProgressFrom(v any) Progress {
	data, _ := json.Marshal(v)
	return Progress{Data: data}
}

// ---------------------------------------------------------------------------
// Request params
// ---------------------------------------------------------------------------

// SyncParams for the index_sync method.
type SyncParams struct {
	Force bool `json:"force,omitempty"`
}

// EmbedSyncParams for the embed_sync method.
type EmbedSyncParams struct {
	Force bool `json:"force,omitempty"` // re-embed everything
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
	Query       string  `json:"query"`
	Project     string  `json:"project,omitempty"`
	Source      string  `json:"source,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	MaxDistance float64 `json:"max_distance,omitempty"`
	Diversity   bool    `json:"diversity,omitempty"`
	Tier        string  `json:"tier,omitempty"`
}

// ---------------------------------------------------------------------------
// Response data (one per method)
// ---------------------------------------------------------------------------

// SearchData is the response payload for MethodSearch.
type SearchData struct {
	Results      []search.SessionResult `json:"results"`
	TotalMatches int                    `json:"total_matches"`
}

// SemanticSearchData is the response payload for MethodSemanticSearch.
type SemanticSearchData struct {
	Results []search.SemanticResult `json:"results"`
}

// StatsData is the response payload for MethodStats.
type StatsData struct {
	TotalProjects   int         `json:"total_projects"`
	TotalSessions   int         `json:"total_sessions"`
	TotalEntries    int         `json:"total_entries"`
	TotalTokens     int         `json:"total_tokens"`
	TotalEmbeddings int         `json:"total_embeddings"`
	EmbedModel      string      `json:"embed_model"`
	TopTools        []ToolCount `json:"top_tools"`
}

// ToolCount is a tool name and its usage count, used in StatsData.
type ToolCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// StatusData is the response payload for MethodStatus.
type StatusData struct {
	Syncing       bool          `json:"syncing"`
	Embedding     bool          `json:"embedding"`
	State         string        `json:"state"` // "idle", "syncing", "embedding", "syncing+embedding"
	SyncProgress  *ProgressInfo `json:"sync_progress,omitempty"`
	EmbedProgress *ProgressInfo `json:"embed_progress,omitempty"`
	Model         string        `json:"model"`
	ModelDim      int           `json:"model_dim"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	Watching      bool          `json:"watching"`
}

// SyncData is the response payload for MethodIndexSync.
type SyncData struct {
	Projects int `json:"projects"`
}

// ConfigReloadData is the response payload for MethodConfigReload.
type ConfigReloadData struct {
	EmbeddingEnabled bool `json:"embedding_enabled"`
	ModelChanged     bool `json:"model_changed,omitempty"`
}

// ---------------------------------------------------------------------------
// Progress data (streaming updates during long operations)
// ---------------------------------------------------------------------------

// ProgressInfo represents progress for a long-running operation (used in StatusData).
type ProgressInfo struct {
	Done         int    `json:"done"`
	Total        int    `json:"total"`
	SessionID    string `json:"session_id,omitempty"`
	Message      string `json:"message,omitempty"`
	Project      int    `json:"project,omitempty"`
	ProjectTotal int    `json:"project_total,omitempty"`
	ProjectName  string `json:"project_name,omitempty"`
	ChunksDone   int    `json:"chunks_done,omitempty"`
	ChunksTotal  int    `json:"chunks_total,omitempty"`
	Entries      int    `json:"entries,omitempty"`
}

// SyncProgressData is sent during index sync to report per-project progress.
type SyncProgressData struct {
	Project      int    `json:"project"`
	ProjectTotal int    `json:"project_total"`
	Session      int    `json:"session"`
	SessionTotal int    `json:"session_total"`
	Message      string `json:"message"`
}

// EmbedProgressData is sent during embed sync to report per-session progress.
type EmbedProgressData struct {
	Done        int    `json:"done"`
	Total       int    `json:"total"`
	Chunks      int    `json:"chunks"`
	Entries     int    `json:"entries"`
	SessionID   string `json:"session_id"`
	SessionPath string `json:"session_path"`
	ElapsedMs   int64  `json:"elapsed_ms"`
}

// EmbedChunkProgressData is sent during embed sync to report chunk-level progress.
type EmbedChunkProgressData struct {
	ChunksDone  int    `json:"chunks_done"`
	ChunksTotal int    `json:"chunks_total"`
	TokensDone  int    `json:"tokens_done"`
	SessionID   string `json:"session_id"`
}

// ModelDownloadProgressData is sent during embedding model download.
type ModelDownloadProgressData struct {
	ModelDownload bool    `json:"model_download"`
	Downloaded    int64   `json:"downloaded"`
	Total         int64   `json:"total"`
	Percent       float64 `json:"percent"`
}
