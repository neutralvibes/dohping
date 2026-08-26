package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"dohping/internal/output"
	"dohping/internal/ping"
	"dohping/internal/state"
)

var noTTY = TTY{Stdout: false, Stdin: false}

// fixed time for deterministic tests: 2026-08-16 11:00:35 +01:00
var t0 = time.Date(2026, 8, 16, 11, 0, 35, 0, time.FixedZone("BST", 3600))

func TestMainHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--help"}, &out, &errb, noTTY)
	if code != ExitOK {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "dohping [options] HOST") {
		t.Errorf("help output missing usage: %q", out.String())
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
}

func TestMainVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-V"}} {
		var out, errb bytes.Buffer
		code := Main(args, &out, &errb, noTTY)
		if code != ExitOK {
			t.Errorf("Main(%q) exit = %d, want 0", args, code)
		}
		if !strings.HasPrefix(out.String(), "dohping ") {
			t.Errorf("version output = %q, want prefix \"dohping \"", out.String())
		}
	}
}

func TestMainMissingHost(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main(nil, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "missing required HOST") {
		t.Errorf("stderr = %q, want missing-host message", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on usage error: %q", out.String())
	}
}

func TestMainInvalidValue(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--window-lines", "0", "h"}, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "window-lines") {
		t.Errorf("stderr = %q, want window-lines message", errb.String())
	}
}

func TestMainConflict(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--no-window", "--window-lines", "5", "h"}, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "conflict") {
		t.Errorf("stderr = %q, want conflict message", errb.String())
	}
}

// noOnlcrScreen models a terminal that does NOT translate LF to CRLF:
// \n moves down preserving the column, only \r returns to column 0. This
// is the environment that exposed the exit-summary drift.
type noOnlcrScreen struct {
	rows, cols int
	cells      [][]rune
	r, c       int
}

func newNoOnlcrScreen(rows, cols int) *noOnlcrScreen {
	cells := make([][]rune, rows)
	for i := range cells {
		cells[i] = make([]rune, cols)
	}
	return &noOnlcrScreen{rows: rows, cols: cols, cells: cells}
}

// feed renders a control/text stream. Handles \r (column 0), \n (down,
// column PRESERVED — no ONLCR), printable text; \x1b[K (clear-to-EOL) and
// other CSI sequences are skipped (they neither move the cursor nor matter
// for column assertions).
func (s *noOnlcrScreen) feed(str string) {
	i := 0
	for i < len(str) {
		ch := str[i]
		switch {
		case ch == '\x1b':
			if i+1 < len(str) && str[i+1] == '[' {
				j := i + 2
				for j < len(str) && (str[j] < 0x40 || str[j] > 0x7e) {
					j++
				}
				i = j + 1
				continue
			}
			i++
		case ch == '\r':
			s.c = 0
			i++
		case ch == '\n':
			s.r++
			if s.r >= s.rows {
				s.r = s.rows - 1
			}
			i++
		case ch < 0x20:
			i++
		default:
			r, size := decodeRune(str[i:])
			if s.r < s.rows && s.c < s.cols {
				s.cells[s.r][s.c] = r
			}
			s.c++
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

// line returns row r with trailing NULs stripped.
func (s *noOnlcrScreen) line(r int) string {
	runes := s.cells[r]
	end := len(runes)
	for end > 0 && runes[end-1] == 0 {
		end--
	}
	return string(runes[:end])
}

// TestExitSummaryRenderedAtColumnZero is the regression test for the
// user's "summary loses formatting" report: after --count exhaustion the
// live line finalizes and the summary must print as a clean left-aligned
// block at column 0 — even on a terminal without ONLCR where a bare LF
// leaves the cursor mid-line. The earlier code ended the finalized line
// with bare \n and wrote the summary with bare \n, so every line drifted
// progressively right until the terminal wrapped.
func TestExitSummaryRenderedAtColumnZero(t *testing.T) {
	var buf bytes.Buffer
	layout := output.NewLayout("google.com", "HH:MM:SS", nil)
	d := output.NewDisplay(&buf, layout, false, false, true, nil) // live, no sizeFn
	d.SetNow(func() time.Time { return t0.Add(time.Minute) })

	// A status change + probe successes (the -c N run), then shutdown.
	d.Handle(state.Event{Kind: state.EventStatusChange, Time: t0, Status: state.StatusUp})
	for i := 1; i <= 4; i++ {
		d.Handle(state.Event{
			Kind: state.EventProbeSuccess, Time: t0.Add(time.Duration(i) * time.Second),
			Status: state.StatusUp, RTT: time.Millisecond,
			Stats: state.Stats{Count: i, Min: time.Millisecond, Max: 3 * time.Millisecond, Sum: time.Duration(2*i) * time.Millisecond},
		})
	}
	d.Finalize()

	// Engine with real totals for the summary.
	eng := state.New(1, 1)
	eng.Step(ping.Result{Outcome: ping.OutcomeUp, RTT: 5 * time.Millisecond}, t0)
	eng.Step(ping.Result{Outcome: ping.OutcomeUp, RTT: 7 * time.Millisecond}, t0.Add(time.Second))
	printSummary(&buf, "google.com", eng, 4*time.Second)

	// Render the full stream (live updates + finalize + summary) through
	// a no-ONLCR terminal and assert the VISIBLE screen: the summary must
	// be a clean block starting at column 0, not drifting right.
	scr := newNoOnlcrScreen(20, 120)
	scr.feed(buf.String())

	var summaryRows []string
	for r := 0; r < scr.rows; r++ {
		ln := scr.line(r)
		if strings.Contains(ln, "dohping summary") || strings.HasPrefix(ln, "host:") ||
			strings.HasPrefix(ln, "current status:") || strings.HasPrefix(ln, "run duration:") ||
			strings.HasPrefix(ln, "total probes:") || strings.HasPrefix(ln, "successful:") ||
			strings.HasPrefix(ln, "failed:") || strings.HasPrefix(ln, "loss:") {
			summaryRows = append(summaryRows, ln)
		}
	}
	if len(summaryRows) != 8 {
		t.Fatalf("rendered summary rows = %d, want 8: %q", len(summaryRows), summaryRows)
	}
	// Every summary row must start at column 0 (no leading drift).
	for i, ln := range summaryRows {
		if strings.HasPrefix(ln, " ") {
			t.Errorf("summary row %d drifts right: %q", i, ln)
		}
	}
	// The marker row must be exactly the marker at column 0.
	if summaryRows[0] != "--- dohping summary ---" {
		t.Errorf("marker row = %q, want clean marker at column 0", summaryRows[0])
	}
	if !strings.HasPrefix(summaryRows[1], "host:            google.com") {
		t.Errorf("host row = %q, want left-aligned label", summaryRows[1])
	}
}
