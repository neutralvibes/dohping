package output

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"dohping/internal/state"
)

func newTestWindow(buf *bytes.Buffer, lines int, quiet, noHeader bool, height int) *Window {
	// Width 0 = unknown (startup column policy) — the pre-resize behavior.
	return newTestWindowSized(buf, lines, quiet, noHeader, 0, height)
}

// newTestWindowSized injects a fixed terminal size. width 0 = unknown.
func newTestWindowSized(buf *bytes.Buffer, lines int, quiet, noHeader bool, width, height int) *Window {
	w := NewWindow(buf, plainLayout("192.168.1.23"), lines, quiet, noHeader, func() (int, int) { return width, height })
	w.SetNow(func() time.Time { return t0.Add(time.Minute) })
	return w
}

// newTestWindowResizable injects a MUTABLE terminal size AND clock: tests
// simulate a resize by changing the captured variables between redraws,
// and advance the clock to settle the resize freeze (DECISIONS #67). The
// closures capture the parameter variables themselves, so mutating the
// returned pointers changes what the next Redraw sees.
func newTestWindowResizable(buf *bytes.Buffer, host string, lines int, quiet, noHeader bool, width, height int) (*Window, *int, *int, *time.Time) {
	now := t0.Add(time.Minute)
	w := NewWindow(buf, plainLayout(host), lines, quiet, noHeader, func() (int, int) { return width, height })
	w.SetNow(func() time.Time { return now })
	return w, &width, &height, &now
}

// cursorUpRe matches in-place redraw markers: \x1b[<n>A (cursor up).
var cursorUpRe = regexp.MustCompile(`\x1b\[\d+A`)

// countRedraws counts cursor-up redraw markers in the output (the first
// frame has none — it is the initial draw in place).
func countRedraws(s string) int { return len(cursorUpRe.FindAllString(s, -1)) }

// termScreen is a minimal VT100-ish emulator for tests: enough to prove
// what a real terminal would show, which escape-stream assertions cannot.
// It handles printable text, CR/LF, cursor up/down, clear-to-EOL, and
// ignores SGR and other CSI sequences that do not move or clear cells.
type termScreen struct {
	rows, cols int
	cells      [][]rune
	r, c       int // cursor
}

func newTermScreen(rows, cols int) *termScreen {
	cells := make([][]rune, rows)
	for i := range cells {
		cells[i] = make([]rune, cols)
	}
	return &termScreen{rows: rows, cols: cols, cells: cells}
}

// feed renders one escape/control/text stream into the screen.
func (t *termScreen) feed(s string) {
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == '\x1b':
			// CSI: ESC [ params final
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				params := []int{}
				num := 0
				haveNum := false
				for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
					if s[j] >= '0' && s[j] <= '9' {
						num = num*10 + int(s[j]-'0')
						haveNum = true
					} else if s[j] == ';' {
						params = append(params, num)
						num = 0
						haveNum = false
					}
					j++
				}
				if j >= len(s) {
					break
				}
				final := s[j]
				if haveNum {
					params = append(params, num)
				}
				n := 1
				if len(params) > 0 && params[0] > 0 {
					n = params[0]
				}
				switch final {
				case 'A': // cursor up
					t.r -= n
					if t.r < 0 {
						t.r = 0
					}
				case 'B': // cursor down
					t.r += n
					if t.r >= t.rows {
						t.r = t.rows - 1
					}
				case 'K': // clear to end of line
					for cc := t.c; cc < t.cols; cc++ {
						t.cells[t.r][cc] = 0
					}
				case 'J': // clear below cursor (should never be emitted)
					for rr := t.r; rr < t.rows; rr++ {
						for cc := 0; cc < t.cols; cc++ {
							t.cells[rr][cc] = 0
						}
					}
				}
				i = j + 1
				continue
			}
			i++ // stray ESC; skip
		case ch == '\r':
			t.c = 0
			i++
		case ch == '\n':
			t.r++
			if t.r >= t.rows {
				t.r = t.rows - 1
			}
			i++
		case ch < 0x20:
			i++ // other control: ignore
		default:
			// DECAWM-style autowrap: at the right margin the next cell starts
			// a new physical row (proves below-floor wrap behavior — DECISIONS
			// #64). Placement is identical to a real terminal for the
			// sequences dohping emits.
			if t.c >= t.cols {
				t.r++
				if t.r >= t.rows {
					t.r = t.rows - 1
				}
				t.c = 0
			}
			// decode the rune
			r, size := decodeRune(s[i:])
			if t.r < t.rows && t.c < t.cols {
				t.cells[t.r][t.c] = r
			}
			t.c++
			i += size
		}
	}
}

func decodeRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 1
}

// line returns row r as a trimmed string (trailing NULs removed).
func (t *termScreen) line(r int) string {
	runes := t.cells[r]
	end := len(runes)
	for end > 0 && runes[end-1] == 0 {
		end--
	}
	return string(runes[:end])
}

// resize changes the terminal width WITHOUT reflowing existing content —
// the behavior of xterm/gnome-terminal-style terminals on resize (lines
// keep their cells; only new writes use the new width). Used to simulate
// a mid-run resize faithfully (DECISIONS #65); the cursor is left where
// it was, exactly as a non-reflowing terminal leaves it.
func (t *termScreen) resize(cols int) {
	if cols > t.cols {
		for i := range t.cells {
			t.cells[i] = append(t.cells[i], make([]rune, cols-t.cols)...)
		}
	} else if cols < t.cols {
		for i := range t.cells {
			t.cells[i] = t.cells[i][:cols]
		}
	}
	t.cols = cols
}

// TestWindowRenderedScreenClean is the regression test for DECISIONS #54:
// the visible screen after several redraws must be a clean fixed block —
// one header row, one live line, blank padding — with no stale fragments
// from earlier frames. The earlier bug wrote redraws at the cursor's
// preserved column (cursor-up without carriage return), which left
// fragments like "348.00  348.00  348.00" on screen.
func TestWindowRenderedScreenClean(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	// change to up, then a few probe successes with growing stats
	w.Handle(changeEvent(t0, state.StatusUp))
	for i := 1; i <= 4; i++ {
		w.Handle(successEvent(t0.Add(time.Duration(i)*time.Second), state.StatusUp,
			buildStats(1*time.Millisecond, 3*time.Millisecond, 2*time.Millisecond, i), 0))
	}
	scr := newTermScreen(10, 120)
	scr.feed(buf.String())

	// Row 0 must be exactly the header starting at column 0.
	if got := scr.line(0); got != "TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS" {
		t.Errorf("row 0 = %q, want clean header at column 0", got)
	}
	// Row 1 must be the live line, starting at column 0 (time at col 0).
	row1 := scr.line(1)
	if !strings.HasPrefix(row1, "11:00:35") {
		t.Errorf("row 1 = %q, want live line starting at column 0", row1)
	}
	if !strings.Contains(row1, "up") {
		t.Errorf("row 1 missing status: %q", row1)
	}
	// Rows 2-4 must be blank padding (window block of 5 data rows).
	for r := 2; r <= 4; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d = %q, want blank padding row", r, got)
		}
	}
	// Nothing below the block.
	for r := 5; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d = %q, want nothing below the window block", r, got)
		}
	}
	// The rendered screen has exactly one header row (row 0): each redraw
	// rewrites the block in place, so no other row may carry a header.
	for r := 1; r < scr.rows; r++ {
		if strings.HasPrefix(scr.line(r), "TIME") {
			t.Errorf("header fragment on row %d: %q", r, scr.line(r))
		}
	}
}

