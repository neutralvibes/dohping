package app

import (
	"context"
	"net"
	"testing"
	"time"

	"dohping/internal/ping"
	"dohping/internal/state"
)

// TestRunLiveTCPProbe exercises the real pipeline — TCP probe against a
// loopback listener, engine, event stream — end to end.
func TestRunLiveTCPProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	pr, err := ping.NewTCPProbe("127.0.0.1", ln.Addr().(*net.TCPAddr).Port, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eng := state.New(1, 1)
	events := make(chan state.Event, 16)

	done := make(chan struct{})
	go func() {
		Run(ctx, pr, eng, 20*time.Millisecond, 4, events)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("live run did not finish after 4 probes")
	}
	close(events)

	statusChanges := 0
	probeEvents := 0
	for ev := range events {
		if ev.Kind == state.EventStatusChange {
			statusChanges++
		}
		if ev.Kind == state.EventProbeSuccess {
			probeEvents++
		}
	}
	// 4 successful probes: 1 status change (unknown→up) + 3 probe-success
	// events (the first success is folded into the status change).
	if statusChanges != 1 {
		t.Errorf("status changes = %d, want 1", statusChanges)
	}
	if probeEvents != 3 {
		t.Errorf("probe-success events = %d, want 3", probeEvents)
	}
	if eng.Status() != state.StatusUp {
		t.Errorf("final status = %v, want up", eng.Status())
	}
	if s := eng.Stats(); s.Count != 4 {
		t.Errorf("stats count = %d, want 4", s.Count)
	}
}
