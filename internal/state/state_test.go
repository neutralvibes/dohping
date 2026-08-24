package state

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"dohping/internal/ping"
)

// Fake clock helpers: deterministic timestamps for engine tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func up(rtt time.Duration) ping.Result { return ping.Result{Outcome: ping.OutcomeUp, RTT: rtt} }
func down() ping.Result                { return ping.Result{Outcome: ping.OutcomeDown} }
func operr() ping.Result               { return ping.Result{Outcome: ping.OutcomeError, Err: errors.New("boom")} }

// feed runs a sequence of results through the engine at advancing clock
// times, returning the events.
func feed(t *testing.T, eng *Engine, clock *fakeClock, results ...ping.Result) []Event {
	t.Helper()
	var events []Event
	for _, r := range results {
		clock.advance(100 * time.Millisecond)
		events = append(events, eng.Step(r, clock.t))
	}
	return events
}

func TestFirstSuccessBecomesUp(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	ev := eng.Step(up(10*time.Millisecond), clock.now())
	if ev.Status != StatusUp {
		t.Fatalf("status = %v, want up", ev.Status)
	}
	if ev.Kind != EventStatusChange {
		t.Errorf("kind = %v, want EventStatusChange", ev.Kind)
	}
	if ev.PrevStatus != StatusUnknown {
		t.Errorf("prev = %v, want unknown", ev.PrevStatus)
	}
}

func TestFirstFailureBecomesDown(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	ev := eng.Step(down(), clock.now())
	if ev.Status != StatusDown {
		t.Fatalf("status = %v, want down", ev.Status)
	}
	if ev.Fails != 1 {
		t.Errorf("fails = %d, want 1", ev.Fails)
	}
}

func TestDownThreshold(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(3, 1) // --down-after 3
	events := feed(t, eng, clock,
		down(), down(), up(1*time.Millisecond), down(), down(), down())

	// First two failures must not flip to down.
	if events[0].Status != StatusUnknown {
		t.Errorf("after fail#1 status = %v, want unknown", events[0].Status)
	}
	if events[1].Status != StatusUnknown {
		t.Errorf("after fail#2 status = %v, want unknown", events[1].Status)
	}
	// Success resets failure progression and flips to up (up-after 1).
	if events[2].Status != StatusUp {
		t.Errorf("after success status = %v, want up", events[2].Status)
	}
	// fail, fail → still up.
	if events[3].Status != StatusUp || events[4].Status != StatusUp {
		t.Errorf("after fail#4/#5 status = %v/%v, want up/up", events[3].Status, events[4].Status)
	}
	// Third consecutive failure flips to down.
	if events[5].Status != StatusDown {
		t.Errorf("after fail#6 status = %v, want down", events[5].Status)
	}
}

func TestUpThreshold(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 2) // --up-after 2
	// Get into down first.
	eng.Step(down(), clock.now())
	events := feed(t, eng, clock,
		up(1*time.Millisecond), down(), up(2*time.Millisecond), up(3*time.Millisecond))

	// First success alone must not flip to up.
	if events[0].Status != StatusDown {
		t.Errorf("after success#1 status = %v, want down", events[0].Status)
	}
	// Failure resets success progression.
	if events[1].Status != StatusDown {
		t.Errorf("after fail status = %v, want down", events[1].Status)
	}
	// success, success → up.
	if events[3].Status != StatusUp {
		t.Errorf("after success#3 status = %v, want up", events[3].Status)
	}
	if events[3].Kind != EventStatusChange {
		t.Errorf("kind = %v, want EventStatusChange", events[3].Kind)
	}
}

func TestRTTStatistics(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	for _, ms := range []int64{1, 2, 3} {
		eng.Step(up(time.Duration(ms)*time.Millisecond), clock.now())
	}
	s := eng.Stats()
	if s.Count != 3 {
		t.Errorf("count = %d, want 3", s.Count)
	}
	if s.Min != 1*time.Millisecond || s.Max != 3*time.Millisecond {
		t.Errorf("min/max = %v/%v, want 1ms/3ms", s.Min, s.Max)
	}
	if got := s.Avg(); got != 2*time.Millisecond {
		t.Errorf("avg = %v, want 2ms", got)
	}
}