func TestWindowStaysOnNormalScreen(t *testing.T) {
	// DECISIONS #53: window mode must not clear or take over the screen.
	// Enter/Exit are no-ops; no alternate-screen, cursor-home, or
	// clear-to-end-of-screen sequences may ever appear.
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	w.Enter()
	if buf.Len() != 0 {
		t.Errorf("Enter wrote output: %q", buf.String())
	}
	buf.Reset()
	w.Exit()
	if buf.Len() != 0 {
		t.Errorf("Exit wrote output: %q", buf.String())
	}
	buf.Reset()
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(5*time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	out := buf.String()
	for _, seq := range []string{"\x1b[?1049h", "\x1b[?1049l", "\x1b[H", "\x1b[J"} {
		if strings.Contains(out, seq) {
			t.Errorf("forbidden screen-clearing sequence %q present: %q", seq, out)
		}
	}
	if !strings.Contains(out, "\x1b[K") {
		t.Errorf("expected per-row clear-to-EOL in in-place redraw: %q", out)
	}
}

func TestWindowRendersHeaderAndLiveLine(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	out := buf.String()
	if !strings.Contains(out, "TIME") {
		t.Errorf("header missing: %q", out)
	}
	if !strings.Contains(out, "up") {
		t.Errorf("live line missing: %q", out)
	}
	// The block occupies exactly header + visible rows: the first frame
	// starts at the cursor and never moves up.
	if countRedraws(out) != 0 {
		t.Errorf("first frame must not move the cursor up: %q", out)
	}
}

func TestWindowLiveUpdateRedraws(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(5*time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	if countRedraws(buf.String()) != 1 {
		t.Errorf("redraws = %d, want 1 cursor-up redraw: %q", countRedraws(buf.String()), buf.String())
	}
	if !strings.Contains(buf.String(), "0d 00:00:05") {
		t.Errorf("duration not updated: %q", buf.String())
	}
	// DECISIONS #54: the redraw must return to column 0 before rewriting,
	// or stale fragments remain on screen. Block = header + 5 rows = 6,
	// so the redraw moves up 5 rows, then carriage-returns.
	if !strings.Contains(buf.String(), "\x1b[5A\r") {
		t.Errorf("redraw missing carriage return after cursor-up (want \\x1b[5A\\r): %q", buf.String())
	}
}

func TestWindowRollingHistory(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 3, false, false, 24) // 2 history + 1 live
	// Four status changes: the first finalized line must drop.
	for i := 0; i < 4; i++ {
		t := t0.Add(time.Duration(i) * 10 * time.Second)
		st := state.StatusUp
		if i%2 == 1 {
			st = state.StatusDown
		}
		ev := state.Event{
			Kind: state.EventStatusChange, Time: t, Status: st,
			Duration: 10 * time.Second, Fails: 1,
		}
		w.Handle(ev)
	}
	// History is bounded to 2; the oldest (t0) has fallen off.
	last := w.history
	if len(last) != 2 {
		t.Fatalf("history = %d, want 2 (bounded)", len(last))
	}
	if last[0].Time.Equal(t0) {
		t.Errorf("oldest line not dropped: %v", last[0].Time)
	}
	if !last[0].Time.Equal(t0.Add(10 * time.Second)) {
		t.Errorf("newest history line wrong: %v", last[0].Time)
	}
	if !last[1].Time.Equal(t0.Add(20 * time.Second)) {
		t.Errorf("history tail wrong: %v", last[1].Time)
	}
	if w.cur == nil || !w.cur.Time.Equal(t0.Add(30*time.Second)) {
		t.Errorf("live line wrong: %+v", w.cur)
	}
}

func TestWindowStatusChangeFinalizes(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	downEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(30 * time.Second),
		Status: state.StatusDown, PrevStatus: state.StatusUp,
		Duration: 30 * time.Second, Fails: 1,
	}
	w.Handle(downEv)
	if len(w.history) != 1 {
		t.Fatalf("history = %d, want 1 finalized line", len(w.history))
	}
	if w.history[0].Duration != 30*time.Second {
		t.Errorf("finalized duration = %v, want 30s", w.history[0].Duration)
	}
	if w.cur == nil || w.cur.Status != state.StatusDown {
		t.Errorf("current line not reset to down: %+v", w.cur)
	}
}

func TestWindowQuietSuppresses(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, true, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Redraw()
	if buf.Len() != 0 {
		t.Errorf("quiet window wrote output: %q", buf.String())
	}
}

func TestWindowNoHeader(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, true, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	if strings.Contains(buf.String(), "TIME") {
		t.Errorf("header present despite noHeader: %q", buf.String())
	}
}

func TestWindowReducesLinesForSmallTerminal(t *testing.T) {
	// Terminal height 3, window 5 → visible = 2 data lines (1 header row
	// reserved), and the render trims history to fit.
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 3)
	w.Handle(changeEvent(t0, state.StatusUp))
	// Force 3 finalized + live by transitions (history capped at 4, then
	// render trims to visible-1=1).
	for i := 0; i < 4; i++ {
		ev := state.Event{
			Kind:     state.EventStatusChange,
			Time:     t0.Add(time.Duration(i+1) * 10 * time.Second),
			Status:   state.StatusUp,
			Duration: 10 * time.Second,
		}
		w.Handle(ev)
	}
	out := buf.String()
	if v := w.visibleLines(); v != 2 {
		t.Errorf("visibleLines = %d, want 2", v)
	}
	// Inspect only the LAST redraw (the buffer accumulates all redraws):
	// with height 3 and noHeader=false, the final frame is header + 1
	// history line + 1 live line = 2 newlines, anchored after a cursor-up.
	chunks := cursorUpRe.Split(out, -1)
	last := chunks[len(chunks)-1]
	if strings.Count(last, "\n") != 2 {
		t.Errorf("last frame has %d newlines, want 2 (header + 1 history): %q",
			strings.Count(last, "\n"), last)
	}
}

