package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"dohping/internal/state"
)

// Display renders the engine event stream as the plain line display.
//
//   - header printed once unless --no-header
//   - the current line live-updates in place (TTY) while the status is
//     unchanged; TIME stays the state-start time
//   - on status change / error / shutdown the current line is finalized
//     (printed once with a newline) and a new line begins
//   - non-live output (piped, --no-live) prints finalized lines only,
//     never carriage returns or ANSI
//
// Live-line width awareness (DECISIONS #65): the live line is re-anchored
// on every redraw using the same physical-row primitive as the window
// block, reduced to one row's bookkeeping. If the terminal is narrower
// than the line it WRAPS, and without bookkeeping every redraw would
// start at the cursor's current row (the end of the wrapped tail), so the
// line walked down the page one row per redraw leaving stale fragments.
// The display tracks lastPhysRows (physical rows the previous live line
// occupied), walks back to the true start before rewriting, and clears
// rows the line no longer uses when it shrinks (e.g. the terminal grew
// back). Column widths stay FIXED for the run — plain mode's history is
// the terminal's scrollback, which cannot be re-laid-out, so HOST never
// re-measures here (unlike window mode, which repaints its own buffer).
// The primitive engages only when live; piped/--no-live output is
// byte-identical plain lines.
//
// REFLOWING terminals (Windows Terminal, Terminal.app, iTerm2) re-wrap
// existing lines on resize, which moves the live line under the relative
// bookkeeping — the walk-back overshoots (creep up) and the re-wrapped
// first half is never overwritten ("does not clear the rest on wrap").
// Fix (DECISIONS #66): when the terminal WIDTH changes, the display
// queries the terminal's cursor position (DSR/CPR, \x1b[6n →
// \x1b[<row>;<col>R, provided by the app) — the cursor sits at the end
// of the (re-wrapped) live line — and recomputes the anchor from where
// the terminal actually put it. On non-reflowing terminals the cursor
// agrees with our bookkeeping and nothing changes; on terminals that do
// not answer the query the display degrades to the relative behavior.
type Display struct {
	w        io.Writer
	layout   *Layout
	quiet    bool
	noHeader bool
	live     bool
	sizeFn   func() (width, height int) // terminal size; width drives wrap math (0 = unknown)
	reanchor func() (row int, ok bool)  // terminal cursor row via DSR/CPR (1-based); nil = never
	now      func() time.Time

	started      bool
	cur          *Line
	frame        int  // liveness animation frame (advances per probe event)
	lastPhysRows int  // physical rows the previous live line occupied (wrap bookkeeping)
	lastWidth    int  // terminal width at the last write (0 = unknown)
	anchor       int  // cursor row (CPR base) where the live line starts
	haveAnchor   bool // anchor calibrated (first write / first reflow)
}

// NewDisplay builds a display. live controls in-place updating (decided
// by the caller from --live/--no-live and TTY state). sizeFn returns the
// terminal size (0 = unknown → never wrap); nil means never wrap.
// reanchor queries the terminal's cursor row (DSR/CPR) after a width
// change — the app supplies it (it owns stdin); nil disables re-anchoring
// (terminals that never answer degrade to relative bookkeeping).
func NewDisplay(w io.Writer, layout *Layout, quiet, noHeader, live bool, sizeFn func() (width, height int), reanchor func() (row int, ok bool)) *Display {
	return &Display{
		w:        w,
		layout:   layout,
		quiet:    quiet,
		noHeader: noHeader,
		live:     live,
		sizeFn:   sizeFn,
		reanchor: reanchor,
		now:      time.Now,
	}
}

// SetNow overrides the clock (test injection).
func (d *Display) SetNow(f func() time.Time) { d.now = f }

