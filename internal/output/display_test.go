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
	// Width 0 = unknown (never wrap) — the pre-#65 behavior.
	return newTestDisplaySized(w, quiet, noHeader, live, 0, 0)
}

// newTestDisplaySized injects a terminal size; width drives the live
// line's wrap bookkeeping (DECISIONS #65).
func newTestDisplaySized(w *bytes.Buffer, quiet, noHeader, live bool, width, height int) *Display {
	d := NewDisplay(w, plainLayout("192.168.1.23"), quiet, noHeader, live, func() (int, int) { return width, height })
	d.SetNow(func() time.Time { return t0.Add(time.Minute) })
	return d
}

// newTestDisplayResizable injects a MUTABLE terminal size AND clock:
// tests simulate a resize by changing the captured variables between
// redraws, and advance the clock to settle the resize freeze (DECISIONS
// #67).
func newTestDisplayResizable(w *bytes.Buffer, quiet, noHeader, live bool, width, height int) (*Display, *int, *int, *time.Time) {
	now := t0.Add(time.Minute)
	d := NewDisplay(w, plainLayout("192.168.1.23"), quiet, noHeader, live, func() (int, int) { return width, height })
	d.SetNow(func() time.Time { return now })
	return d, &width, &height, &now
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
	// the live line, and must END with \r\n (not bare \n) so the cursor
	// lands at column 0 of the next line — otherwise whatever prints next
	// (the exit summary) drifts right (user report 2026-08-17).
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	d.Finalize()
	out := buf.String()
	// The finalized line must end with explicit CRLF, not bare LF.
	if !strings.HasSuffix(out, "\r\n") {
		t.Errorf("finalized line must end with \\r\\n: %q", out)
	}
	// The finalized line itself must be \r-prefixed (strip the trailing
	// CRLF terminator, then the last \r starts the finalized line).
	body := strings.TrimSuffix(out, "\r\n")
	lastCR := strings.LastIndex(body, "\r")
	if lastCR < 0 {
		t.Fatalf("no carriage return in live output: %q", out)
	}
	if !strings.HasPrefix(body[lastCR:], "\r1") && !strings.HasPrefix(body[lastCR:], "\r0") {
		t.Errorf("finalized line not \r-prefixed: %q", out)
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
	if !strings.Contains(out, "error 0d 00:01:00") {
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
	if !strings.Contains(last, "error 0d 00:00:05") {
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

// TestLiveLineShowsAnimationFrame verifies the liveness animation renders
// on the live line only: live updates carry the rising bar in the
// DURATION padding (column 47), the finalized line at status change is a
// plain static line, and non-live output contains no frame glyphs at all
// (user request 2026-08-17). The visible screen is asserted through the
// terminal emulator — the raw stream contains every live redraw, so
// byte-level glyph checks would be wrong (DECISIONS #54 lesson).
func TestLiveLineShowsAnimationFrame(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true) // live
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	// A status change finalizes the up line into a plain static line.
	downEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(2 * time.Second),
		Status: state.StatusDown, PrevStatus: state.StatusUp,
		Duration: 2 * time.Second, Fails: 1,
	}
	d.Handle(downEv)
	// The animation advances on the 1-second ticker, not per event.
	d.Tick()
	d.Tick()

	scr := newTermScreen(10, 120)
	scr.feed(buf.String())
	frames := "▁▃▅▇"
	// Row 1: the finalized up line — static, no frame glyph.
	if strings.ContainsAny(scr.line(1), frames) {
		t.Errorf("finalized line animated: %q", scr.line(1))
	}
	// Row 2: the live down line — carries the animation frame.
	if !strings.ContainsAny(scr.line(2), frames) {
		t.Errorf("live line missing animation frame: %q", scr.line(2))
	}
	// The frame lives at column 45 (inside the DURATION padding at the
	// HOST-15 minimum), and the separator at column 46 stays a space so
	// it doesn't touch MIN.
	if runes := []rune(scr.line(2)); len(runes) > 46 {
		if c := runes[45]; !strings.ContainsRune(frames, c) {
			t.Errorf("live frame not at column 45 (got %q): %q", c, scr.line(2))
		}
		if c := runes[46]; c != ' ' {
			t.Errorf("separator at col 46 = %q, want space: %q", c, scr.line(2))
		}
	}
}

// TestDisplayTickAdvancesFrame verifies the animation advances on the
// 1-second ticker (user report 2026-08-17: per-event advancement is too
// slow with a long --interval).
func TestDisplayTickAdvancesFrame(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true)
	d.Handle(changeEvent(t0, state.StatusUp))

	frames := []rune{'▁', '▃', '▅', '▇'}
	for i := 0; i < 4; i++ {
		buf.Reset()
		d.Tick()
		// Handle already rendered frame 0 (▁); ticks advance to
		// ▃,▅,▇,▁.
		if !strings.ContainsRune(buf.String(), frames[(i+1)%len(frames)]) {
			t.Errorf("after Tick %d: missing frame %q in %q", i+1, frames[(i+1)%len(frames)], buf.String())
		}
	}
	// Tick is a no-op when non-live: it must not add anything beyond the
	// header Handle already wrote.
	nl := newTestDisplay(&buf, false, false, false)
	nl.Handle(changeEvent(t0, state.StatusUp))
	before := buf.Len()
	nl.Tick()
	if buf.Len() != before {
		t.Errorf("non-live Tick wrote output: %q", buf.String()[before:])
	}
}

// TestDisplayTickRefreshesDuration: the 1-second tick must refresh
// DURATION from the wall clock (now = t0+1min in the test display), so the
// counter keeps moving between probe events — not just the animation
// frame (user report 2026-08-17: duration did not update on the same
// schedule as the animation).
func TestDisplayTickRefreshesDuration(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, true) // live, now = t0+1min
	d.Handle(changeEvent(t0, state.StatusUp))

	// The first event line: duration from the probe event (0s).
	if strings.Contains(buf.String(), "0d 00:01:00") {
		t.Errorf("event line already shows tick duration: %q", buf.String())
	}
	// Tick: duration advances to the wall-clock value (1 min).
	buf.Reset()
	d.Tick()
	if !strings.Contains(buf.String(), "0d 00:01:00") {
		t.Errorf("tick did not refresh duration to 1m: %q", buf.String())
	}
	// Another tick (advance the injected clock) keeps it counting.
	d.SetNow(func() time.Time { return t0.Add(2 * time.Minute) })
	buf.Reset()
	d.Tick()
	if !strings.Contains(buf.String(), "0d 00:02:00") {
		t.Errorf("tick did not advance duration to 2m: %q", buf.String())
	}
}

