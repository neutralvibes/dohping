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
// Terminal resize (DECISIONS #64): the block re-measures the terminal on
// EVERY redraw — width drives the HOST column (Layout.Resize: it expands
// with available width, retracts to the 15-cell minimum, and truncates
// long hosts with …), height drives the visible-line count, and the
// cursor math counts PHYSICAL rows so a line that wraps below the minimum
// column widths still repaints as a coherent block. The re-measure is
// platform-neutral: it runs on probe events and the 1-second tick (so
// Windows, which has no SIGWINCH, self-heals within a second), and the
// Unix SIGWINCH channel is just the immediate-repaint fast path.
//
// The state engine's event semantics are identical to the plain display;
// only the rendering differs.
type Window struct {
	w        io.Writer
	layout   *Layout
	lines    int // visible data lines (history + live)
	quiet    bool
	noHeader bool
	sizeFn   func() (width, height int) // terminal size; 0 = unknown
	now      func() time.Time

	started      bool
	lastPhysRows int    // PHYSICAL rows the block occupied in the previous frame
	history      []Line // finalized lines, bounded to lines-1
	cur          *Line  // current live line
	frame        int    // liveness animation frame (advances on Tick)
}

// NewWindow builds a window display. lines is the visible data-line count
// (--window-lines); sizeFn returns the terminal size in cells (0 =
// unknown → startup column policy, full configured window height).
func NewWindow(w io.Writer, layout *Layout, lines int, quiet, noHeader bool, sizeFn func() (width, height int)) *Window {
	return &Window{
		w:        w,
		layout:   layout,
		lines:    lines,
		quiet:    quiet,
		noHeader: noHeader,
		sizeFn:   sizeFn,
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

// Tick advances the liveness animation one frame, refreshes the live
// line's DURATION from the wall clock (it grows between probe events),
// and repaints the block. Driven by the app loop's 1-second timer,
// independent of probe cadence (user report 2026-08-17). No-op when quiet
// or no live line.
func (w *Window) Tick() {
	if w.quiet || w.cur == nil {
		return
	}
	w.cur.Duration = w.now().Sub(w.cur.Time)
	w.frame++
	w.Redraw()
}

// Redraw repaints the window block in place. The block is always the same
// height (header + visible data rows), padded with blank rows when there
// are fewer events than the window holds, so the block never grows into
// the terminal and never relies on scrollback.
//
// Every redraw re-measures the terminal (DECISIONS #64): width re-computes
// the HOST column, height re-computes the visible-line count, and each
// row's PHYSICAL span is counted (a line wider than the terminal wraps,
// so the cursor-up count and stale-row clearing are in physical rows, not
// logical lines — logical math is what fragmented the screen on resize).
func (w *Window) Redraw() {
	if w.quiet {
		return
	}
	tw, th := 0, 0
	if w.sizeFn != nil {
		tw, th = w.sizeFn()
	}
	if tw > 0 {
		w.layout.Resize(tw)
	}
	visible := w.visibleLinesFrom(th)
	rows := visible
	if !w.noHeader {
		rows++
	}

	// Render every row of the frame up front so the physical span of the
	// whole block is known before any cursor movement is emitted.
	rowStrs := make([]string, rows)
	idx := 0
	if !w.noHeader {
		rowStrs[idx] = w.layout.Header()
		idx++
	}
	hist := w.history
	if len(hist) > visible-1 {
		hist = hist[len(hist)-(visible-1):]
	}
	for i := 0; i < visible; i++ {
		switch {
		case i < len(hist):
			rowStrs[idx] = w.layout.FormatLine(hist[i])
		case w.cur != nil && i == len(hist):
			// Live row carries the liveness animation frame; history rows
			// stay static (FormatLine) so the block doesn't buzz.
			rowStrs[idx] = w.layout.FormatLiveLine(*w.cur, frameChar(w.frame))
		}
		idx++
	}
	totalPhys := 0
	for _, s := range rowStrs {
		totalPhys += physicalRows(cellWidth(s), tw)
	}

	var sb strings.Builder
	if w.started && w.lastPhysRows > 1 {
		// The cursor sits on the last physical row of the previous block;
		// move it back to the block's top row AND to column 0. Cursor-up
		// alone preserves the column, which would start every row mid-line
		// and leave stale fragments on screen (user report, DECISIONS #54).
		fmt.Fprintf(&sb, "\x1b[%dA\r", w.lastPhysRows-1)
	}
	for i, s := range rowStrs {
		sb.WriteString(s)
		sb.WriteString("\x1b[K") // clear this row to its end (stale chars)
		if i < len(rowStrs)-1 {
			// Rows separated by CRLF — bare LF moves down without
			// resetting the column (DECISIONS #54).
			sb.WriteString("\r\n")
		}
	}
	// If the block shrank (terminal resized bigger, or a wrapped line
	// unwrapped), clear the stale rows left below it, then return the
	// cursor to the new last row. Each clear resets to column 0 first:
	// cursor-down preserves the column, and the cursor may sit at the end
	// of a non-blank last row (full window), so ESC[K alone would only
	// clear from that column and leave the stale text (DECISIONS #65).
	if w.started && w.lastPhysRows > totalPhys {
		for i := 0; i < w.lastPhysRows-totalPhys; i++ {
			sb.WriteString("\x1b[1B\r\x1b[K")
		}
		fmt.Fprintf(&sb, "\x1b[%dA", w.lastPhysRows-totalPhys)
	}
	w.started = true
	w.lastPhysRows = totalPhys
	fmt.Fprint(w.w, sb.String())
}

// physicalRows is how many terminal rows a line of the given cell width
// occupies at the given terminal width: 1 unless it wraps. Unknown
// terminal width (≤ 0) never wraps.
func physicalRows(cells, termWidth int) int {
	if termWidth <= 0 || cells <= termWidth {
		return 1
	}
	return (cells + termWidth - 1) / termWidth
}

// visibleLines returns how many data lines fit: the configured window
// size, reduced when the terminal is too small (spec §8.5), never below 1.
func (w *Window) visibleLines() int {
	_, th := w.terminalSize()
	return w.visibleLinesFrom(th)
}

func (w *Window) visibleLinesFrom(h int) int {
	n := w.lines
	if h > 0 {
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

func (w *Window) terminalSize() (int, int) {
	if w.sizeFn == nil {
		return 0, 0
	}
	return w.sizeFn()
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
