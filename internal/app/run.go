package app

import (
	"context"
	"time"

	"dohping/internal/ping"
	"dohping/internal/state"
)

// Run probes on a fixed-rate cadence (interval between probe starts, never
// overlapping — a probe that outlives the interval delays the next start)
// and feeds results into the state engine. Events are delivered to events
// if non-nil. Returns when ctx is cancelled, when count probes have run
// (count > 0), or when the probe loop stops.
func Run(ctx context.Context, pr ping.Probe, eng *state.Engine, interval time.Duration, count int, events chan<- state.Event) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	n := count
	for {
		res := pr.Probe(ctx)
		if ctx.Err() != nil {
			// Shutdown in progress — drop results after cancellation.
			return
		}
		ev := eng.Step(res, time.Now())
		if events != nil {
			events <- ev
		}
		if n > 0 {
			n--
			if n == 0 {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
