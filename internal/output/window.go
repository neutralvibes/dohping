package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"dohping/internal/debugx"
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
//
// REFLOWING terminals re-wrap the block on resize (see Display doc —
// DECISIONS #67): when the width changes, the block FREEZES redraws until
// the terminal settles, then restarts on a fresh row below the frozen
// rendering; the old block stays in scrollback as history. The CPR
// re-anchor (#66) was removed — ConPTY's cursor positions are unreliable
// on resize (microsoft/terminal#18725). DECISIONS #70: the freeze is
// CONDITIONAL — it fires only when the reflow would actually move the
// block (any line of the last completed frame changes its physical row
// count at the new width). A same-band resize (every line keeps its rows,
// so a reflow leaves the block untouched) repaints in place at the new
// columns instead of leaving a frozen block behind — but only after the
// width has been stable for resizeSettleDelay (DECISIONS #73): an
// event-timed redraw landing mid-reflow writes to a canvas that is still
// moving, which shifts the block by a row. Same-band repaints are
// deferred, never restarted.
//
// REFLOW-AWARE RECLAIM (SPEC-window-resize-reclaim.md): a CROSSING that
// settles above the essentials floor no longer freezes either. The
// terminal reflows the on-screen frame to a known span — R = Σ
// physicalRows(cellWidth(row), tw) over the last completed frame's rows,
// the same sum the crossing decision already computes — and the cursor
// follows its content through the reflow, so the reflowed frame's top is
// exactly R−1 rows above the cursor. The block is therefore RECLAIMED in
// place: walk back R−1, rewrite the fresh trimmed frame, clear the
// R−N stale rows. No frozen copy, no restart, no scrollback reliance
// (SPECIFICATION.md §8.5). The freeze survives only below the essentials
// floor, where the fresh frame itself wraps and its reflowed span can
// exceed the screen (R > th — the anchor is unknowable then).
type Window struct {
	w        io.Writer
	layout   *Layout
	lines    int // visible data lines (history + live)
	quiet    bool
	noHeader bool
	sizeFn   func() (width, height int) // terminal size; 0 = unknown
	now      func() time.Time

	started       bool
	lastPhysRows  int       // PHYSICAL rows the block occupied in the previous frame
	history       []Line    // finalized lines, bounded to lines-1
	cur           *Line     // current live line
	frame         int       // liveness animation frame (advances on Tick)
	lastWidth     int       // terminal width at the last redraw (0 = unknown)
	resizeSince   time.Time // when the current width change was first observed
	resizePending bool      // width changed (crossing); redraws frozen until the terminal settles
	lastRows      []string  // rendered rows of the last COMPLETED frame (resize reflow check)
	deferPending  bool      // same-band width change; repaints deferred until the width settles (#73)
	reclaimRows   int       // crossing settled above the floor: reflowed span of the on-screen frame to reclaim in place (SPEC-window-resize-reclaim)
	forceRender   bool      // finalize: render now regardless of the settle window
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
	// A resize in flight forces the render: the final block must land
	// NOW, not after the settle window — reclaimed in place when the
	// crossing is reclaimable, restarted below otherwise (DECISIONS #67,
	// SPEC-window-resize-reclaim.md). The same-band defer (#73) is
	// cleared for the same reason.
	w.forceRender = true
	w.Redraw()
	w.forceRender = false
	_, _ = fmt.Fprint(w.w, "\r\n")
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
	// RESIZE (DECISIONS #67 + #70): freeze redraws while the width is
	// settling — a reflowing terminal re-wraps the block and its position
	// is unknowable mid-reflow (the CPR re-anchor of #66 failed on ConPTY,
	// which reports unreliable cursor positions — microsoft/terminal
	// #18725). The freeze is conditional (#70): only a width change that
	// would MOVE the last completed frame (a line crossing a wrap
	// boundary) freezes; same-band changes repaint in place. Once settled,
	// restart the block on a fresh row below the frozen rendering; the
	// old block stays in scrollback as history.
	// Debug forensics (DECISIONS #74): redraws during a resize episode
	// are logged — the suppressed and deferred ones are the evidence
	// that no write landed mid-reflow, and the settle repaint records
	// the block's physical span against the previous frame's (a span
	// mismatch is how a shifted block shows up in the log).
	interesting := w.resizePending || w.deferPending
	w.observeResize(tw)
	if w.resizePending && !w.resizeSettled() {
		debugx.Debugf("redraw", "suppressed (crossing, %v left of settle)", resizeSettleDelay-w.now().Sub(w.resizeSince))
		return // mid-reflow: defer the redraw
	}
	if w.deferPending {
		// Same-band resize (DECISIONS #73): the reflow cannot move the
		// block, but the terminal is mid-reflow right now — writing now
		// lands on a canvas that is still moving. Defer the in-place
		// repaint until the width has been stable for resizeSettleDelay,
		// then repaint in place (no frozen block, no restart). Every
		// further width change restarts the settle clock (a drag extends
		// the hold).
		if w.resizeSettled() {
			debugx.Debugf("redraw", "defer released → in-place repaint")
			w.deferPending = false
		} else {
			debugx.Debugf("redraw", "deferred (settle, %v left)", resizeSettleDelay-w.now().Sub(w.resizeSince))
			return
		}
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

	// Settled crossing (REFLOW-AWARE RECLAIM, SPEC-window-resize-reclaim
	// .md): the terminal has re-wrapped the on-screen frame to R rows —
	// reflowedSpan of the last completed frame at the new width — and the
	// cursor followed its content, so the frame's top is exactly R−1 rows
	// above the cursor. Reclaim it in place: walk back R−1, overwrite
	// with the fresh trimmed frame, clear the R−N stale rows. No frozen
	// copy, no restart, no scrollback reliance (§8.5). The freeze remains
	// the fallback when the fresh frame itself wraps (below the essentials
	// floor — its anchor is unknowable) or when the reflowed span cannot
	// fit the screen (R > th).
	if w.resizePending {
		if freshFits(rowStrs, tw) {
			R := reflowedSpan(w.lastRows, tw)
			if R <= th {
				debugx.Debugf("redraw", "reclaim in place (R=%d N=%d tw=%d)", R, totalPhys, tw)
				w.reclaimRows = R
			} else {
				debugx.Debugf("redraw", "freeze settled → restart below frozen block (tw=%d)", tw)
				w.resizeRestart()
			}
		} else {
			debugx.Debugf("redraw", "freeze settled → restart below frozen block (tw=%d)", tw)
			w.resizeRestart()
		}
		w.resizePending = false
	}

	// Reflowing-terminal re-anchor was removed with the CPR machinery
	// (DECISIONS #67): ConPTY's cursor positions are unreliable on resize,
	// so the block never reclaims a reflowed rendering via a query — the
	// reclaim above needs no answer, only the app's own R math.

	var sb strings.Builder
	walkBack := w.lastPhysRows
	if w.reclaimRows > 0 {
		walkBack = w.reclaimRows
	}
	if w.started && walkBack > 1 {
		// The cursor sits on the last physical row of the previous block;
		// move it back to the block's top row AND to column 0. Cursor-up
		// alone preserves the column, which would start every row mid-line
		// and leave stale fragments on screen (user report, DECISIONS #54).
		fmt.Fprintf(&sb, "\x1b[%dA\r", walkBack-1)
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
	staleClear := w.lastPhysRows
	if w.reclaimRows > 0 {
		staleClear = w.reclaimRows
	}
	if w.started && staleClear > totalPhys {
		for i := 0; i < staleClear-totalPhys; i++ {
			sb.WriteString("\x1b[1B\r\x1b[K")
		}
		fmt.Fprintf(&sb, "\x1b[%dA", staleClear-totalPhys)
	}
	reclaimed := w.reclaimRows > 0
	w.reclaimRows = 0
	w.started = true
	w.lastPhysRows = totalPhys
	w.lastRows = rowStrs
	if interesting {
		if reclaimed {
			debugx.Debugf("redraw", "reclaimed tw=%d phys=%d (was %d) rows=%d", tw, totalPhys, walkBack, rows)
		} else {
			debugx.Debugf("redraw", "repainted tw=%d phys=%d (was %d) rows=%d", tw, totalPhys, w.lastPhysRows, rows)
		}
	}
	_, _ = fmt.Fprint(w.w, sb.String())
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

// reflowedSpan is the physical span the given frame's rows would occupy
// if the terminal re-wrapped them at tw — the height of the on-screen
// block after a reflow, and therefore the anchor math for the in-place
// reclaim (SPEC-window-resize-reclaim.md §3.1): the cursor follows its
// content through the reflow, so the reflowed frame's top is exactly
// reflowedSpan−1 rows above it.
func reflowedSpan(rows []string, tw int) int {
	n := 0
	for _, r := range rows {
		n += physicalRows(cellWidth(r), tw)
	}
	return n
}

// freshFits reports whether every row of the fresh frame fits on a single
// physical row at tw — the essentials-floor test. Above the floor the
// trim (DECISIONS #71) guarantees it, so a reclaim leaves a clean block;
// below it the fresh frame wraps by design and its reflowed anchor is
// unknowable, so the freeze fallback applies (SPEC-window-resize-reclaim
// .md §3.2).
func freshFits(rows []string, tw int) bool {
	if tw <= 0 {
		return false
	}
	for _, r := range rows {
		if cellWidth(r) > tw {
			return false
		}
	}
	return true
}

// resizeSettled reports whether the width has been stable for
// resizeSettleDelay — the point where a write is safe again (no
// mid-reflow canvas). A finalize forces the render immediately
// (forceRender), never waiting out the settle.
func (w *Window) resizeSettled() bool {
	return w.forceRender || w.now().Sub(w.resizeSince) >= resizeSettleDelay
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

// observeResize records a width change and decides how the block must
// behave (DECISIONS #67 + #70 + #73, SPEC-window-resize-reclaim.md). A
// reflowing terminal re-wraps existing lines on width change, but a line
// that keeps its physical row count cannot move in a reflow (it fits in
// the same rows at both widths — the DECISIONS #68 safety reasoning,
// generalized from one line to the whole block). So: if any row of the
// last COMPLETED frame changes its physical row count at the new width,
// the block would move — redraws FREEZE until the width has been stable
// for resizeSettleDelay, then the settle decides between an in-place
// RECLAIM (above the essentials floor, where the reflowed span is
// exactly known) and a restart below the frozen rendering (below the
// floor / when the reflowed span exceeds the screen). If every row keeps
// its count, the block may be reclaimed in place — but the repaint is
// DEFERRED for the same settle delay (#73), so no event-timed redraw
// lands mid-reflow. While either is pending, every further width change
// restarts the settle clock (a drag extends the hold). The first
// observation only calibrates lastWidth; no decision runs before a frame
// has been completed. No-op when the width is unknown (≤ 0).
func (w *Window) observeResize(tw int) {
	if tw <= 0 {
		return
	}
	if w.lastWidth == 0 {
		w.lastWidth = tw
		return
	}
	if tw == w.lastWidth {
		return
	}
	old := w.lastWidth
	w.lastWidth = tw
	if w.resizePending {
		// Already pending: keep holding and restart the settle clock —
		// a drag across further widths extends the hold (#67 behavior).
		w.resizeSince = w.now()
		debugx.Debugf("resize", "drag: %d→%d (settle clock restarted)", old, tw)
		return
	}
	if !w.started {
		debugx.Debugf("resize", "%d→%d before first frame (no block on screen — ignored)", old, tw)
		return // no completed frame yet — nothing on screen to reclaim
	}
	// Would a reflow to the new width move the last completed frame?
	reflowed := reflowedSpan(w.lastRows, tw)
	if reflowed != w.lastPhysRows {
		w.resizeSince = w.now()
		w.resizePending = true
		w.deferPending = false
		debugx.Debugf("resize", "%d→%d rows %d→%d → crossing (settle decides reclaim/freeze)", old, tw, w.lastPhysRows, reflowed)
	} else {
		// Same band: nothing moved, so the block may be reclaimed in
		// place — but NOT this instant. The terminal is reflowing right
		// now, and an event-timed redraw (probe/tick) landing mid-reflow
		// writes to a canvas that is still moving, shifting the block by
		// a row (user report: "when it wraps it creates another area to
		// write to"). Defer the repaint until the width settles (DECISIONS
		// #73) — the same hold the plain display has, without a frozen
		// block or restart.
		w.resizeSince = w.now()
		w.deferPending = true
		debugx.Debugf("resize", "%d→%d rows %d→%d → defer (in-place after settle)", old, tw, w.lastPhysRows, reflowed)
	}
}

// resizeRestart moves the block below the frozen (re-wrapped) rendering
// and resets the wrap bookkeeping to the fresh row. No-op unless a resize
// is pending (DECISIONS #67).
func (w *Window) resizeRestart() {
	if !w.resizePending {
		return
	}
	w.resizePending = false
	_, _ = fmt.Fprint(w.w, "\r\n")
	w.lastPhysRows = 1
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
