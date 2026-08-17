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
	w := NewWindow(buf, plainLayout("192.168.1.23"), lines, quiet, noHeader, func() (int, int) { return width, height }, nil)
	w.SetNow(func() time.Time { return t0.Add(time.Minute) })
	return w
}

// newTestWindowResizable injects a MUTABLE terminal size: tests simulate a
// resize by changing the captured variables between redraws. The closure
// captures the parameter variables themselves, so mutating the returned
// pointers changes what the next Redraw sees.
func newTestWindowResizable(buf *bytes.Buffer, host string, lines int, quiet, noHeader bool, width, height int) (*Window, *int, *int) {
	w := NewWindow(buf, plainLayout(host), lines, quiet, noHeader, func() (int, int) { return width, height }, nil)
	w.SetNow(func() time.Time { return t0.Add(time.Minute) })
	return w, &width, &height
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
	if got := scr.line(0); got != "TIME      HOST            STATUS  DURATION       MIN     MAX     AVG     FAILS" {
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
	// The frame lives at column 47 (inside the DURATION padding) and the
	// separator at 48 stays a space — MIN keeps its column.
	if runes := []rune(scr.line(2)); len(runes) > 48 {
		if c := runes[47]; !strings.ContainsRune(frames, c) {
			t.Errorf("live frame not at column 47 (got %q): %q", c, scr.line(2))
		}
		if c := runes[48]; c != ' ' {
			t.Errorf("separator at col 48 = %q, want space: %q", c, scr.line(2))
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

// TestWindowResizeGrowBackClearsStaleRowsFullBlock: the stale-row clear
// must wipe the ENTIRE row, not just from the cursor's column — with a
// FULL block (history + live, no blank padding) the cursor sits at the end
// of the live line, and a clear without a column reset would leave the
// stale wrap tail's text behind (DECISIONS #65).
func TestWindowResizeGrowBackClearsStaleRowsFullBlock(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _ := newTestWindowResizable(&buf, "frigate.app.home", 2, false, false, 60, 24)
	*wPtr = 60
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

	buf.Reset()
	*wPtr = 120
	w.Tick()
	frame2 := buf.String()

	scr := newTermScreen(24, 60)
	scr.feed(frame1)
	scr.resize(120)
	scr.feed(frame2)
	// The block's last row (the live line) is non-blank; the stale rows
	// below it must be fully cleared.
	for r := 3; r < 8; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not cleared after grow-back (full block): %q", r, got)
		}
	}
	if !strings.HasPrefix(scr.line(0), "TIME") {
		t.Errorf("row 0 must be the header: %q", scr.line(0))
	}
}

// TestWindowResizeRetractsHostColumn: on a narrow terminal the HOST column
// retracts to fit (content-fit with terminal cap — user-approved
// 2026-08-17), the STATUS column follows, and growing back restores the
// original columns. Rendered through the emulator at both widths.
func TestWindowResizeRetractsHostColumn(t *testing.T) {
	var buf bytes.Buffer
	// "frigate.app.home" is 16 cells → column 16 at startup.
	w, wPtr, _ := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 120, 24)
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

	// Shrink to 81 (the minimum floor): HOST retracts to 15, STATUS moves
	// to col 26, and the line still fits — no wrap at the minimum.
	*wPtr = 81
	w.Tick()
	scr = newTermScreen(24, 81)
	scr.feed(buf.String())
	if runes := []rune(scr.line(1)); runes[statusCol(15)] != 'u' {
		t.Errorf("narrow: status not at col %d: %q", statusCol(15), scr.line(1))
	}
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("narrow: row %d not blank (block should be 6 rows at min width): %q", r, got)
		}
	}

	// Grow back to 120: columns restore, screen stays clean.
	*wPtr = 120
	w.Tick()
	scr = newTermScreen(24, 120)
	scr.feed(buf.String())
	if runes := []rune(scr.line(1)); runes[statusCol(16)] != 'u' {
		t.Errorf("grow-back: status not at col %d: %q", statusCol(16), scr.line(1))
	}
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("grow-back: row %d not blank: %q", r, got)
		}
	}
}

// TestWindowResizeMinWidthTruncatesHost: at the 81-cell minimum the HOST
// column is 15 cells and a long host shows the ellipsis; the truncation is
// cell-exact (rune-based, DECISIONS #64) so STATUS stays at col 26.
func TestWindowResizeMinWidthTruncatesHost(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindowSized(&buf, 5, false, false, 81, 24)
	w.layout = NewLayout("a-very-long-hostname.internal", "HH:MM:SS", nil) // 29 cells
	w.Handle(changeEvent(t0, state.StatusUp))

	scr := newTermScreen(24, 81)
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
// reported bug: a terminal narrower than the 81-cell minimum wraps every
// line, and the block must repaint as a coherent stack — one header, one
// live line, no interleaved fragments — even across repeated redraws.
// The old code counted LOGICAL rows for cursor movement, so a wrapped
// block's cursor-up landed mid-block and rows overwrote each other.
func TestWindowResizeBelowFloorWrapsCoherently(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _ := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 60, 24)
	*wPtr = 60
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	w.Tick() // a third frame: cumulative drift must not appear

	scr := newTermScreen(24, 60)
	scr.feed(buf.String())

	// Exactly one header, exactly one live line, nothing interleaved.
	if !strings.HasPrefix(scr.line(0), "TIME") {
		t.Errorf("row 0 must be the header: %q", scr.line(0))
	}
	if got := strings.Count(strings.Join(screenRows(scr), "\n"), "TIME"); got != 1 {
		t.Errorf("header appears %d times (fragmentation): rows %q", got, screenRows(scr))
	}
	if !strings.HasPrefix(scr.line(2), "11:00:35") {
		t.Errorf("live line must start at row 2 col 0 (wrapped after the header): row2=%q", scr.line(2))
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
	scr2 := newTermScreen(24, 60)
	scr2.feed(buf.String())
	if !strings.HasPrefix(scr2.line(0), "TIME") || !strings.HasPrefix(scr2.line(2), "11:00:35") {
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

// TestWindowResizeGrowBackClearsStaleRows: growing the terminal back after
// a narrow (wrapped) frame must clear the wrapped tails left below the
// block — the user's "going back to original width restores it" becomes
// automatic. The shrink-clear runs on PHYSICAL row counts.
func TestWindowResizeGrowBackClearsStaleRows(t *testing.T) {
	var buf bytes.Buffer
	w, wPtr, _ := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 60, 24)
	*wPtr = 60
	w.Handle(changeEvent(t0, state.StatusUp)) // wrapped: 8 physical rows

	*wPtr = 120
	w.Tick() // unwrapped: 6 physical rows → 2 stale rows must be cleared

	scr := newTermScreen(24, 120)
	scr.feed(buf.String())
	if runes := []rune(scr.line(1)); runes[statusCol(16)] != 'u' {
		t.Errorf("grow-back: status not at col %d: %q", statusCol(16), scr.line(1))
	}
	for r := 6; r < scr.rows; r++ {
		if got := scr.line(r); got != "" {
			t.Errorf("row %d not cleared after grow-back (stale wrap tail): %q", r, got)
		}
	}
}