func TestWindowFinalizeOnShutdown(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Finalize()
	if len(w.history) != 1 {
		t.Fatalf("history = %d, want 1", len(w.history))
	}
	if w.history[0].Duration != time.Minute {
		t.Errorf("finalized duration = %v, want 1m (SetNow)", w.history[0].Duration)
	}
	// The summary cursor moves below the block.
	if !strings.HasSuffix(buf.String(), "\r\n") {
		t.Errorf("finalize must leave the cursor on a fresh line below the block: %q", buf.String())
	}
	// Idempotent.
	w.Finalize()
	if len(w.history) != 1 {
		t.Errorf("Finalize not idempotent: history = %d", len(w.history))
	}
}

func TestWindowHistoryCapAtLeastOne(t *testing.T) {
	// --window-lines 1: history cap clamps to 1, live line still renders.
	var buf bytes.Buffer
	w := newTestWindow(&buf, 1, false, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp))
	ev := state.Event{Kind: state.EventStatusChange, Time: t0.Add(time.Second), Status: state.StatusDown, Duration: time.Second}
	w.Handle(ev)
	if len(w.history) != 1 {
		t.Errorf("history = %d, want 1", len(w.history))
	}
	if w.cur == nil {
		t.Error("live line missing")
	}
}

func TestWindowLiveRowAnimatedHistoryStatic(t *testing.T) {
	// The liveness animation appears on the live row only; finalized
	// history rows are plain (user request 2026-08-17). Render through
	// the terminal emulator and check the visible screen.
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24)
	w.Handle(changeEvent(t0, state.StatusUp)) // live row, frame 0
	// Finalize the up line into history, then start a down live row.
	downEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(2 * time.Second),
		Status: state.StatusDown, PrevStatus: state.StatusUp,
		Duration: 2 * time.Second, Fails: 1,
	}
	w.Handle(downEv)
	// The animation advances on the 1-second ticker, not per event.
	w.Tick()

	scr := newTermScreen(10, 120)
	scr.feed(buf.String())
	frames := "▁▃▅▇"
	// Row 1 (first history row) must be static: no frame glyph.
	if strings.ContainsAny(scr.line(1), frames) {
		t.Errorf("history row animated: %q", scr.line(1))
	}
	// The live row (row 2) carries the animation frame.
	if !strings.ContainsAny(scr.line(2), frames) {
		t.Errorf("live row missing animation frame: %q", scr.line(2))
	}
	// The frame lives at column 45 (inside the DURATION padding) and the
	// separator at 46 stays a space — MIN keeps its column.
	if runes := []rune(scr.line(2)); len(runes) > 46 {
		if c := runes[45]; !strings.ContainsRune(frames, c) {
			t.Errorf("live frame not at column 45 (got %q): %q", c, scr.line(2))
		}
		if c := runes[46]; c != ' ' {
			t.Errorf("separator at col 46 = %q, want space: %q", c, scr.line(2))
		}
	}
	// Tick advances the frame: another tick changes the glyph.
	before := scr.line(2)
	buf.Reset()
	w.Tick()
	scr2 := newTermScreen(10, 120)
	scr2.feed(buf.String())
	if scr2.line(2) == before {
		t.Errorf("window Tick did not advance the animation frame")
	}
}

// TestWindowTickRefreshesDuration: the 1-second tick must refresh the
// live row's DURATION from the wall clock (now = t0+1min in the test
// window), matching the plain display (user report 2026-08-17).
func TestWindowTickRefreshesDuration(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindow(&buf, 5, false, false, 24) // now = t0+1min
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Tick()

	scr := newTermScreen(10, 120)
	scr.feed(buf.String())
	if !strings.Contains(scr.line(1), "0d 00:01:00") {
		t.Errorf("window tick did not refresh duration to 1m: %q", scr.line(1))
	}
}

// statusCol returns the column where the status value starts on the given
// rendered row: TIME(8) + 2sp + HOST(hostWidth) + 1sp.
func statusCol(hostWidth int) int { return 8 + 2 + hostWidth + 1 }

