package output

import (
	"fmt"
	"io"
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
type Display struct {
	w        io.Writer
	layout   *Layout
	quiet    bool
	noHeader bool
	live     bool
	now      func() time.Time

	started bool
	cur     *Line
	frame   int // liveness animation frame (advances per probe event)
}

// NewDisplay builds a display. live controls in-place updating (decided
// by the caller from --live/--no-live and TTY state).
func NewDisplay(w io.Writer, layout *Layout, quiet, noHeader, live bool) *Display {
	return &Display{
		w:        w,
		layout:   layout,
		quiet:    quiet,
		noHeader: noHeader,
		live:     live,
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
			d.printLine(d.layout.Header(), false)
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
			d.printLine(d.layout.FormatLiveLine(*d.cur, frameChar(d.frame)), true)
		}
	case state.EventProbeSuccess, state.EventProbeFailure, state.EventProbeError:
		if d.cur == nil {
			d.cur = &Line{Time: ev.Time, Status: ev.Status}
		}
		d.cur.Duration = ev.Time.Sub(d.cur.Time)
		d.cur.Stats = ev.Stats
		d.cur.Fails = ev.Fails
		if d.live {
			d.printLine(d.layout.FormatLiveLine(*d.cur, frameChar(d.frame)), true)
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
	d.printLine(d.layout.FormatLiveLine(*d.cur, frameChar(d.frame)), true)
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
// the end of the last live update, so the line must be preceded by a
// carriage return (plus clear-to-EOL to wipe any live residue); it must
// also END with an explicit CRLF — a bare LF moves down but does not
// reset the column, so whatever prints next (the exit summary) would
// start mid-line and drift right (DECISIONS #54 lesson, user report
// 2026-08-17). In non-live mode it is plain newline-terminated output.
func (d *Display) printFinalized(s string) {
	if d.live {
		fmt.Fprintf(d.w, "\r%s\x1b[K\r\n", s)
		return
	}
	fmt.Fprintln(d.w, s)
}

// printLine writes a line. Live lines use a carriage return + clear-to-EOL
// so the previous live line is overwritten in place; finalized lines are
// plain newline-terminated output (scrollback history).
func (d *Display) printLine(s string, live bool) {
	if live {
		fmt.Fprintf(d.w, "\r%s\x1b[K", s)
		return
	}
	fmt.Fprintln(d.w, s)
}