func TestNonLiveNoAnimation(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplay(&buf, false, false, false) // non-live: finalized only
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	d.Finalize()
	for _, g := range []string{"▁", "▃", "▅", "▇"} {
		if strings.Contains(buf.String(), g) {
			t.Errorf("non-live output contains frame glyph %q: %q", g, buf.String())
		}
	}
}

// TestDisplayLiveLineStaysAnchoredWhenWrapped is the regression test for
// the reported plain-mode bug (DECISIONS #65): at a width below the
// minimum the live line wraps, and the pre-fix code wrote every redraw at
// the cursor's CURRENT row (the end of the wrapped tail), so the line
// walked DOWN one row per redraw, leaving stale fragments at every
// previous position. The fix walks back to the true start and the line
// must stay anchored at rows 2-3 (header wrapped to rows 0-1) across many
// redraws.
func TestDisplayLiveLineStaysAnchoredWhenWrapped(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplaySized(&buf, false, false, true, 60, 24)
	d.Handle(changeEvent(t0, state.StatusUp))
	for i := 1; i <= 3; i++ {
		d.Handle(successEvent(t0.Add(time.Duration(i)*time.Second), state.StatusUp,
			buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	}
	d.Tick() // another frame with a fresh wall-clock duration

	scr := newTermScreen(24, 60)
	scr.feed(buf.String())

	if !strings.HasPrefix(scr.line(0), "TIME") {
		t.Errorf("row 0 must be the header: %q", scr.line(0))
	}
	var anchors []int
	for r := 0; r < scr.rows; r++ {
		if strings.HasPrefix(scr.line(r), "11:00:35") {
			anchors = append(anchors, r)
		}
	}
	if len(anchors) != 1 || anchors[0] != 2 {
		t.Errorf("live line anchored at rows %v, want exactly [2] (no downward drift): %q", anchors, screenRows(scr))
	}
	for r := 0; r < scr.rows; r++ {
		if strings.Count(scr.line(r), "11:00:35") > 1 {
			t.Errorf("row %d carries two timestamps (interleaved): %q", r, scr.line(r))
		}
	}
}

// TestDisplayLiveLineGrowBackKeepsFrozenLine: growing the terminal back
// after a wrapped live line must NOT reclaim the old rendering (DECISIONS
// #67): the wrapped line freezes in place as history, and after the width
// settles the display restarts on a fresh row below it. Everything below
// the fresh line must be clean.
func TestDisplayLiveLineGrowBackKeepsFrozenLine(t *testing.T) {
	var buf bytes.Buffer
	d, wPtr, _, now := newTestDisplayResizable(&buf, false, false, true, 60, 24)
	*wPtr = 60
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0)) // 69 cells → wraps
	frame1 := buf.String()

	// Grow to 120 and observe the change while still settling: no write.
	*wPtr = 120
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	d.Tick()
	if buf.Len() != 0 {
		t.Fatalf("mid-settle write produced output: %q", buf.String())
	}
	// Past the settle window: the display restarts on a fresh row.
	*now = now.Add(time.Second)
	buf.Reset()
	d.Tick()
	frame2 := buf.String()
	if !strings.HasPrefix(frame2, "\r\n") {
		t.Errorf("restart must begin with CRLF to move below the frozen line: %q", frame2)
	}

	scr := newTermScreen(24, 60)
	scr.feed(frame1)
	scr.resize(120)
	scr.feed(frame2)
	// The frozen wrapped line stays (rows 2-3) — it is history now.
	if !strings.HasPrefix(scr.line(2), "11:00:35") {
		t.Errorf("frozen line not at row 2: row2=%q", scr.line(2))
	}
	// The fresh live line restarts below, on its own row.
	if !strings.HasPrefix(scr.line(4), "11:00:35") {
		t.Errorf("live line not restarted at row 4: row4=%q", scr.line(4))
	}
	// Nothing below the fresh line.
	for r := 5; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the fresh line: %q", r, got)
		}
	}
}

