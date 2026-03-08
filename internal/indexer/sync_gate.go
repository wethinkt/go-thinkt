package indexer

import (
	"sync"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

// SyncGate coordinates a single-flight long-running operation with progress
// fan-out. If a second caller arrives while the operation is running, it
// subscribes to the progress stream and waits for the same result.
type SyncGate struct {
	mu     sync.Mutex
	subsMu sync.Mutex
	subs   []rpcSubscriber
	done   chan struct{}
	result *rpc.Response
	err    error
}

type rpcSubscriber struct {
	id int
	fn func(rpc.Progress)
}

var nextSubID int

// Run attempts to execute fn. If another Run is already in progress, the
// caller subscribes to its progress and blocks until it completes, returning
// the same result. Only one fn runs at a time.
func (g *SyncGate) Run(send func(rpc.Progress), fn func(broadcast func(rpc.Progress)) (*rpc.Response, error)) (*rpc.Response, error) {
	if !g.mu.TryLock() {
		remove := g.addSubscriber(send)
		defer remove()
		<-g.done
		return g.result, g.err
	}
	defer g.mu.Unlock()

	g.done = make(chan struct{})
	remove := g.addSubscriber(send)

	defer func() {
		remove()
		close(g.done)
	}()

	g.result, g.err = fn(g.Broadcast)
	return g.result, g.err
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
	nextSubID++
	id := nextSubID
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
