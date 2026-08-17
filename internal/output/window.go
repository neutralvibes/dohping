package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"dohping/internal/state"
)

// Window renders the fixed auto-scrolling window mode (spec §8): a bounded
// block of the most recent status lines plus the current live line, drawn
// in place on the normal terminal.
//
// The window is a fixed-size block (header + up to --window-lines data
// rows) anchored at the cursor position where it first appears. Every
// redraw moves the cursor back to the block's top row and rewrites the
// block in place, clearing each line to its end so stale characters never
// survive. Nothing else on screen is cleared or replaced: no alternate
// screen, no cursor-home, no clear-to-end-of-screen (the spec never asks
// for a screen clear — see DECISIONS #53). Content above and below the
// window block is left exactly as it was.
//
// The state engine's event semantics are identical to the plain display;
// only the rendering differs.
type Window struct {
	w        io.Writer
	layout   *Layout
	lines    int // visible data lines (history + live)
	quiet    bool
	noHeader bool
	heightFn func() int // terminal height; 0 = unknown
	now      func() time.Time

	started  bool
	lastRows int    // rows the block occupied in the previous frame
	history  []Line // finalized lines, bounded to lines-1
	cur      *Line  // current live line
	frame    int    // liveness animation frame (advances per probe event)
}

// NewWindow builds a window display. lines is the visible data-line count
// (--window-lines); heightFn returns the terminal height (0 = unknown).
func NewWindow(w io.Writer, layout *Layout, lines int, quiet, noHeader bool, heightFn func() int) *Window {
	return &Window{
		w:        w,
		layout:   layout,
		lines:    lines,
		quiet:    quiet,
		noHeader: noHeader,
		heightFn: heightFn,
		now:      time.Now,
	}
}

// SetNow overrides the clock (test injection).
func (w *Window) SetNow(f func() time.Time) { w.now = f }

// Enter is a no-op: the window renders in place on the normal terminal and
// takes over no screen state, so there is nothing to enter (DECISIONS #53).
func (w *Window) Enter() {}

// Exit is a no-op for the same reason: the block is left visible on the
// normal screen (like plain line mode leaves its lines) and the terminal
// needs no restoration.
func (w *Window) Exit() {}

// Handle consumes one engine event (same semantics as Display.Handle).
// The animation frame is NOT advanced here — it is driven by the 1-second
// Tick timer so the block keeps moving even when probe events are rare
// (long --interval; user report 2026-08-17).
func (w *Window) Handle(ev state.Event) {
	if w.quiet {
		return
	}
	switch ev.Kind {
	case state.EventStatusChange, state.EventError:
		w.finalizeLine(ev)
		w.cur = &Line{
			Time:   ev.Time,
			Status: ev.Status,
			Stats:  ev.Stats,
			Fails:  ev.Fails,
		}
	case state.EventProbeSuccess, state.EventProbeFailure, state.EventProbeError:
		if w.cur == nil {
			w.cur = &Line{Time: ev.Time, Status: ev.Status}
		}
		w.cur.Duration = ev.Time.Sub(w.cur.Time)
		w.cur.Stats = ev.Stats
		w.cur.Fails = ev.Fails
	}
	w.Redraw()
}

// Finalize closes the display: the current line becomes finalized history
// and the block is redrawn, then the cursor is moved to a fresh line below
// the block so the exit summary lands underneath it. Idempotent.
func (w *Window) Finalize() {
	if w.quiet || w.cur == nil {
		return
	}
	ln := *w.cur
	ln.Duration = w.now().Sub(ln.Time)
	w.pushHistory(ln)
	w.cur = nil
	w.Redraw()
	fmt.Fprint(w.w, "\r\n")
}

// Tick advances the liveness animation one frame and repaints the block.
// Driven by the app loop's 1-second timer, independent of probe cadence
// (user report 2026-08-17). No-op when quiet or no live line.
func (w *Window) Tick() {
	if w.quiet || w.cur == nil {
		return
	}
	w.frame++
	w.Redraw()
}

// Redraw repaints the window block in place. The block is always the same
// height (header + visible data rows), padded with blank rows when there
// are fewer events than the window holds, so the block never grows into
// the terminal and never relies on scrollback.
func (w *Window) Redraw() {
	if w.quiet {
		return
	}
	visible := w.visibleLines()
	rows := visible
	if !w.noHeader {
		rows++
	}
	var sb strings.Builder
	if w.started && w.lastRows > 1 {
		// The cursor sits on the last row of the previous block; move it
		// back to the block's top row AND to column 0. Cursor-up alone
		// preserves the column, which would start every row mid-line and
		// leave stale fragments on screen (user report, DECISIONS #54).
		fmt.Fprintf(&sb, "\x1b[%dA\r", w.lastRows-1)
	}
	if !w.noHeader {
		sb.WriteString(w.layout.Header())
		sb.WriteString("\x1b[K\r\n")
	}
	hist := w.history
	if len(hist) > visible-1 {
		hist = hist[len(hist)-(visible-1):]
	}
	// Data rows: finalized history (oldest at top), then the live line,
	// then blank padding rows so the block height stays constant. Rows are
	// separated by CRLF — bare LF moves down without resetting the column,
	// which would start every row at the previous row's end (DECISIONS #54).
	for i := 0; i < visible; i++ {
		switch {
		case i < len(hist):
			sb.WriteString(w.layout.FormatLine(hist[i]))
		case w.cur != nil && i == len(hist):
			// Live row carries the liveness animation frame; history rows
			// stay static (FormatLine) so the block doesn't buzz.
			sb.WriteString(w.layout.FormatLiveLine(*w.cur, frameChar(w.frame)))
		}
		sb.WriteString("\x1b[K") // clear this row to its end (stale chars)
		if i < visible-1 {
			sb.WriteString("\r\n")
		}
	}
	// If the block shrank (terminal resized smaller), clear the stale rows
	// left below it, then return the cursor to the new last row.
	if w.started && w.lastRows > rows {
		for i := 0; i < w.lastRows-rows; i++ {
			sb.WriteString("\x1b[1B\x1b[K")
		}
		fmt.Fprintf(&sb, "\x1b[%dA", w.lastRows-rows)
	}
	w.started = true
	w.lastRows = rows
	fmt.Fprint(w.w, sb.String())
}

// visibleLines returns how many data lines fit: the configured window
// size, reduced when the terminal is too small (spec §8.5), never below 1.
func (w *Window) visibleLines() int {
	n := w.lines
	if h := w.heightFn(); h > 0 {
		headerRows := 0
		if !w.noHeader {
			headerRows = 1
		}
		if h-headerRows < n {
			n = h - headerRows
		}
	}
	if n < 1 {
		n = 1
	}
	return n
}

// finalizeLine moves the current line into bounded history when a status
// change ends it.
func (w *Window) finalizeLine(ev state.Event) {
	if w.cur == nil {
		return
	}
	ln := *w.cur
	ln.Duration = ev.Duration
	ln.Stats = ev.PrevStats
	ln.Fails = ev.Fails
	w.pushHistory(ln)
	w.cur = nil
}

// pushHistory appends a finalized line, dropping the oldest beyond the
// window's history capacity (spec §8.4).
func (w *Window) pushHistory(ln Line) {
	w.history = append(w.history, ln)
	cap := w.lines - 1
	if cap < 1 {
		cap = 1
	}
	if len(w.history) > cap {
		w.history = w.history[len(w.history)-cap:]
	}
}