// TestDisplayResizeFreezeThenRestart: on a width change the live line is
// FROZEN (no in-place writes) until the width has been stable for
// resizeSettleDelay, then the display restarts on a fresh row below the
// frozen rendering (DECISIONS #67). This is the acceptance contract for
// reflowing terminals — no CPR, no reclaim of the re-wrapped line.
func TestDisplayResizeFreezeThenRestart(t *testing.T) {
	var buf bytes.Buffer
	d, wPtr, _, now := newTestDisplayResizable(&buf, false, false, true, 60, 24)
	*wPtr = 60
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0)) // 69 cells → wraps rows 2-3
	frame1 := buf.String()

	*wPtr = 100
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	d.Tick()
	if buf.Len() != 0 {
		t.Fatalf("mid-settle write produced output: %q", buf.String())
	}

	*now = now.Add(time.Second) // settled: 1.1s since the change
	buf.Reset()
	d.Tick()
	restart := buf.String()
	if !strings.HasPrefix(restart, "\r\n") {
		t.Errorf("restart must begin with CRLF to move below the frozen line: %q", restart)
	}
	if strings.Contains(restart, "\x1b[1A\r-") {
		t.Errorf("wrapped-line settle must NOT mark the frozen row (geometry unsafe): %q", restart)
	}

	scr := newTermScreen(24, 60)
	scr.feed(frame1)
	scr.resize(100)
	scr.feed(restart)
	// Frozen wrapped line at rows 2-3 (history), fresh line at row 4.
	if !strings.HasPrefix(scr.line(2), "11:00:35") {
		t.Errorf("frozen line not at row 2: row2=%q", scr.line(2))
	}
	if !strings.HasPrefix(scr.line(4), "11:00:35") {
		t.Errorf("fresh live line not at row 4: row4=%q", scr.line(4))
	}
	for r := 5; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the fresh line: %q", r, got)
		}
	}
}

