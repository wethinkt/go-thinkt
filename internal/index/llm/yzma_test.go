package llm

import (
	"sync"
	"testing"
	"time"
)

func TestGPUMutexSerializes(t *testing.T) {
	mu := &GPUMutex{}
	var order []int
	var orderMu sync.Mutex
	var wg sync.WaitGroup

	for i := range 3 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			orderMu.Lock()
			order = append(order, n)
			orderMu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}(i)
	}

	wg.Wait()
	if len(order) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(order))
	}
}

func TestAutoProcessor(t *testing.T) {
	proc := AutoProcessor()
	if proc == "" {
		t.Fatal("expected non-empty processor")
	}
	t.Logf("detected processor: %s", proc)
}