// TestWindowResizeGrowBackKeepsFrozenBlock: with a FULL block (history +
// live, no blank padding) a grow-back resize must NOT reclaim the old
// rendering — the block freezes and, once the width settles, restarts on a
// fresh row below it (DECISIONS #67). Everything below the fresh block
// must be clean.
func TestWindowResizeGrowBackKeepsFrozenBlock(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 2, false, false, 40, 24)
	*wPtr = 40
	// Fill the block: one finalized history line + live (window-lines 2 →
	// rows = header + 2 data, no padding).
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	upEv := state.Event{
		Kind: state.EventStatusChange, Time: t0.Add(4 * time.Second),
		Status: state.StatusUp, PrevStatus: state.StatusUp,
		Duration: 4 * time.Second, PrevStats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1),
	}
	w.Handle(upEv) // history 1 + live 1 — full block
	frame1 := buf.String()

	*wPtr = 120
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	w.Tick()
	if buf.Len() != 0 {
		t.Fatalf("mid-settle redraw produced output: %q", buf.String())
	}
	*now = now.Add(time.Second)
	buf.Reset()
	w.Tick()
	frame2 := buf.String()

	scr := newTermScreen(24, 40)
	scr.feed(frame1)
	scr.resize(120)
	scr.feed(frame2)
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	// Exactly two blocks: the frozen one and the fresh one below it.
	if len(headers) != 2 {
		t.Fatalf("header rows = %v, want exactly 2 (frozen block + fresh block)", headers)
	}
	if headers[1] <= headers[0] {
		t.Fatalf("fresh block must start below the frozen block: %v", headers)
	}
	// Fresh block = header + 2 data rows at the new width: nothing below.
	for r := headers[1] + 3; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the fresh block: %q", r, got)
		}
	}
}

// TestWindowResizeRetractsHostColumn: on a narrow terminal the HOST column
// retracts to fit (content-fit with terminal cap — user-approved
// 2026-08-17), the STATUS column follows, and growing back restores the
// original columns. A resize now freezes and restarts the block below
// (DECISIONS #67); the NEW block uses the new width's columns. Rendered
// through the emulator at both widths (each frame starts at the cursor
// origin, so the restart CRLF places the block's header at row 1).
func TestWindowResizeRetractsHostColumn(t *testing.T) {
	var buf bytes.Buffer
	// "frigate.app.home" is 16 cells → column 16 at startup.
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 120, 24)
	*wPtr = 120
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))

	// Wide terminal: HOST keeps its content width (16), STATUS at col 27.
	scr := newTermScreen(24, 120)
	scr.feed(buf.String())
	if runes := []rune(scr.line(1)); runes[statusCol(16)] != 'u' {
		t.Errorf("wide: status not at col %d: %q", statusCol(16), scr.line(1))
	}

	// Shrink to 79 (the minimum floor): same wrap band — the line fits in
	// one row at both widths, so a reflow cannot move the block and it
	// repaints IN PLACE with HOST retracted to 15 and STATE at col 26: no
	// freeze, no frozen duplicate (DECISIONS #70 makes the freeze
	// conditional on a wrap-boundary crossing).
	*wPtr = 79
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	w.Tick()
	repaint := buf.String()
	if repaint == "" || strings.HasPrefix(repaint, "\r\n") {
		t.Fatalf("same-band shrink must repaint in place (no freeze/restart): %q", repaint)
	}
	scr = newTermScreen(24, 79)
	scr.feed(repaint)
	if !strings.HasPrefix(scr.line(0), "TIME") {
		t.Errorf("narrow: in-place header not at row 0: row0=%q", scr.line(0))
	}
	if runes := []rune(scr.line(1)); runes[statusCol(15)] != 'u' {
		t.Errorf("narrow: status not at col %d: %q", statusCol(15), scr.line(1))
	}
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("narrow: row %d not blank (block is 6 rows at min width): %q", r, got)
		}
	}

	// Grow back to 120: same band again — an in-place repaint restores
	// the column layout (HOST 16, STATE at col 27), header still at row 0.
	*wPtr = 120
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	w.Tick()
	repaint = buf.String()
	if repaint == "" || strings.HasPrefix(repaint, "\r\n") {
		t.Fatalf("same-band grow-back must repaint in place: %q", repaint)
	}
	scr = newTermScreen(24, 120)
	scr.feed(repaint)
	if !strings.HasPrefix(scr.line(0), "TIME") {
		t.Errorf("grow-back: in-place header not at row 0: row0=%q", scr.line(0))
	}
	if runes := []rune(scr.line(1)); runes[statusCol(16)] != 'u' {
		t.Errorf("grow-back: status not at col %d: %q", statusCol(16), scr.line(1))
	}
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("grow-back: row %d not blank: %q", r, got)
		}
	}
}