// TestDisplayResizeSettleMarksFrozenRow: when the frozen line is provably
// a single row (the live line was one row before AND after the resize),
// the settle marks it with '-' so the scrollback shows a resize artifact
// instead of a duplicate-looking data row (DECISIONS #68). The geometry
// is safe on reflowing and non-reflowing terminals alike: a line that
// fits at both widths cannot re-wrap, so the frozen row is exactly the
// one directly above the fresh line.
func TestDisplayResizeSettleMarksFrozenRow(t *testing.T) {
	var buf bytes.Buffer
	d, wPtr, _, now := newTestDisplayResizable(&buf, false, false, true, 120, 24)
	*wPtr = 120
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0)) // 46 cells: 1 row at both widths
	frame1 := buf.String()

	*wPtr = 100
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	d.Tick() // frozen mid-settle
	if buf.Len() != 0 {
		t.Fatalf("mid-settle write produced output: %q", buf.String())
	}
	*now = now.Add(time.Second)
	buf.Reset()
	d.Tick()
	restart := buf.String()
	if !strings.Contains(restart, "\x1b[1A\r-\x1b[K\x1b[1B\r") {
		t.Errorf("settle must mark the frozen row with '-': %q", restart)
	}

	scr := newTermScreen(24, 120)
	scr.feed(frame1)
	scr.resize(100)
	scr.feed(restart)
	// The frozen row above the fresh line is now just '-'; the fresh line
	// is directly below it.
	if got := scr.line(1); got != "-" {
		t.Errorf("frozen row = %q, want just '-' (TIME cleared)", got)
	}
	if !strings.HasPrefix(scr.line(2), "11:00:35") {
		t.Errorf("fresh live line not at row 2: row2=%q", scr.line(2))
	}
	for r := 3; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the fresh line: %q", r, got)
		}
	}
}

// TestDisplayResizeDragKeepsFreezing: a resize DRAG fires many width
// changes in quick succession; each one must RESTART the settle clock, so
// the display stays frozen while the width keeps moving and restarts
// exactly once, only after the width has been stable for resizeSettleDelay
// (user-requested acceptance check, DECISIONS #67).
func TestDisplayResizeDragKeepsFreezing(t *testing.T) {
	var buf bytes.Buffer
	d, wPtr, _, now := newTestDisplayResizable(&buf, false, false, true, 60, 24)
	*wPtr = 60
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	buf.Reset()

	type step struct {
		width int
		adv   time.Duration
		want  string // "" = frozen, "restart" = fresh-row restart emitted
	}
	steps := []step{
		{100, 100 * time.Millisecond, ""},        // drag: 60 → 100
		{80, 100 * time.Millisecond, ""},         // drag: 100 → 80 (clock restarts)
		{100, 100 * time.Millisecond, ""},        // drag: 80 → 100 (clock restarts)
		{100, 200 * time.Millisecond, ""},        // 200ms after the LAST change: still settling
		{100, 100 * time.Millisecond, "restart"}, // 300ms of stability: restart fires
	}
	var restartOut string
	for i, s := range steps {
		*wPtr = s.width
		*now = now.Add(s.adv)
		buf.Reset()
		d.Tick()
		got := buf.String()
		switch s.want {
		case "":
			if got != "" {
				t.Fatalf("step %d (width %d): expected frozen, wrote: %q", i, s.width, got)
			}
		case "restart":
			if !strings.HasPrefix(got, "\r\n") {
				t.Errorf("step %d: restart must begin with CRLF: %q", i, got)
			}
			restartOut = got
		}
	}
	if restartOut == "" {
		t.Fatal("no restart emitted after the width settled")
	}
}