// Handle consumes one engine event.
func (d *Display) Handle(ev state.Event) {
	if d.quiet {
		return
	}
	if !d.started {
		d.started = true
		if !d.noHeader {
			d.printLine(d.layout.Header())
		}
	}

	switch ev.Kind {
	case state.EventStatusChange, state.EventError:
		d.finalizeLine(ev)
		d.cur = &Line{
			Time:   ev.Time,
			Status: ev.Status,
			Stats:  ev.Stats,
			Fails:  ev.Fails,
		}
		if d.live {
			d.writeLive(d.layout.FormatLiveLine(*d.cur, frameChar(d.frame)))
		}
	case state.EventProbeSuccess, state.EventProbeFailure, state.EventProbeError:
		if d.cur == nil {
			d.cur = &Line{Time: ev.Time, Status: ev.Status}
		}
		d.cur.Duration = ev.Time.Sub(d.cur.Time)
		d.cur.Stats = ev.Stats
		d.cur.Fails = ev.Fails
		if d.live {
			d.writeLive(d.layout.FormatLiveLine(*d.cur, frameChar(d.frame)))
		}
	}
}

// Tick advances the liveness animation one frame AND refreshes the
// DURATION from the wall clock, then redraws the live line in place. It is
// driven by a 1-second timer in the app loop, INDEPENDENT of probe
// cadence: with a long --interval the probe events are rare, but the
// display must still visibly move every second (user report 2026-08-17).
// Duration is "how long has this status held" — wall-clock elapsed time,
// which grows between probes; the event-based value is only a sample
// (same math Finalize uses at shutdown, monotonic-safe per spec §20.4).
// No-op when quiet, non-live, or no current line.
func (d *Display) Tick() {
	if d.quiet || !d.live || d.cur == nil {
		return
	}
	d.cur.Duration = d.now().Sub(d.cur.Time)
	d.frame++
	d.writeLive(d.layout.FormatLiveLine(*d.cur, frameChar(d.frame)))
}

// finalizeLine prints the current line as finalized history when a status
// change ends it. ev carries the ended state's final duration and stats.
func (d *Display) finalizeLine(ev state.Event) {
	if d.cur == nil {
		return
	}
	ln := *d.cur
	ln.Duration = ev.Duration
	ln.Stats = ev.PrevStats
	ln.Fails = ev.Fails
	d.printFinalized(d.layout.FormatLine(ln))
	d.cur = nil
}

// Finalize closes the display: the current line is printed once and the
// display stops accepting events. Called on shutdown and on --count
// exhaustion. Idempotent.
func (d *Display) Finalize() {
	if d.quiet || d.cur == nil {
		return
	}
	ln := *d.cur
	ln.Duration = d.now().Sub(ln.Time)
	d.printFinalized(d.layout.FormatLine(ln))
	d.cur = nil
}

// printFinalized writes a finalized line. In live mode the cursor sits at
// the end of the last live update (possibly on a WRAPPED tail row), so the
// line must be walked back to the live line's true start before writing,
// plus clear-to-EOL to wipe any live residue, and must END with an explicit
// CRLF — a bare LF moves down but does not reset the column, so whatever
// prints next (the exit summary) would start mid-line and drift right
// (DECISIONS #54 lesson, user report 2026-08-17). Defensive clearing
// removes rows the previous live line used but this finalized line does
// not (DECISIONS #65). The next live line starts fresh below, so the wrap
// bookkeeping resets. In non-live mode it is plain newline-terminated
// output.
func (d *Display) printFinalized(s string) {
	if !d.live {
		fmt.Fprintln(d.w, s)
		return
	}
	tw := d.termWidth()
	d.maybeReanchor(tw, s)
	var sb strings.Builder
	if d.lastPhysRows > 1 {
		fmt.Fprintf(&sb, "\x1b[%dA\r", d.lastPhysRows-1)
	} else {
		sb.WriteString("\r") // always return to column 0 first
	}
	sb.WriteString(s)
	sb.WriteString("\x1b[K")
	phys := physicalRows(cellWidth(s), tw)
	if phys < d.lastPhysRows {
		for i := 0; i < d.lastPhysRows-phys; i++ {
			// Reset to column 0 before clearing: cursor-down preserves the
			// column, and the cursor sits at the END of the written line —
			// ESC[K alone would only clear from there and leave the stale
			// text at the row's start (DECISIONS #65).
			sb.WriteString("\x1b[1B\r\x1b[K")
		}
		fmt.Fprintf(&sb, "\x1b[%dA", d.lastPhysRows-phys)
	}
	sb.WriteString("\r\n")
	d.lastPhysRows = 1 // next live line starts fresh below the finalized line
	fmt.Fprint(d.w, sb.String())
}

