package index

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerProcessesEmbedQueue(t *testing.T) {
	var processed atomic.Int32
	w := NewWorker(WorkerConfig{
		EmbedFn: func(ctx context.Context, sessionID string) error {
			processed.Add(1)
			return nil
		},
		SummarizeFn: func(ctx context.Context, sessionID string) error {
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.QueueEmbed("sess-1")
	w.QueueEmbed("sess-2")

	deadline := time.After(2 * time.Second)
	for processed.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out, only processed %d", processed.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestWorkerStopsOnContextCancel(t *testing.T) {
	w := NewWorker(WorkerConfig{
		EmbedFn:     func(ctx context.Context, sessionID string) error { return nil },
		SummarizeFn: func(ctx context.Context, sessionID string) error { return nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	cancel()
	time.Sleep(100 * time.Millisecond)
}