// TestWindowResizeMinWidthTruncatesHost: at the 79-cell minimum the HOST
// column is 15 cells and a long host shows the ellipsis; the truncation is
// cell-exact (rune-based, DECISIONS #64) so STATE stays at col 26.
func TestWindowResizeMinWidthTruncatesHost(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindowSized(&buf, 5, false, false, 79, 24)
	w.layout = NewLayout("a-very-long-hostname.internal", "HH:MM:SS", nil) // 29 cells
	w.Handle(changeEvent(t0, state.StatusUp))

	scr := newTermScreen(24, 79)
	scr.feed(buf.String())
	row := scr.line(1)
	if !strings.Contains(row, "…") {
		t.Errorf("long host not truncated at min width: %q", row)
	}
	runes := []rune(row)
	// 14 cells of host + ellipsis fills the 15-wide field exactly.
	if string(runes[10:24]) != "a-very-long-ho" {
		t.Errorf("host prefix wrong: %q", string(runes[10:24]))
	}
	if runes[24] != '…' {
		t.Errorf("ellipsis not at col 24: %q", row)
	}
	if runes[statusCol(15)] != 'u' {
		t.Errorf("status not at col %d after truncation: %q", statusCol(15), row)
	}
}

// TestWindowResizeBelowFloorWrapsCoherently is the regression test for the
// reported bug: a terminal narrower than the essentials floor (47 cells —
// columns dropped first per DECISIONS #71) wraps every line, and the block
// must repaint as a coherent stack — one header, one live line, no
// interleaved fragments — even across repeated redraws. The old code
// counted LOGICAL rows for cursor movement, so a wrapped block's cursor-up
// landed mid-block and rows overwrote each other.
func TestWindowResizeBelowFloorWrapsCoherently(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, _ := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 40, 24)
	*wPtr = 40
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	w.Tick() // a third frame: cumulative drift must not appear

	scr := newTermScreen(24, 40)
	scr.feed(buf.String())

	// Exactly one header (40 cells, fits in one row at 40), exactly one
	// live line (46 cells with its animation frame — wraps to rows 1-2),
	// nothing interleaved.
	if !strings.HasPrefix(scr.line(0), "TIME") {
		t.Errorf("row 0 must be the header: %q", scr.line(0))
	}
	if got := strings.Count(strings.Join(screenRows(scr), "\n"), "TIME"); got != 1 {
		t.Errorf("header appears %d times (fragmentation): rows %q", got, screenRows(scr))
	}
	if !strings.HasPrefix(scr.line(1), "11:00:35") {
		t.Errorf("live line must start at row 1 col 0 (wrapped tail on row 2): row1=%q", scr.line(1))
	}
	// No repeated rows stacked on one screen line (the old bug signature:
	// "down ... 1      15:54:45 ... down" fragments on a single row).
	for r := 0; r < scr.rows; r++ {
		if strings.Count(scr.line(r), "11:00:35") > 1 {
			t.Errorf("row %d carries two timestamps (interleaved rows): %q", r, scr.line(r))
		}
	}

	// One more redraw must leave the same coherent screen (no cumulative
	// drift — the old bug got worse with every repaint).
	buf.Reset()
	w.Tick()
	scr2 := newTermScreen(24, 40)
	scr2.feed(buf.String())
	if !strings.HasPrefix(scr2.line(0), "TIME") || !strings.HasPrefix(scr2.line(1), "11:00:35") {
		t.Errorf("repeat redraw broke coherence: rows %q", screenRows(scr2))
	}
}

// screenRows renders all rows of a termScreen joined by newlines.
func screenRows(scr *termScreen) []string {
	rows := make([]string, scr.rows)
	for r := range rows {
		rows[r] = scr.line(r)
	}
	return rows
}

// TestWindowResizeGrowBackKeepsFrozenRows: growing the terminal back after
// a narrow (wrapped) frame — the block freezes at the old width and
// restarts below once settled; nothing below the fresh block may remain
// (DECISIONS #67 — the in-place reclaim is gone).
func TestWindowResizeGrowBackKeepsFrozenRows(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 40, 24)
	*wPtr = 40
	w.Handle(changeEvent(t0, state.StatusUp)) // wrapped at 40 (below the 47-cell floor)
	frame1 := buf.String()

	*wPtr = 120
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	w.Tick() // frozen mid-settle
	if buf.Len() != 0 {
		t.Fatalf("mid-settle redraw produced output: %q", buf.String())
	}
	*now = now.Add(time.Second)
	buf.Reset()
	w.Tick()
	frame2 := buf.String()

	scr := newTermScreen(24, 40)
	scr.feed(frame1)
	scr.resize(120)
	scr.feed(frame2)
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	// Exactly two blocks: the frozen one and the fresh one below it.
	if len(headers) != 2 || headers[1] <= headers[0] {
		t.Fatalf("want frozen block + fresh block below: header rows %v", headers)
	}
	// The fresh block uses the restored column layout (HOST 16).
	if runes := []rune(scr.line(headers[1] + 1)); runes[statusCol(16)] != 'u' {
		t.Errorf("grow-back: status not at col %d: %q", statusCol(16), scr.line(headers[1]+1))
	}
	// Fresh block = header + 5 data rows at the new width.
	for r := headers[1] + 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the fresh block: %q", r, got)
		}
	}
}

