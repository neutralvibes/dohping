package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"dohping/internal/state"
)

// fake events: a status change and a probe event.
func changeEvent(t time.Time, st state.Status) state.Event {
	return state.Event{Kind: state.EventStatusChange, Time: t, Status: st, PrevStatus: state.StatusUnknown}
}

func successEvent(t time.Time, st state.Status, stats state.Stats, fails int) state.Event {
	return state.Event{Kind: state.EventProbeSuccess, Time: t, Status: st, Stats: stats, Fails: fails}
}

func newTestDisplay(w *bytes.Buffer, quiet, noHeader, live bool) *Display {
	d := NewDisplay(w, plainLayout("192.168.1.23"), quiet, noHeader, live)
	d.SetNow(func() time.Time { return t0.Add(time.Minute) })
	return d
}

func TestHeaderPrintedByDefault(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false)
	d.Handle(changeEvent(t0, state.StatusUp))
	if !strings.Contains(buf.String(), "TIME") {
		t.Errorf("header missing: %q", buf.String())
	}
}

func TestNoHeaderSuppressesHeader(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, true, false)
	d.Handle(changeEvent(t0, state.StatusUp))
	if strings.Contains(buf.String(), "TIME") {
		t.Errorf("header present despite --no-header: %q", buf.String())
	}
}

func TestQuietSuppressesEverything(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, true, false, false)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	d.Finalize()
	if buf.Len() != 0 {
		t.Errorf("quiet mode wrote output: %q", buf.String())
	}
}

func TestNonLivePrintsOnlyFinalizedLines(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false)
	d.Handle(changeEvent(t0, state.StatusUp))                                                                                                                  // starts up line
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0)) // live update (ignored)
	// No finalized line yet: only the header.
	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("non-live output has %d lines before finalize: %q", strings.Count(buf.String(), "\n"), buf.String())
	}
	d.Finalize()
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 { // header + finalized line
		t.Errorf("finalized output = %d lines, want 2: %q", len(lines), buf.String())
	}
	if strings.Contains(buf.String(), "\r") {
		t.Errorf("non-live output contains carriage return: %q", buf.String())
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("non-live output contains ANSI: %q", buf.String())
	}
}

func TestLiveUpdateUsesCarriageReturn(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	out := buf.String()
	if !strings.Contains(out, "\r") {
		t.Errorf("live mode missing carriage return: %q", out)
	}
	if strings.Count(out, "\n") != 1 { // header only; live lines have no newline
		t.Errorf("live output newline count = %d, want 1: %q", strings.Count(out, "\n"), out)
	}
	d.Finalize()
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("finalized output must end with newline: %q", buf.String())
	}
}

func TestStatusChangeFinalizesLine(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(30*time.Second), state.StatusUp, state.Stats{Count: 2, Min: time.Millisecond, Max: 2 * time.Millisecond, Sum: 3 * time.Millisecond}, 0))
	// Flip to down: the up line finalizes with the duration of the ended state.
	downEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(30 * time.Second),
		Status: state.StatusDown, PrevStatus: state.StatusUp,
		Duration:  30 * time.Second,
		PrevStats: state.Stats{Count: 2, Min: time.Millisecond, Max: 2 * time.Millisecond, Sum: 3 * time.Millisecond},
		Fails:     1,
		Stats:     state.Stats{},
	}
	d.Handle(downEv)
	out := buf.String()
	if !strings.Contains(out, "0d 00:00:30") {
		t.Errorf("finalized duration missing: %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 { // header + finalized up line
		t.Errorf("lines after status change = %d, want 2: %q", len(lines), out)
	}
}

func TestFinalizeOnShutdownUsesCurrentDuration(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Finalize() // now = t0 + 1min (SetNow)
	out := buf.String()
	if !strings.Contains(out, "0d 00:01:00") {
		t.Errorf("shutdown finalize duration = %q, want 0d 00:01:00", out)
	}
}

