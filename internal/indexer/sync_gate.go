package indexer

import (
	"sync"
	"sync/atomic"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

// SyncGate coordinates a single-flight long-running operation with progress
// fan-out. If a second caller arrives while the operation is running, it
// subscribes to the progress stream and waits for the same result.
type SyncGate struct {
	mu     sync.Mutex
	doneMu sync.Mutex // guards done, result, err reads/writes
	subsMu sync.Mutex
	subs   []rpcSubscriber
	done   chan struct{}
	result *rpc.Response
	err    error
}

type rpcSubscriber struct {
	id uint64
	fn func(rpc.Progress)
}

var nextSubID atomic.Uint64

// Run attempts to execute fn. If another Run is already in progress, the
// caller subscribes to its progress and blocks until it completes, returning
// the same result. Only one fn runs at a time.
func (g *SyncGate) Run(send func(rpc.Progress), fn func(broadcast func(rpc.Progress)) (*rpc.Response, error)) (*rpc.Response, error) {
	if !g.mu.TryLock() {
		remove := g.addSubscriber(send)
		defer remove()
		// Snapshot done under doneMu so we always wait on the
		// current run's channel, not a stale closed one.
		g.doneMu.Lock()
		ch := g.done
		g.doneMu.Unlock()
		<-ch
		g.doneMu.Lock()
		r, e := g.result, g.err
		g.doneMu.Unlock()
		return r, e
	}
	defer g.mu.Unlock()

	// Initialize done channel under doneMu before any subscriber can see it.
	g.doneMu.Lock()
	g.done = make(chan struct{})
	g.doneMu.Unlock()

	remove := g.addSubscriber(send)

	defer func() {
		remove()
		// Close done after setting result/err, all under doneMu,
		// so subscribers see consistent state.
		g.doneMu.Lock()
		close(g.done)
		g.doneMu.Unlock()
	}()

	resp, err := fn(g.Broadcast)
	g.doneMu.Lock()
	g.result, g.err = resp, err
	g.doneMu.Unlock()
	return resp, err
}

// WaitIdle blocks until no Run is in progress.
func (g *SyncGate) WaitIdle() {
	g.mu.Lock()
	g.mu.Unlock() //nolint:SA2001 // intentional barrier
}

// Broadcast sends a progress event to all subscribers.
func (g *SyncGate) Broadcast(p rpc.Progress) {
	g.subsMu.Lock()
	snapshot := make([]rpcSubscriber, len(g.subs))
	copy(snapshot, g.subs)
	g.subsMu.Unlock()

	for _, sub := range snapshot {
		sub.fn(p)
	}
}

func (g *SyncGate) addSubscriber(fn func(rpc.Progress)) func() {
	g.subsMu.Lock()
	id := nextSubID.Add(1)
	g.subs = append(g.subs, rpcSubscriber{id: id, fn: fn})
	g.subsMu.Unlock()

	return func() {
		g.subsMu.Lock()
		defer g.subsMu.Unlock()
		for i, sub := range g.subs {
			if sub.id == id {
				g.subs = append(g.subs[:i], g.subs[i+1:]...)
				break
			}
		}
	}
}