// TestWindowResizeFreezeThenRestart: on a width change the block freezes
// (no redraws) until the width is stable for resizeSettleDelay, then
// restarts on a fresh row below the frozen block (DECISIONS #67).
func TestWindowResizeFreezeThenRestart(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 40, 24)
	*wPtr = 40
	w.Handle(changeEvent(t0, state.StatusUp))
	frame1 := buf.String()

	*wPtr = 100
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	w.Tick()
	if buf.Len() != 0 {
		t.Fatalf("mid-settle redraw produced output: %q", buf.String())
	}
	*now = now.Add(time.Second)
	buf.Reset()
	w.Tick()
	restart := buf.String()
	if !strings.HasPrefix(restart, "\r\n") {
		t.Errorf("restart must begin with CRLF to move below the frozen block: %q", restart)
	}

	scr := newTermScreen(24, 40)
	scr.feed(frame1)
	scr.resize(100)
	scr.feed(restart)
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	// Exactly two blocks: the frozen one and the fresh one below it.
	if len(headers) != 2 || headers[1] <= headers[0] {
		t.Fatalf("want frozen block + fresh block below: header rows %v", headers)
	}
	// Fresh block = header + 5 data rows at the new width.
	for r := headers[1] + 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the fresh block: %q", r, got)
		}
	}
}

// TestWindowResizeDragKeepsFreezing: a resize drag restarts the settle
// clock on every width change — the block stays frozen while the width
// keeps moving and restarts exactly once after the width has been stable
// for resizeSettleDelay (user-requested acceptance check, DECISIONS #67).
func TestWindowResizeDragKeepsFreezing(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 40, 24)
	*wPtr = 40
	w.Handle(changeEvent(t0, state.StatusUp))
	buf.Reset()

	type step struct {
		width int
		adv   time.Duration
		want  string // "" = frozen, "restart" = fresh-row restart emitted
	}
	steps := []step{
		{60, 100 * time.Millisecond, ""},        // drag: 40 → 60 (crosses the 47 floor)
		{35, 100 * time.Millisecond, ""},        // drag: 60 → 35 (clock restarts)
		{60, 100 * time.Millisecond, ""},        // drag: 35 → 60 (clock restarts)
		{60, 200 * time.Millisecond, ""},        // 200ms after the LAST change: still settling
		{60, 100 * time.Millisecond, "restart"}, // 300ms of stability: restart fires
	}
	var restartOut string
	for i, s := range steps {
		*wPtr = s.width
		*now = now.Add(s.adv)
		buf.Reset()
		w.Tick()
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

// TestWindowResizeSameBandRepaintsInPlace: a width change that leaves every
// line's physical row count unchanged (60 → 55: with column trimming the
// line fits in one row at both widths — DECISIONS #71) cannot move the
// block in a reflow, so it must repaint IN PLACE — no freeze, no fresh-row
// restart, no frozen block left in scrollback (DECISIONS #70). Regression
// for the user's window-mode resize artifacts.
func TestWindowResizeSameBandRepaintsInPlace(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 60, 24)
	*wPtr = 60
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	frame1 := buf.String()

	*wPtr = 55
	*now = now.Add(100 * time.Millisecond) // mid-settle time: must NOT matter
	buf.Reset()
	w.Tick()
	repaint := buf.String()
	if repaint == "" {
		t.Fatal("same-band resize must repaint in place, not freeze")
	}
	if strings.HasPrefix(repaint, "\r\n") {
		t.Fatalf("same-band resize must NOT restart on a fresh row: %q", repaint)
	}

	scr := newTermScreen(24, 60)
	scr.feed(frame1)
	scr.resize(55)
	scr.feed(repaint)
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	if len(headers) != 1 {
		t.Fatalf("same-band resize must keep exactly one block: header rows %v", headers)
	}
	if !strings.HasPrefix(scr.line(1), "11:00:35") {
		t.Errorf("live line not at row 1 col 0 after same-band repaint: %q", scr.line(1))
	}
}

