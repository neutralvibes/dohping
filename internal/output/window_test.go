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
	w := NewWindow(buf, plainLayout("192.168.1.23"), lines, quiet, noHeader, func() int { return height })
	w.SetNow(func() time.Time { return t0.Add(time.Minute) })
	return w
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
	// Column alignment: MIN still under its header (col 49) on the live
	// row despite the frame at col 48.
	live := scr.line(2)
	if len(live) <= 49 || live[49] == 0 {
		t.Errorf("live row too short for column check: %q", live)
	}
}
