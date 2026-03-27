package index

import (
	"sync"
	"time"
)

// StatusSnapshot is an immutable snapshot of the current worker state.
type StatusSnapshot struct {
	State       string `json:"state"`
	Syncing     bool   `json:"syncing"`
	Embedding   bool   `json:"embedding"`
	Summarizing bool   `json:"summarizing"`

	EmbedDone      int    `json:"embed_done"`
	EmbedTotal     int    `json:"embed_total"`
	EmbedSessionID string `json:"embed_session_id,omitempty"`

	SumDone      int    `json:"sum_done"`
	SumTotal     int    `json:"sum_total"`
	SumSessionID string `json:"sum_session_id,omitempty"`

	Model    string `json:"model,omitempty"`
	ModelDim int    `json:"model_dim,omitempty"`

	EmbedQueueLen int `json:"embed_queue_len"`
	SumQueueLen   int `json:"sum_queue_len"`

	UptimeSeconds int64 `json:"uptime_seconds"`
}

// StatusTracker provides thread-safe status updates and snapshots.
type StatusTracker struct {
	mu          sync.RWMutex
	syncing     bool
	embedding   bool
	summarizing bool
	startedAt   time.Time

	embedDone, embedTotal int
	embedSessionID        string
	sumDone, sumTotal     int
	sumSessionID          string
	model                 string
	modelDim              int
	embedQueueLen         int
	sumQueueLen           int
}

func NewStatusTracker() *StatusTracker {
	return &StatusTracker{startedAt: time.Now()}
}

func (s *StatusTracker) Snapshot() StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := "idle"
	var parts []string
	if s.syncing {
		parts = append(parts, "syncing")
	}
	if s.embedding {
		parts = append(parts, "embedding")
	}
	if s.summarizing {
		parts = append(parts, "summarizing")
	}
	if len(parts) > 0 {
		state = parts[0]
		for _, p := range parts[1:] {
			state += "+" + p
		}
	}

	return StatusSnapshot{
		State: state, Syncing: s.syncing, Embedding: s.embedding, Summarizing: s.summarizing,
		EmbedDone: s.embedDone, EmbedTotal: s.embedTotal, EmbedSessionID: s.embedSessionID,
		SumDone: s.sumDone, SumTotal: s.sumTotal, SumSessionID: s.sumSessionID,
		Model: s.model, ModelDim: s.modelDim,
		EmbedQueueLen: s.embedQueueLen, SumQueueLen: s.sumQueueLen,
		UptimeSeconds: int64(time.Since(s.startedAt).Seconds()),
	}
}

func (s *StatusTracker) SetSyncing(v bool)    { s.mu.Lock(); s.syncing = v; s.mu.Unlock() }
func (s *StatusTracker) SetEmbedding(v bool)  { s.mu.Lock(); s.embedding = v; s.mu.Unlock() }
func (s *StatusTracker) SetSummarizing(v bool) { s.mu.Lock(); s.summarizing = v; s.mu.Unlock() }

func (s *StatusTracker) SetEmbedProgress(done, total int, sessionID string) {
	s.mu.Lock(); s.embedDone = done; s.embedTotal = total; s.embedSessionID = sessionID; s.mu.Unlock()
}
func (s *StatusTracker) SetSumProgress(done, total int, sessionID string) {
	s.mu.Lock(); s.sumDone = done; s.sumTotal = total; s.sumSessionID = sessionID; s.mu.Unlock()
}
func (s *StatusTracker) SetModel(model string, dim int) {
	s.mu.Lock(); s.model = model; s.modelDim = dim; s.mu.Unlock()
}
func (s *StatusTracker) SetQueueLens(embed, sum int) {
	s.mu.Lock(); s.embedQueueLen = embed; s.sumQueueLen = sum; s.mu.Unlock()
}