// TestWindowResizeSameBandWrappedInPlace: terminal BELOW the essentials
// floor (the live line wraps at both widths — 46 cells with its animation
// frame vs 40/45 cols; the header fits at 40+), resized within the same
// wrap band. Must repaint in place, wrapped and coherent, with exactly one
// block (no frozen duplicate).
func TestWindowResizeSameBandWrappedInPlace(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, _ := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 40, 24)
	*wPtr = 40
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	frame1 := buf.String()

	*wPtr = 45
	buf.Reset()
	w.Tick()
	repaint := buf.String()
	if repaint == "" || strings.HasPrefix(repaint, "\r\n") {
		t.Fatalf("below-floor same-band resize must repaint in place: %q", repaint)
	}

	scr := newTermScreen(24, 40)
	scr.feed(frame1)
	scr.resize(45)
	scr.feed(repaint)
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	if len(headers) != 1 {
		t.Fatalf("wrapped same-band resize must keep exactly one block: header rows %v", headers)
	}
	if !strings.HasPrefix(scr.line(1), "11:00:35") {
		t.Errorf("live line not at row 1 col 0 (wrapped tail on row 2): %q", scr.line(1))
	}
}

// TestWindowResizeSameBandDragNoFreeze: a drag confined to one wrap band
// repaints continuously — every step produces an in-place redraw and not
// a single freeze/restart (the old unconditional behavior froze on every
// step of the drag, stacking a block per width).
func TestWindowResizeSameBandDragNoFreeze(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 60, 24)
	*wPtr = 60
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))

	for _, width := range []int{55, 65, 60} {
		*wPtr = width
		*now = now.Add(100 * time.Millisecond)
		buf.Reset()
		w.Tick()
		got := buf.String()
		if got == "" || strings.HasPrefix(got, "\r\n") {
			t.Fatalf("same-band drag step %d must repaint in place: %q", width, got)
		}
	}
}

// TestWindowResizeCrossesBandFreezes: the conditional freeze must still
// fire when the wrap band changes (45 → 60: the 46-cell live line wraps at
// 45, fits at 60) — a reflow moves the block then, and reclaiming it would
// corrupt the screen (the #67 contract, now band-scoped; above the floor
// columns drop instead, so crossings only exist below it — DECISIONS #71).
func TestWindowResizeCrossesBandFreezes(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 45, 24)
	*wPtr = 45
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	frame1 := buf.String()

	*wPtr = 60
	*now = now.Add(100 * time.Millisecond)
	buf.Reset()
	w.Tick()
	if buf.Len() != 0 {
		t.Fatalf("band-crossing resize must freeze mid-settle: %q", buf.String())
	}
	*now = now.Add(time.Second)
	buf.Reset()
	w.Tick()
	restart := buf.String()
	if !strings.HasPrefix(restart, "\r\n") {
		t.Fatalf("band-crossing restart must begin with CRLF: %q", restart)
	}

	scr := newTermScreen(24, 45)
	scr.feed(frame1)
	scr.resize(60)
	scr.feed(restart)
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	if len(headers) != 2 || headers[1] <= headers[0] {
		t.Fatalf("band-crossing resize must leave frozen block + fresh block below: header rows %v", headers)
	}
}

// TestWindowTrimAtNarrowWidth: below the 79-cell line width the window
// drops rightmost columns instead of wrapping (DECISIONS #71) — at 60 cols
// the line is 55 cells (TIME/HOST/STATE/DURATION/MIN: FAILS, AVG and MAX
// dropped), the block is a single row per line, and the screen is one
// clean unwrapped block with the live line directly under the header.
func TestWindowTrimAtNarrowWidth(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _, _ := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 60, 24)
	*wPtr = 60
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	w.Tick()

	scr := newTermScreen(24, 60)
	scr.feed(buf.String())
	rows := screenRows(scr)
	var headers []int
	for r, ln := range rows {
		if strings.HasPrefix(ln, "TIME") {
			headers = append(headers, r)
		}
	}
	if len(headers) != 1 {
		t.Fatalf("want exactly one block: header rows %v", headers)
	}
	// No wrapping: header at row 0, live line at row 1.
	if !strings.HasPrefix(scr.line(0), "TIME") || !strings.HasPrefix(scr.line(1), "11:00:35") {
		t.Fatalf("trimmed block must be unwrapped: row0=%q row1=%q", scr.line(0), scr.line(1))
	}
	// Rightmost columns gone: header ends at MIN (retained 1).
	hdr := scr.line(0)
	if !strings.HasSuffix(hdr, "MIN") || strings.Contains(hdr, "FAILS") ||
		strings.Contains(hdr, "MAX") || strings.Contains(hdr, "AVG") {
		t.Errorf("header at 60 cols must show only TIME/HOST/STATE/DURATION/MIN: %q", hdr)
	}
	// Block = header + 5 data rows, nothing below.
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not blank below the trimmed block: %q", r, got)
		}
	}
}