func TestFinalizeIdempotent(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Finalize()
	d.Finalize()
	if strings.Count(buf.String(), "\n") != 2 {
		t.Errorf("Finalize not idempotent: %q", buf.String())
	}
}

func TestLiveFinalizeStartsWithCarriageReturn(t *testing.T) {
	// Regression: in live mode the cursor sits at the end of the last live
	// update; the finalized line must start with \r or it concatenates onto
	// the live line.
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	d.Finalize()
	out := buf.String()
	// The finalized line (after the last \r) must itself be \r-prefixed.
	lastCR := strings.LastIndex(out, "\r")
	if lastCR < 0 {
		t.Fatalf("no carriage return in live output: %q", out)
	}
	if !strings.HasPrefix(out[lastCR:], "\r1") && !strings.HasPrefix(out[lastCR:], "\r0") {
		t.Errorf("finalized line not \r-prefixed: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("finalized line must end with newline: %q", out)
	}
}

func TestNoOutputBeforeFirstEvent(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false)
	_ = d
	if buf.Len() != 0 {
		t.Errorf("output before first event: %q", buf.String())
	}
}

// TestErrorProbeUpdatesLiveLine: consecutive error probes must update the
// existing error line in place, never emit a new line (regression for the
// repeated-error-lines report).
func TestErrorProbeUpdatesLiveLine(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false) // non-live: only finalized lines
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(state.Event{Kind: state.EventError, Time: t0, Status: state.StatusError, Err: errBoom()})
	// A second error while already in error: the current (unprinted) error
	// line just updates — no new finalized line may appear.
	d.Handle(state.Event{Kind: state.EventProbeError, Time: t0.Add(2 * time.Second), Status: state.StatusError, Err: errBoom()})
	// header + finalized up line = 2; the errors added nothing yet.
	if got := strings.Count(buf.String(), "\n"); got != 2 {
		t.Errorf("newlines before finalize = %d, want 2: %q", got, buf.String())
	}
	// The error line finalizes exactly once at shutdown with the shutdown
	// duration (injected clock: t0 + 1min).
	d.Finalize()
	out := buf.String()
	if got := strings.Count(out, "\n"); got != 3 {
		t.Errorf("newlines after finalize = %d, want 3 (header + up + error): %q", got, out)
	}
	if !strings.Contains(out, "error   0d 00:01:00") {
		t.Errorf("error line missing final duration: %q", out)
	}
}

// TestErrorProbeLiveDurationUpdate verifies that in live mode the
// in-place error line's duration advances with each error probe (one
// logical line, redrawn in place — never new finalized lines).
func TestErrorProbeLiveDurationUpdate(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(state.Event{Kind: state.EventError, Time: t0, Status: state.StatusError, Err: errBoom()})
	d.Handle(state.Event{Kind: state.EventProbeError, Time: t0.Add(2 * time.Second), Status: state.StatusError, Err: errBoom()})
	d.Handle(state.Event{Kind: state.EventProbeError, Time: t0.Add(5 * time.Second), Status: state.StatusError, Err: errBoom()})
	out := buf.String()
	// The final redraw shows the advancing duration (5s).
	last := out[strings.LastIndex(out, "\r")+1:]
	if !strings.Contains(last, "error   0d 00:00:05") {
		t.Errorf("live error duration not updated to 5s: %q", last)
	}
	// The error line is redrawn in place, never finalized/appended: every
	// "error" occurrence is part of a live redraw (followed by \x1b[K or
	// end), never followed by a newline.
	if got := strings.Count(out, "error\n"); got != 0 {
		t.Errorf("error line finalized %d times: %q", got, out)
	}
	if got := strings.Count(out, "error"); got != 3 {
		t.Errorf("error redraws = %d, want 3 (0s/2s/5s): %q", got, out)
	}
}

func errBoom() error { return errors.New("boom") }
