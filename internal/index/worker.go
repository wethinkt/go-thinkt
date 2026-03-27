package index

import (
	"context"

	"github.com/wethinkt/go-thinkt/internal/index/llm"
	"github.com/wethinkt/go-thinkt/internal/tuilog"
)

// Worker manages background embed and summarize goroutines.
type Worker struct {
	embedQueue  chan string
	sumQueue    chan string
	status      *StatusTracker
	gpuMu       *llm.GPUMutex
	embedFn     func(ctx context.Context, sessionID string) error
	summarizeFn func(ctx context.Context, sessionID string) error
}

type WorkerConfig struct {
	EmbedQueueSize int
	SumQueueSize   int
	GPUMutex       *llm.GPUMutex
	Status         *StatusTracker
	EmbedFn        func(ctx context.Context, sessionID string) error
	SummarizeFn    func(ctx context.Context, sessionID string) error
}

func NewWorker(cfg WorkerConfig) *Worker {
	embedSize := cfg.EmbedQueueSize
	if embedSize <= 0 {
		embedSize = 1000
	}
	sumSize := cfg.SumQueueSize
	if sumSize <= 0 {
		sumSize = 1000
	}
	gpuMu := cfg.GPUMutex
	if gpuMu == nil {
		gpuMu = &llm.GPUMutex{}
	}
	status := cfg.Status
	if status == nil {
		status = NewStatusTracker()
	}
	return &Worker{
		embedQueue: make(chan string, embedSize), sumQueue: make(chan string, sumSize),
		status: status, gpuMu: gpuMu, embedFn: cfg.EmbedFn, summarizeFn: cfg.SummarizeFn,
	}
}

func (w *Worker) Start(ctx context.Context) {
	go w.embedLoop(ctx)
	go w.sumLoop(ctx)
}

func (w *Worker) QueueEmbed(sessionID string) {
	select {
	case w.embedQueue <- sessionID:
	default:
		tuilog.Log.Warn("worker: embed queue full, dropping", "session_id", sessionID)
	}
}

func (w *Worker) QueueSummarize(sessionID string) {
	select {
	case w.sumQueue <- sessionID:
	default:
		tuilog.Log.Warn("worker: summarize queue full, dropping", "session_id", sessionID)
	}
}

func (w *Worker) Status() *StatusTracker { return w.status }

func (w *Worker) embedLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sessionID := <-w.embedQueue:
			w.status.SetQueueLens(len(w.embedQueue), len(w.sumQueue))
			w.status.SetEmbedding(true)
			if w.embedFn != nil {
				w.gpuMu.Lock()
				err := w.embedFn(ctx, sessionID)
				w.gpuMu.Unlock()
				if err != nil {
					tuilog.Log.Error("worker: embed failed", "session_id", sessionID, "error", err)
				} else {
					w.QueueSummarize(sessionID)
				}
			}
			w.status.SetEmbedding(false)
			w.status.SetQueueLens(len(w.embedQueue), len(w.sumQueue))
		}
	}
}

func (w *Worker) sumLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case sessionID := <-w.sumQueue:
			w.status.SetQueueLens(len(w.embedQueue), len(w.sumQueue))
			w.status.SetSummarizing(true)
			if w.summarizeFn != nil {
				w.gpuMu.Lock()
				err := w.summarizeFn(ctx, sessionID)
				w.gpuMu.Unlock()
				if err != nil {
					tuilog.Log.Error("worker: summarize failed", "session_id", sessionID, "error", err)
				}
			}
			w.status.SetSummarizing(false)
			w.status.SetQueueLens(len(w.embedQueue), len(w.sumQueue))
		}
	}
}