// TestDisplayFinalizeDuringResize: a status change while the width is
// settling must still finalize correctly — the finalized line lands on a
// fresh row below the frozen rendering, never reclaimed mid-reflow
// (DECISIONS #67).
func TestDisplayFinalizeDuringResize(t *testing.T) {
	var buf bytes.Buffer
	d, wPtr, _, now := newTestDisplayResizable(&buf, false, false, true, 60, 24)
	*wPtr = 60
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	frame1 := buf.String()

	// Width changes; before the settle window passes, a status change
	// forces the finalize path.
	*wPtr = 100
	*now = now.Add(100 * time.Millisecond)
	downEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(30 * time.Second),
		Status: state.StatusDown, PrevStatus: state.StatusUp,
		Duration: 30 * time.Second, Fails: 1,
		PrevStats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1),
	}
	buf.Reset()
	d.Handle(downEv) // finalize mid-settle

	scr := newTermScreen(24, 60)
	scr.feed(frame1)
	scr.resize(100)
	scr.feed(buf.String())
	// Frozen up line at rows 2-3; finalized line at row 4; new live down
	// line at row 5.
	if !strings.HasPrefix(scr.line(4), "11:00:35") {
		t.Errorf("finalized line not at row 4: row4=%q", scr.line(4))
	}
	if !strings.HasPrefix(scr.line(5), "11:01:05") {
		t.Errorf("new live line not at row 5: row5=%q", scr.line(5))
	}
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank: %q", r, got)
		}
	}
}

// TestDisplayFinalizeAfterWrappedLiveLineStartsClean: finalizing a wrapped
// live line must write the finalized line from the live line's TRUE start
// (walk-back), not from the cursor's row on the wrap tail — otherwise the
// finalized scrollback line's head is chopped into the old tail. The new
// live line then starts fresh below it. (The up line carries RTT stats so
// it genuinely wraps at 60; a statless up line is only 48 cells.)
func TestDisplayFinalizeAfterWrappedLiveLineStartsClean(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplaySized(&buf, false, false, true, 60, 24)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0)) // live up: rows 2-3
	downEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(30 * time.Second),
		Status: state.StatusDown, PrevStatus: state.StatusUp,
		Duration: 30 * time.Second, Fails: 1,
		PrevStats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), // ended up state
	}
	d.Handle(downEv) // finalize the up line, start a down live line

	scr := newTermScreen(24, 60)
	scr.feed(buf.String())
	// The finalized line starts at the anchor row (2) at column 0 with its
	// own timestamp; the new live line starts at row 4 below it.
	if !strings.HasPrefix(scr.line(2), "11:00:35") {
		t.Errorf("finalized line not at row 2 col 0 (interleaved with wrap tail?): row2=%q", scr.line(2))
	}
	if !strings.HasPrefix(scr.line(4), "11:01:05") {
		t.Errorf("new live line not at row 4: row4=%q", scr.line(4))
	}
	// Exactly one of each timestamp on screen — no duplicated fragments.
	if got := strings.Count(strings.Join(screenRows(scr), "\n"), "11:00:35"); got != 1 {
		t.Errorf("finalized timestamp appears %d times: %q", got, screenRows(scr))
	}
	if got := strings.Count(strings.Join(screenRows(scr), "\n"), "11:01:05"); got != 1 {
		t.Errorf("live timestamp appears %d times: %q", got, screenRows(scr))
	}
}

// TestDisplayNonLiveStaysEscapeFree: the wrap primitive must NEVER engage
// for piped/--no-live output — even at a width that would wrap, the output
// stays plain newline-terminated lines (the script contract, spec §2.5).
func TestDisplayNonLiveStaysEscapeFree(t *testing.T) {
	var buf bytes.Buffer
	d := newTestDisplaySized(&buf, false, false, false, 60, 24)
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	d.Finalize()
	if strings.Contains(buf.String(), "\x1b") {
		t.Errorf("non-live output contains escapes: %q", buf.String())
	}
}

// TestDisplayResizeDebugForensics: the plain display's resize observation
// (The display-resize debug-forensics test moved to forensics_debug_test.go
// — it asserts on debugx output and lives under -tags debug.)
