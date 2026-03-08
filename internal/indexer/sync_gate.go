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
	stateMu sync.Mutex
	run     *syncGateRun
}

type syncGateRun struct {
	done   chan struct{}
	subsMu sync.Mutex
	subs   []rpcSubscriber
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
	g.stateMu.Lock()
	if run := g.run; run != nil {
		remove := run.addSubscriber(send)
		g.stateMu.Unlock()
		defer remove()

		<-run.done
		return run.result, run.err
	}

	run := &syncGateRun{done: make(chan struct{})}
	g.run = run
	remove := run.addSubscriber(send)
	g.stateMu.Unlock()

	defer remove()

	resp, err := fn(run.Broadcast)

	g.stateMu.Lock()
	run.result, run.err = resp, err
	close(run.done)
	if g.run == run {
		g.run = nil
	}
	g.stateMu.Unlock()

	return resp, err
}

// WaitIdle blocks until no Run is in progress.
func (g *SyncGate) WaitIdle() {
	for {
		g.stateMu.Lock()
		run := g.run
		g.stateMu.Unlock()
		if run == nil {
			return
		}
		<-run.done
	}
}

// Broadcast sends a progress event to all subscribers.
func (r *syncGateRun) Broadcast(p rpc.Progress) {
	r.subsMu.Lock()
	snapshot := make([]rpcSubscriber, len(r.subs))
	copy(snapshot, r.subs)
	r.subsMu.Unlock()

	for _, sub := range snapshot {
		sub.fn(p)
	}
}

func (r *syncGateRun) addSubscriber(fn func(rpc.Progress)) func() {
	r.subsMu.Lock()
	id := nextSubID.Add(1)
	r.subs = append(r.subs, rpcSubscriber{id: id, fn: fn})
	r.subsMu.Unlock()

	return func() {
		r.subsMu.Lock()
		defer r.subsMu.Unlock()
		for i, sub := range r.subs {
			if sub.id == id {
				r.subs = append(r.subs[:i], r.subs[i+1:]...)
				break
			}
		}
	}
}