// writeLive writes the live line in place with wrap bookkeeping (DECISIONS
// #65): if the previous live line wrapped, walk back to its true start
// before rewriting (the cursor sits on the wrapped tail row otherwise);
// if this line uses fewer rows than the previous one (terminal grew back),
// clear the rows no longer used. Terminal width is re-read on EVERY write,
// so a resize is picked up by probe events, the 1-second tick, and the
// SIGWINCH fast path alike. A width change also triggers a cursor-query
// re-anchor for reflowing terminals (DECISIONS #66).
func (d *Display) writeLive(s string) {
	tw := d.termWidth()
	d.maybeReanchor(tw, s)
	var sb strings.Builder
	if d.lastPhysRows > 1 {
		fmt.Fprintf(&sb, "\x1b[%dA\r", d.lastPhysRows-1)
	} else {
		sb.WriteString("\r") // always return to column 0 first
	}
	sb.WriteString(s)
	sb.WriteString("\x1b[K")
	phys := physicalRows(cellWidth(s), tw)
	if phys < d.lastPhysRows {
		for i := 0; i < d.lastPhysRows-phys; i++ {
			// Reset to column 0 before clearing: cursor-down preserves the
			// column, and the cursor sits at the END of the written line —
			// ESC[K alone would only clear from there and leave the stale
			// text at the row's start (DECISIONS #65).
			sb.WriteString("\x1b[1B\r\x1b[K")
		}
		fmt.Fprintf(&sb, "\x1b[%dA", d.lastPhysRows-phys)
	}
	d.lastPhysRows = phys
	fmt.Fprint(d.w, sb.String())
}

// maybeReanchor handles a terminal resize on a REFLOWING terminal
// (DECISIONS #66): when the width changed since the last write, the
// terminal may have re-wrapped the live line, invalidating the relative
// walk-back bookkeeping. Query the terminal's cursor position (DSR/CPR) —
// the cursor sits at the end of the (re-wrapped) live line — and recompute
// the anchor from where the terminal actually put it. Three outcomes:
//   - cursor agrees with our bookkeeping → no reflow happened, nothing to do
//   - cursor elsewhere → reflow: the line now spans physicalRows(cells, tw)
//     rows ending at the cursor; re-anchor and reset the bookkeeping
//   - no answer (piped stdin, dumb terminal) → keep relative bookkeeping
//     (degrades to the #65 behavior)
func (d *Display) maybeReanchor(tw int, s string) {
	if tw == d.lastWidth || d.reanchor == nil {
		return
	}
	d.lastWidth = tw
	row, ok := d.reanchor()
	if !ok {
		return
	}
	if d.haveAnchor && row == d.anchor+d.lastPhysRows-1 {
		return // cursor where we left it: no reflow
	}
	if !d.haveAnchor {
		// First write: the cursor is at the live line's start.
		d.anchor = row
		d.haveAnchor = true
		return
	}
	// Reflow detected: the line was re-wrapped at the new width and the
	// cursor sits at its end.
	p := physicalRows(cellWidth(s), tw)
	d.anchor = row - (p - 1)
	d.lastPhysRows = p
}

// termWidth returns the terminal width in cells (0 = unknown → no wrap).
func (d *Display) termWidth() int {
	if d.sizeFn == nil {
		return 0
	}
	w, _ := d.sizeFn()
	return w
}

// printLine writes a plain (non-live) line: the header, or finalized
// lines in non-live mode.
func (d *Display) printLine(s string) {
	fmt.Fprintln(d.w, s)
}