func TestStatsResetOnStatusChange(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	for _, ms := range []int64{5, 7} {
		eng.Step(up(time.Duration(ms)*time.Millisecond), clock.now())
	}
	if eng.Stats().Count != 2 {
		t.Fatalf("stats count = %d, want 2", eng.Stats().Count)
	}
	// Flip down, then back up.
	eng.Step(down(), clock.now())
	if eng.Stats().Count != 0 {
		t.Errorf("stats not reset on down: count = %d", eng.Stats().Count)
	}
	eng.Step(up(9*time.Millisecond), clock.now())
	s := eng.Stats()
	if s.Count != 1 || s.Min != 9*time.Millisecond || s.Max != 9*time.Millisecond {
		t.Errorf("new up state stats = %+v, want clean single-sample {9ms,9ms}", s)
	}
}

func TestFailureCountIncrementsWhileDown(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	eng.Step(down(), clock.now())
	if eng.Fails() != 1 {
		t.Fatalf("fails = %d, want 1", eng.Fails())
	}
	for i := 2; i <= 5; i++ {
		ev := eng.Step(down(), clock.now())
		if ev.Fails != i {
			t.Errorf("event fails = %d, want %d", ev.Fails, i)
		}
		if eng.Fails() != i {
			t.Errorf("engine fails = %d, want %d", eng.Fails(), i)
		}
		if ev.Status != StatusDown {
			t.Errorf("status = %v, want down", ev.Status)
		}
	}
}

func TestDurationPerState(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	// T0: up begins.
	t0 := clock.t
	eng.Step(up(1*time.Millisecond), t0)
	if eng.Start() != t0 {
		t.Errorf("up start = %v, want %v", eng.Start(), t0)
	}
	// Advance 30s, stay up.
	clock.advance(30 * time.Second)
	eng.Step(up(1*time.Millisecond), clock.t)
	if got := clock.t.Sub(eng.Start()); got != 30*time.Second {
		t.Errorf("up duration = %v, want 30s", got)
	}
	// Flip to down at T0+30s: previous duration finalized on the event.
	ev := eng.Step(down(), clock.t)
	if ev.Kind != EventStatusChange {
		t.Fatalf("kind = %v, want EventStatusChange", ev.Kind)
	}
	if ev.Duration != 30*time.Second {
		t.Errorf("finalized duration = %v, want 30s", ev.Duration)
	}
	if ev.PrevStats.Count != 2 {
		t.Errorf("finalized stats count = %d, want 2 (two up probes)", ev.PrevStats.Count)
	}
	// New down state starts at the transition time.
	if eng.Start() != clock.t {
		t.Errorf("down start = %v, want %v", eng.Start(), clock.t)
	}
	// Advance 5s in down; duration reflects the down state only.
	clock.advance(5 * time.Second)
	eng.Step(down(), clock.t)
	if got := clock.t.Sub(eng.Start()); got != 5*time.Second {
		t.Errorf("down duration = %v, want 5s", got)
	}
}

func TestErrorState(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	eng.Step(up(1*time.Millisecond), clock.now())
	ev := eng.Step(operr(), clock.now())
	if ev.Kind != EventError {
		t.Fatalf("kind = %v, want EventError", ev.Kind)
	}
	if ev.Status != StatusError {
		t.Errorf("status = %v, want error", ev.Status)
	}
	if ev.Err == nil {
		t.Error("err = nil, want the operational error")
	}
	if eng.Status() != StatusError {
		t.Errorf("engine status = %v, want error", eng.Status())
	}
	// Recovery: a success flips back to up.
	ev = eng.Step(up(2*time.Millisecond), clock.now())
	if ev.Status != StatusUp {
		t.Errorf("after recovery status = %v, want up", ev.Status)
	}
}

