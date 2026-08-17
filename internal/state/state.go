// Package state implements the dohping status engine: a pure state machine
// that consumes probe results and emits semantic events.
//
// It knows nothing about terminals, colors, windows, or log files. It
// tracks the four statuses (unknown/up/down/error), per-state duration,
// RTT statistics, consecutive success/failure counts (hysteresis), and
// probe totals. The clock is injected as a func() time.Time so tests are
// deterministic.
package state

import (
	"time"

	"dohping/internal/ping"
)

// Status is the monitored status of the host.
type Status int

const (
	// StatusUnknown: no probe has established status yet.
	StatusUnknown Status = iota
	// StatusUp: the host is considered reachable.
	StatusUp
	// StatusDown: the host is considered unreachable.
	StatusDown
	// StatusError: an operational error prevents probing or interpretation.
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusUp:
		return "up"
	case StatusDown:
		return "down"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// Stats tracks RTT statistics for the current status. Zero value is an
// empty sample set (all fields blank/zero).
type Stats struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Sum   time.Duration
}

// Add folds one RTT sample into the statistics.
func (s *Stats) Add(rtt time.Duration) {
	if s.Count == 0 || rtt < s.Min {
		s.Min = rtt
	}
	if s.Count == 0 || rtt > s.Max {
		s.Max = rtt
	}
	s.Sum += rtt
	s.Count++
}

// Reset clears the statistics.
func (s *Stats) Reset() { *s = Stats{} }

// Avg returns the mean RTT, or 0 with no samples (division-by-zero safe).
func (s *Stats) Avg() time.Duration {
	if s.Count == 0 {
		return 0
	}
	return s.Sum / time.Duration(s.Count)
}

// EventKind identifies the type of a semantic event.
type EventKind int

const (
	// EventProbeSuccess: a probe proved reachability; RTT and stats updated.
	EventProbeSuccess EventKind = iota
	// EventProbeFailure: a probe timed out; consecutive-failure count updated.
	EventProbeFailure
	// EventProbeError: an operational-error probe while ALREADY in the
	// error state (no transition; the live line just updates).
	EventProbeError
	// EventStatusChange: the status transitioned; Duration/Stats/Fails
	// describe the state that just ended.
	EventStatusChange
	// EventError: an operational error occurred and the status became
	// StatusError.
	EventError
)

// Event is one semantic event emitted by the engine. Consumers (display,
// logging) render from these — the engine never formats anything.
type Event struct {
	Kind EventKind
	Time time.Time // wall-clock moment of the probe result (engine clock)

	PrevStatus Status // status before this event
	Status     Status // status after this event

	RTT time.Duration // EventProbeSuccess: this probe's RTT
	Err error         // EventError: the operational error

	Fails int // consecutive failures at event time

	// EventStatusChange / EventError: duration and stats of the state that
	// just ended (used to finalize the previous display line).
	Duration  time.Duration
	PrevStats Stats

	// Stats is the statistics of the CURRENT (post-event) state — for
	// probe events the running stats, for status changes the fresh stats
	// of the new state (its first sample included).
	Stats Stats
}

// Engine is the dohping status state machine.
type Engine struct {
	downAfter int
	upAfter   int

	status    Status
	start     time.Time // when the current status began (monotonic-safe via injected clock)
	fails     int       // consecutive failed probes
	successes int       // consecutive successful probes
	stats     Stats

	totalProbes int
	totalOK     int
	totalFail   int
}

// New creates an engine with the given hysteresis thresholds. Both must be
// >= 1 (validated by the CLI).
func New(downAfter, upAfter int) *Engine {
	return &Engine{downAfter: downAfter, upAfter: upAfter, status: StatusUnknown}
}

// Status returns the current status.
func (e *Engine) Status() Status { return e.status }

// Start returns when the current status began (engine clock).
func (e *Engine) Start() time.Time { return e.start }

// Fails returns the current consecutive-failure count.
func (e *Engine) Fails() int { return e.fails }

// Stats returns the current RTT statistics.
func (e *Engine) Stats() Stats { return e.stats }

// Totals returns aggregate probe counters for the exit summary.
func (e *Engine) Totals() (probes, ok, fail int) {
	return e.totalProbes, e.totalOK, e.totalFail
}

// Step applies one probe result at wall-clock time t and returns the event
// describing what happened. t is injected so tests are deterministic.
func (e *Engine) Step(res ping.Result, t time.Time) Event {
	// The very first probe anchors the initial (unknown) state's start.
	if e.totalProbes == 0 {
		e.start = t
	}
	e.totalProbes++

	ev := Event{Time: t, PrevStatus: e.status, Status: e.status}

	switch res.Outcome {
	case ping.OutcomeUp:
		e.totalOK++
		e.successes++
		oldFails := e.fails
		e.fails = 0
		// Transition FIRST so the new state's statistics start clean, then
		// fold this probe's RTT in as the first sample of the (new) up
		// state — the triggering success must not be lost.
		if e.status != StatusUp && e.successes >= e.upAfter {
			ev.Kind = EventStatusChange
			ev.Duration = t.Sub(e.start)
			ev.PrevStats = e.stats
			ev.Fails = oldFails // final failure count of the ended down state
			e.transition(StatusUp, t)
		}
		e.stats.Add(res.RTT)
		ev.RTT = res.RTT
		if ev.Kind != EventStatusChange {
			ev.Kind = EventProbeSuccess
		}
	case ping.OutcomeDown:
		e.totalFail++
		e.fails++
		e.successes = 0
		ev.Kind = EventProbeFailure
		ev.Fails = e.fails
		if e.status != StatusDown && e.fails >= e.downAfter {
			ev.Kind = EventStatusChange
			ev.Duration = t.Sub(e.start)
			ev.PrevStats = e.stats
			e.transition(StatusDown, t)
		}
	case ping.OutcomeError:
		if e.status != StatusError {
			// Enter the error state: finalize the previous state.
			ev.Kind = EventError
			ev.Err = res.Err
			ev.Duration = t.Sub(e.start)
			ev.PrevStats = e.stats
			ev.Fails = e.fails
			e.transition(StatusError, t)
		} else {
			// Already in error: no transition, no new status line — the
			// live line just keeps updating its duration.
			ev.Kind = EventProbeError
			ev.Err = res.Err
			ev.Fails = e.fails
		}
	}

	ev.Status = e.status
	ev.Stats = e.stats
	return ev
}

// transition moves to a new status: the previous state's duration and
// statistics are finalized (carried on the event by the caller), and the
// new state starts with clean statistics. The success/failure counters are
// owned by the hysteresis logic in Step, not reset here — a transition to
// up already zeroed fails, and a transition to down keeps the run that
// triggered it.
func (e *Engine) transition(s Status, t time.Time) {
	e.status = s
	e.start = t
	e.stats.Reset()
}
