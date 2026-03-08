package indexer

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
)

func TestSyncGateSharesInFlightResultAndProgress(t *testing.T) {
	var gate SyncGate

	started := make(chan struct{})
	release := make(chan struct{})
	progressSeen := make(chan string, 2)

	want := &rpc.Response{OK: true, Data: json.RawMessage(`{"run":1}`)}

	var wg sync.WaitGroup
	wg.Add(2)

	results := make(chan *rpc.Response, 2)
	errs := make(chan error, 2)

	go func() {
		defer wg.Done()
		resp, err := gate.Run(func(rpc.Progress) {
			progressSeen <- "first"
		}, func(broadcast func(rpc.Progress)) (*rpc.Response, error) {
			close(started)
			waitForSubscribers(t, &gate, 2)
			broadcast(rpc.Progress{Progress: true, Data: json.RawMessage(`{"step":1}`)})
			<-release
			return want, nil
		})
		results <- resp
		errs <- err
	}()

	<-started

	secondRan := make(chan struct{}, 1)
	go func() {
		defer wg.Done()
		resp, err := gate.Run(func(rpc.Progress) {
			progressSeen <- "second"
		}, func(func(rpc.Progress)) (*rpc.Response, error) {
			secondRan <- struct{}{}
			return &rpc.Response{OK: true, Data: json.RawMessage(`{"run":2}`)}, nil
		})
		results <- resp
		errs <- err
	}()
	waitForProgress(t, progressSeen, 2)

	close(release)
	wg.Wait()

	select {
	case <-secondRan:
		t.Fatal("second caller executed instead of joining the in-flight run")
	default:
	}

	for i := 0; i < 2; i++ {
		resp := <-results
		if resp != want {
			t.Fatalf("got response %p, want shared response %p", resp, want)
		}
		if err := <-errs; err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestSyncGateSequentialRunsStayIsolated(t *testing.T) {
	var gate SyncGate

	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	firstDone := make(chan struct{})

	firstResp := &rpc.Response{OK: true, Data: json.RawMessage(`{"run":1}`)}
	secondResp := &rpc.Response{OK: true, Data: json.RawMessage(`{"run":2}`)}

	waiterResult := make(chan *rpc.Response, 1)
	waiterErr := make(chan error, 1)

	go func() {
		resp, err := gate.Run(func(rpc.Progress) {}, func(func(rpc.Progress)) (*rpc.Response, error) {
			close(firstStarted)
			waitForSubscribers(t, &gate, 2)
			<-firstRelease
			defer close(firstDone)
			return firstResp, nil
		})
		if resp != firstResp || err != nil {
			t.Errorf("first run = (%p, %v), want (%p, nil)", resp, err, firstResp)
		}
	}()

	<-firstStarted

	go func() {
		resp, err := gate.Run(func(rpc.Progress) {}, func(func(rpc.Progress)) (*rpc.Response, error) {
			t.Error("waiter unexpectedly started a second run")
			return nil, nil
		})
		waiterResult <- resp
		waiterErr <- err
	}()

	close(firstRelease)
	<-firstDone

	resp, err := gate.Run(func(rpc.Progress) {}, func(func(rpc.Progress)) (*rpc.Response, error) {
		return secondResp, nil
	})
	if resp != secondResp || err != nil {
		t.Fatalf("second run = (%p, %v), want (%p, nil)", resp, err, secondResp)
	}

	select {
	case got := <-waiterResult:
		if got != firstResp {
			t.Fatalf("waiter got %p, want first response %p", got, firstResp)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for waiter result")
	}

	if err := <-waiterErr; err != nil {
		t.Fatalf("unexpected waiter error: %v", err)
	}
}

func waitForSubscribers(t *testing.T, gate *SyncGate, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gate.stateMu.Lock()
		run := gate.run
		gate.stateMu.Unlock()
		if run != nil {
			run.subsMu.Lock()
			got := len(run.subs)
			run.subsMu.Unlock()
			if got == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %d subscribers", want)
}

func waitForProgress(t *testing.T, ch <-chan string, want int) {
	t.Helper()

	for i := 0; i < want; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for shared progress")
		}
	}
}

func TestSyncGateWaitIdle(t *testing.T) {
	var gate SyncGate

	started := make(chan struct{})
	release := make(chan struct{})
	waitReturned := make(chan struct{})

	go func() {
		_, _ = gate.Run(func(rpc.Progress) {}, func(func(rpc.Progress)) (*rpc.Response, error) {
			close(started)
			<-release
			return &rpc.Response{OK: true}, nil
		})
	}()

	<-started

	go func() {
		gate.WaitIdle()
		close(waitReturned)
	}()

	select {
	case <-waitReturned:
		t.Fatal("WaitIdle returned while a run was still active")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case <-waitReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitIdle did not return after the run completed")
	}
}