// TestErrorStateNoRepeatLines: consecutive error probes must NOT re-enter
// the error state — one status line, duration updating in place (the
// regression behind the user's repeated-error-lines report).
func TestErrorStateNoRepeatLines(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	eng.Step(up(1*time.Millisecond), clock.now()) // up
	first := eng.Step(operr(), clock.now())       // → error (transition)
	if first.Kind != EventError {
		t.Fatalf("first error kind = %v, want EventError", first.Kind)
	}
	start := eng.Start()
	for i := 0; i < 5; i++ {
		clock.advance(time.Second)
		ev := eng.Step(operr(), clock.now())
		if ev.Kind != EventProbeError {
			t.Fatalf("error #%d kind = %v, want EventProbeError", i+2, ev.Kind)
		}
		if ev.Status != StatusError {
			t.Errorf("error #%d status = %v, want error", i+2, ev.Status)
		}
	}
	// No re-transition: the state start must still be the FIRST error.
	if !eng.Start().Equal(start) {
		t.Errorf("state start moved on consecutive errors: %v → %v", start, eng.Start())
	}
	// Duration grows from the first error.
	if got := clock.t.Sub(eng.Start()); got != 5*time.Second {
		t.Errorf("error-state duration = %v, want 5s", got)
	}
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{
		StatusUnknown: "unknown",
		StatusUp:      "up",
		StatusDown:    "down",
		StatusError:   "error",
	} {
		if got := s.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestStatsZeroSamples(t *testing.T) {
	var s Stats
	if s.Avg() != 0 {
		t.Errorf("avg of empty stats = %v, want 0", s.Avg())
	}
	if s.Min != 0 || s.Max != 0 {
		t.Errorf("min/max of empty stats = %v/%v, want 0/0", s.Min, s.Max)
	}
}

func TestStatsSingleSample(t *testing.T) {
	var s Stats
	s.Add(42 * time.Millisecond)
	if s.Min != 42*time.Millisecond || s.Max != 42*time.Millisecond || s.Avg() != 42*time.Millisecond {
		t.Errorf("single sample stats = %+v", s)
	}
}

func TestTotals(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(1, 1)
	eng.Step(up(1*time.Millisecond), clock.now())
	eng.Step(up(2*time.Millisecond), clock.now())
	eng.Step(down(), clock.now())
	eng.Step(down(), clock.now())
	eng.Step(operr(), clock.now())
	p, ok, fail := eng.Totals()
	if p != 5 || ok != 2 || fail != 2 {
		t.Errorf("totals = %d/%d/%d, want 5/2/2 (error probe counted, not ok/fail)", p, ok, fail)
	}
}

func TestDownAfterEventCarriesFinalStats(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(2, 1)
	eng.Step(up(1*time.Millisecond), clock.now())
	eng.Step(up(2*time.Millisecond), clock.now())
	ev := eng.Step(down(), clock.now()) // fails=1, still up
	if ev.Kind != EventProbeFailure {
		t.Fatalf("kind = %v, want EventProbeFailure", ev.Kind)
	}
	ev = eng.Step(down(), clock.now()) // fails=2 → down
	if ev.Kind != EventStatusChange {
		t.Fatalf("kind = %v, want EventStatusChange", ev.Kind)
	}
	if ev.PrevStats.Count != 2 || ev.PrevStats.Min != 1*time.Millisecond || ev.PrevStats.Max != 2*time.Millisecond {
		t.Errorf("finalized up stats = %+v, want count=2 min=1ms max=2ms", ev.PrevStats)
	}
	if ev.Fails != 2 {
		t.Errorf("fails on down transition = %d, want 2", ev.Fails)
	}
}

// Demonstrates the engine never panics on any input shape.
func TestNoPanicOnWeirdResults(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	eng := New(5, 5)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	for i := 0; i < 50; i++ {
		clock.advance(time.Second)
		switch i % 4 {
		case 0:
			eng.Step(up(1*time.Millisecond), clock.t)
		case 1:
			eng.Step(down(), clock.t)
		case 2:
			eng.Step(operr(), clock.t)
		case 3:
			eng.Step(ping.Result{Outcome: ping.Outcome(99), Err: fmt.Errorf("unknown outcome")}, clock.t)
		}
	}
}
