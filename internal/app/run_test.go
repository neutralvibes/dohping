package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"dohping/internal/ping"
	"dohping/internal/state"
)

// blockingProbe blocks until ctx is cancelled — used to prove cancellation
// stops the loop promptly.
type blockingProbe struct{}

func (blockingProbe) Probe(ctx context.Context) ping.Result {
	<-ctx.Done()
	return ping.Result{Outcome: ping.OutcomeDown}
}
func (blockingProbe) Close() error { return nil }

// scriptedProbe returns a fixed sequence of results, then blocks.
type scriptedProbe struct {
	mu      sync.Mutex
	results []ping.Result
}

func (p *scriptedProbe) Probe(ctx context.Context) ping.Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.results) == 0 {
		<-ctx.Done()
		return ping.Result{Outcome: ping.OutcomeDown}
	}
	r := p.results[0]
	p.results = p.results[1:]
	return r
}

func (p *scriptedProbe) Close() error { return nil }

func TestRunCancellationStopsLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	eng := state.New(1, 1)
	done := make(chan struct{})
	go func() {
		Run(ctx, blockingProbe{}, eng, 10*time.Millisecond, 0, nil)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("loop returned before cancellation")
	case <-time.After(30 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
		// Clean return.
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return after cancellation (goroutine leak)")
	}
}

func TestRunCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eng := state.New(1, 1)
	events := make(chan state.Event, 32)
	done := make(chan struct{})
	go func() {
		Run(ctx, &scriptedProbe{results: []ping.Result{
			{Outcome: ping.OutcomeUp, RTT: time.Millisecond},
			{Outcome: ping.OutcomeUp, RTT: time.Millisecond},
			{Outcome: ping.OutcomeUp, RTT: time.Millisecond},
		}}, eng, 10*time.Millisecond, 3, events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not finish after 3 probes")
	}
	close(events)

	n := 0
	for range events {
		n++
	}
	if n != 3 {
		t.Errorf("events = %d, want 3", n)
	}
	if eng.Status() != state.StatusUp {
		t.Errorf("status = %v, want up", eng.Status())
	}
	if p, ok, fail := eng.Totals(); p != 3 || ok != 3 || fail != 0 {
		t.Errorf("totals = %d/%d/%d, want 3/3/0", p, ok, fail)
	}
}

func TestRunDropsPostCancellationResults(t *testing.T) {
	// A probe that returns a result only AFTER ctx is cancelled must not
	// feed the engine.
	ctx, cancel := context.WithCancel(context.Background())
	eng := state.New(1, 1)
	slow := &scriptedProbe{} // blocks forever
	done := make(chan struct{})
	go func() {
		Run(ctx, slow, eng, 10*time.Millisecond, 0, nil)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return")
	}
	if p, _, _ := eng.Totals(); p != 0 {
		t.Errorf("engine got %d probes after cancellation, want 0", p)
	}
}
