package output

import (
	"strings"
	"testing"
	"time"

	"dohping/internal/state"
)

// fixed time for deterministic golden output: 2026-08-16 11:00:35 +01:00
var t0 = time.Date(2026, 8, 16, 11, 0, 35, 0, time.FixedZone("BST", 3600))

func plainLayout(host string) *Layout { return NewLayout(host, "HH:MM:SS", nil) }

// buildStats constructs Stats with the given min/max/avg over count samples.
func buildStats(min, max, avg time.Duration, count int) state.Stats {
	return state.Stats{Count: count, Min: min, Max: max, Sum: avg * time.Duration(count)}
}

func TestHeaderGolden(t *testing.T) {
	got := plainLayout("192.168.1.23").Header()
	want := "TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS"
	if got != want {
		t.Errorf("header mismatch:\n got: %q\nwant: %q", got, want)
	}
	if strings.TrimRight(got, " ") != got {
		t.Error("header has trailing whitespace")
	}
}

func TestUpLineGolden(t *testing.T) {
	ln := Line{
		Time:     t0,
		Status:   state.StatusUp,
		Duration: 35*time.Minute + 26*time.Second,
		Stats:    buildStats(1700*time.Microsecond, 5900*time.Microsecond, 2700*time.Microsecond, 3),
	}
	got := plainLayout("192.168.1.23").FormatLine(ln)
	// Column-starts (0-based, DECISIONS #68): TIME@0 HOST@10 STATE@26
	// DURATION@32 MIN@47 MAX@55 AVG@63 FAILS@71.
	// Values are left-aligned in their fields, starting directly under the
	// header labels — this line is byte-identical to the spec §7.4 example.
	want := "11:00:35  192.168.1.23    up" + strings.Repeat(" ", 4) + "0d 00:35:26" +
		strings.Repeat(" ", 4) + "1.70" + strings.Repeat(" ", 4) + "5.90" +
		strings.Repeat(" ", 4) + "2.70"
	if got != want {
		t.Errorf("up line mismatch:\n got: %q\nwant: %q", got, want)
	}
	// Alignment invariants: values under their header labels.
	for _, col := range []struct {
		s string
		i int
	}{
		{"1.70", 47}, {"5.90", 55}, {"2.70", 63},
	} {
		if !strings.HasPrefix(got[col.i:], col.s) {
			t.Errorf("column %q not at %d in %q", col.s, col.i, got)
		}
	}
}

func TestDownLineGolden(t *testing.T) {
	ln := Line{
		Time:     time.Date(2026, 8, 16, 11, 5, 23, 0, time.FixedZone("BST", 3600)),
		Status:   state.StatusDown,
		Duration: 65 * time.Second,
		Fails:    23,
	}
	got := plainLayout("192.168.1.23").FormatLine(ln)
	// FAILS left-aligned under its header at column 71.
	want := "11:05:23  192.168.1.23    down" + strings.Repeat(" ", 2) + "0d 00:01:05" + strings.Repeat(" ", 28) + "23"
	if got != want {
		t.Errorf("down line mismatch:\n got: %q\nwant: %q", got, want)
	}
	if !strings.HasPrefix(got[71:], "23") {
		t.Errorf("FAILS value not aligned at column 71: %q", got)
	}
}

func TestErrorLineGolden(t *testing.T) {
	ln := Line{
		Time:     time.Date(2026, 8, 16, 13, 0, 0, 0, time.FixedZone("BST", 3600)),
		Status:   state.StatusError,
		Duration: 3 * time.Second,
	}
	got := plainLayout("192.168.1.23").FormatLine(ln)
	want := "13:00:00  192.168.1.23    error 0d 00:00:03"
	if got != want {
		t.Errorf("error line mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestUnknownLineGolden(t *testing.T) {
	// The never-established status renders as "?" in the table (DECISIONS
	// #68): it must fit the 5-cell STATE field so the line stays 79 wide.
	// The word "unknown" survives in prose contexts (the exit summary).
	ln := Line{
		Time:     t0,
		Status:   state.StatusUnknown,
		Duration: 1 * time.Second,
	}
	layout := plainLayout("192.168.1.23")
	got := layout.FormatLine(ln)
	want := "11:00:35  192.168.1.23    ?" + strings.Repeat(" ", 5) + "0d 00:00:01"
	if got != want {
		t.Errorf("unknown line mismatch:\n got: %q\nwant: %q", got, want)
	}
	if layout.FullWidth() != 79 {
		t.Errorf("FullWidth = %d, want 79 (under 80)", layout.FullWidth())
	}
}

func TestHostWidthMin15(t *testing.T) {
	// A short host still gets the 15-char column (header parity).
	got := plainLayout("a").Header()
	want := "TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS"
	if got != want {
		t.Errorf("min-width header mismatch:\n got: %q\nwant: %q", got, want)
	}
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got = plainLayout("a").FormatLine(ln)
	// "a" in a 15-wide field + 1 sep → 15 spaces before "up"; RTT values
	// left-aligned under their headers.
	want = "11:00:35  a" + strings.Repeat(" ", 15) + "up" +
		strings.Repeat(" ", 4) + "0d 00:00:01" +
		strings.Repeat(" ", 4) + "1.00" +
		strings.Repeat(" ", 4) + "1.00" +
		strings.Repeat(" ", 4) + "1.00"
	if got != want {
		t.Errorf("min-width line mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestHostWidthWidensForLongHost(t *testing.T) {
	// A long host (incl. IPv6 literals) widens the column; width is fixed
	// for the run.
	host := "2001:0db8:85a3:0000:0000:8a2e:0370:7334" // 39 chars
	layout := plainLayout(host)
	got := layout.Header()
	// HOST field 39 wide + 1 sep → 36 spaces between "HOST" and "STATE".
	want := "TIME      HOST" + strings.Repeat(" ", 36) + "STATE DURATION       MIN     MAX     AVG     FAILS"
	if got != want {
		t.Errorf("wide header mismatch:\n got: %q\nwant: %q", got, want)
	}
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got = layout.FormatLine(ln)
	// STATE column must sit at 26 + (39-15) = 50.
	if !strings.HasPrefix(got[50:], "up") {
		t.Errorf("status not at widened column 50: %q", got)
	}
}

func TestHostTruncationOver40(t *testing.T) {
	long := strings.Repeat("x", 41)
	layout := plainLayout(long)
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got := layout.FormatLine(ln)
	// Host truncated to 40: 39 x's + "…", then 1 sep, then "up".
	want := "11:00:35  " + strings.Repeat("x", 39) + "…" + " " + "up" +
		strings.Repeat(" ", 4) + "0d 00:00:01" +
		strings.Repeat(" ", 4) + "1.00" +
		strings.Repeat(" ", 4) + "1.00" +
		strings.Repeat(" ", 4) + "1.00"
	if got != want {
		t.Errorf("truncation mismatch:\n got: %q\nwant: %q", got, want)
	}
	// Host column exactly 40 wide: "…" ends at index 49.
	if !strings.HasPrefix(got[49:], "…") {
		t.Errorf("ellipsis not at column 49: %q", got)
	}
}

func TestDurationCap99d(t *testing.T) {
	ln := Line{Time: t0, Status: state.StatusUp, Duration: 100 * 24 * time.Hour, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got := plainLayout("h").FormatLine(ln)
	if !strings.Contains(got, "99d+") {
		t.Errorf("duration cap missing in %q", got)
	}
	// Just under the cap formats normally.
	ln.Duration = 99*24*time.Hour - time.Second
	got = plainLayout("h").FormatLine(ln)
	if !strings.Contains(got, "98d 23:59:59") {
		t.Errorf("duration = %q, want 98d 23:59:59", got)
	}
}

func TestRTTFormatting(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{1700 * time.Microsecond, "1.70"},
		{2500 * time.Microsecond, "2.50"},
		{12340 * time.Microsecond, "12.34"},
		{time.Millisecond, "1.00"},
	}
	for _, c := range cases {
		if got := FormatRTT(c.d); got != c.want {
			t.Errorf("FormatRTT(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestDurationFormatting(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0d 00:00:00"},
		{time.Second, "0d 00:00:01"},
		{65 * time.Second, "0d 00:01:05"},
		{35*time.Minute + 26*time.Second, "0d 00:35:26"},
		{2*24*time.Hour + 45*time.Minute + 26*time.Second, "2d 00:45:26"},
		{99 * 24 * time.Hour, "99d+"},
		{100 * 24 * time.Hour, "99d+"},
		{-5 * time.Second, "0d 00:00:00"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.d); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestRFC3339HeaderAndLine(t *testing.T) {
	layout := NewLayout("192.168.1.23", "rfc3339", nil)
	if got := layout.Header(); !strings.Contains(got, "TIME") {
		t.Errorf("rfc3339 header missing TIME: %q", got)
	}
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got := layout.FormatLine(ln)
	if want := "2026-08-16T11:00:35+01:00"; !strings.HasPrefix(got, want) {
		t.Errorf("rfc3339 line missing timestamp %q: %q", want, got)
	}
}

func TestFullWidthStable(t *testing.T) {
	// The padded width used for live overwrite must equal the sum of field
	// widths + separators, independent of content. Minimum line = 64
	// fixed + 15 HOST = 79 — under 80 (DECISIONS #68).
	layout := plainLayout("192.168.1.23")
	short := layout.FormatLine(Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)})
	_ = short
	if layout.FullWidth() != 79 {
		t.Errorf("FullWidth = %d, want 79", layout.FullWidth())
	}
}

func TestLayoutResizeContentFit(t *testing.T) {
	// HOST is content-fit with a terminal cap (DECISIONS #64,
	// user-approved 2026-08-17): as wide as the host's own length (clamped
	// to [15, 40]) but never wider than the terminal leaves after the
	// fixed columns; the floor is 15 and unknown width changes nothing.
	l := NewLayout("google.com", "HH:MM:SS", nil) // 10 cells → min 15
	if l.hostWidth != 15 {
		t.Fatalf("short host width = %d, want 15", l.hostWidth)
	}
	l.Resize(200) // wide terminal: short host stays compact
	if l.hostWidth != 15 {
		t.Errorf("Resize(200) = %d, want 15 (content-fit)", l.hostWidth)
	}
	l.Resize(0) // unknown width: no change
	if l.hostWidth != 15 {
		t.Errorf("Resize(0) = %d, want 15 (unchanged)", l.hostWidth)
	}

	l2 := NewLayout("a-very-long-hostname.internal", "HH:MM:SS", nil) // 29
	if l2.hostWidth != 29 {
		t.Fatalf("long host width = %d, want 29", l2.hostWidth)
	}
	l2.Resize(200) // room: content wins
	if l2.hostWidth != 29 {
		t.Errorf("Resize(200) = %d, want 29", l2.hostWidth)
	}
	l2.Resize(90) // only 26 left → capped
	if l2.hostWidth != 26 {
		t.Errorf("Resize(90) = %d, want 26 (terminal cap)", l2.hostWidth)
	}
	l2.Resize(79) // exactly the minimum (79 = 64 fixed + 15 HOST)
	if l2.hostWidth != 15 {
		t.Errorf("Resize(79) = %d, want 15 (floor)", l2.hostWidth)
	}
	if l2.displayHost != "a-very-long-ho…" {
		t.Errorf("displayHost after floor resize = %q, want 14 cells + ellipsis", l2.displayHost)
	}
	l2.Resize(78) // below the 79-cell floor: HOST stays content-fit (22),
	// the FAILS column drops instead of the line wrapping (DECISIONS #71)
	if l2.hostWidth != 22 {
		t.Errorf("Resize(78) = %d, want 22 (HOST absorbs, FAILS dropped)", l2.hostWidth)
	}
	if l2.retained != 3 {
		t.Errorf("Resize(78) retained = %d, want 3 (FAILS dropped)", l2.retained)
	}

	l3 := NewLayout(strings.Repeat("x", 41), "HH:MM:SS", nil) // > 40 → cap
	if l3.hostWidth != 40 {
		t.Fatalf("41-char host width = %d, want 40", l3.hostWidth)
	}
	l3.Resize(100) // 36 left → capped by the terminal
	if l3.hostWidth != 36 {
		t.Errorf("Resize(100) = %d, want 36", l3.hostWidth)
	}
	l3.Resize(70) // HOST absorbs (22), MAX+AVG+FAILS dropped (retained 2)
	if l3.hostWidth != 22 {
		t.Errorf("Resize(70) = %d, want 22", l3.hostWidth)
	}
	if l3.retained != 2 {
		t.Errorf("Resize(70) retained = %d, want 2 (FAILS/AVG dropped)", l3.retained)
	}
}

func TestLayoutTrimDropsRightmostColumns(t *testing.T) {
	// DECISIONS #71: below the HOST floor the rightmost columns drop (each
	// 8 cells) so the line fits instead of wrapping. Thresholds at
	// HH:MM:SS + HOST-15: 79 all four, 71-78 no FAILS, 63-70 no FAILS/AVG,
	// 55-62 only MIN, 47-54 essentials only, <47 wraps.
	l := NewLayout("frigate.app.home", "HH:MM:SS", nil) // 16 cells
	cases := []struct {
		width, wantRetained int
	}{
		{79, 4}, {78, 3}, {71, 3}, {70, 2}, {63, 2}, {62, 1}, {55, 1}, {54, 0}, {47, 0},
	}
	for _, c := range cases {
		l.Resize(c.width)
		if l.retained != c.wantRetained {
			t.Errorf("Resize(%d) retained = %d, want %d", c.width, l.retained, c.wantRetained)
		}
		ln := Line{Time: t0, Status: state.StatusUp, Duration: 4 * time.Second,
			Stats: buildStats(time.Millisecond, 3*time.Millisecond, 2*time.Millisecond, 2)}
		for _, s := range []string{l.Header(), l.FormatLine(ln)} {
			if w := cellWidth(s); w > c.width {
				t.Errorf("Resize(%d): rendered %d cells > width (%q)", c.width, w, s)
			}
		}
	}
	// Retained columns render right-to-left: at 62 the line ends with MIN,
	// at 54 with DURATION; header matches.
	l.Resize(62)
	if got := l.Header(); !strings.HasSuffix(got, "MIN") {
		t.Errorf("header at retained 1 must end with MIN: %q", got)
	}
	l.Resize(54)
	if got := l.Header(); !strings.HasSuffix(got, "DURATION") {
		t.Errorf("header at retained 0 must end with DURATION: %q", got)
	}
	// Below the 46-cell floor the line wraps (no more columns to drop):
	// the LIVE line is 46 cells (its animation frame keeps the DURATION
	// field untrimmed, so the header's 40 cells are not the widest line).
	l.Resize(46)
	if l.retained != 0 || cellWidth(l.Header()) != 40 {
		t.Errorf("Resize(46): retained=%d header=%d cells, want 0 / 40 (fits)", l.retained, cellWidth(l.Header()))
	}
	l.Resize(45)
	ln := Line{Time: t0, Status: state.StatusUp, Duration: 4 * time.Second,
		Stats: buildStats(time.Millisecond, 3*time.Millisecond, 2*time.Millisecond, 2)}
	if w := cellWidth(l.FormatLiveLine(ln, '▃')); w != 46 {
		t.Errorf("live line at retained 0 = %d cells, want 46", w)
	}
	if physicalRows(cellWidth(l.FormatLiveLine(ln, '▃')), 45) != 2 {
		t.Error("live line must wrap at 45 (the essentials floor)")
	}
}

func TestRuneBasedCellWidth(t *testing.T) {
	// cellWidth is the display-cell width: runes minus ANSI escape
	// sequences (SGR colors add bytes, not cells) — the exact measure the
	// window renderer's wrap math needs (DECISIONS #64).
	cases := []struct {
		s    string
		want int
	}{
		{"11:00:35  192.168.1.23    up", 28},
		{"…", 1},
		{"▁▃▅▇", 4},
		{"\x1b[1mab\x1b[0m", 2}, // colored: 2 cells
		{"münchen", 7},          // multibyte: 7 cells
	}
	for _, c := range cases {
		if got := cellWidth(c.s); got != c.want {
			t.Errorf("cellWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestPadIsRuneBased(t *testing.T) {
	// Padding is computed in cells (runes), so multibyte content never
	// shifts columns; pad never truncates.
	if got := pad("…", 3, false); got != "…  " {
		t.Errorf("pad(…,3,left) = %q", got)
	}
	if got := pad("x", 3, true); got != "  x" {
		t.Errorf("pad(x,3,right) = %q", got)
	}
	if got := pad("long", 2, false); got != "long" {
		t.Errorf("pad must not truncate: %q", got)
	}
}

func TestTruncateHostRuneBased(t *testing.T) {
	// Byte-slicing would cut mid-rune and misalign the column; rune-slicing
	// keeps the field exactly w cells (DECISIONS #64).
	if got := truncateHost("münchen.example.com", 6); got != "münch…" {
		t.Errorf("truncateHost(6) = %q, want münch…", got)
	}
	if got := truncateHost("abc", 5); got != "abc" {
		t.Errorf("truncateHost short = %q, want abc", got)
	}
	if got := truncateHost("abcdef", 6); got != "abcdef" {
		t.Errorf("exactly-w host must not truncate: %q", got)
	}
	if got := truncateHost("abcdef", 5); got != "abcd…" {
		t.Errorf("truncateHost(5) = %q, want abcd…", got)
	}
}

func TestLiveLineAnimationFrame(t *testing.T) {
	// The liveness animation occupies the last cell of the DURATION
	// field's padding (column 45 at the HOST-15 minimum): finalized lines
	// keep a plain space (byte-identical to spec §7.4), live lines show
	// the rising bar there, and the DURATION↔MIN separator (column 46)
	// stays a space so the bar floats between the values with whitespace
	// on both sides.
	layout := plainLayout("192.168.1.23")
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, 3*time.Millisecond, 2*time.Millisecond, 2)}

	// Finalized line: plain spaces at columns 45-46, MIN at column 47.
	fin := layout.FormatLine(ln)
	if fin[45] != ' ' || fin[46] != ' ' {
		t.Errorf("finalized separator cols 45-46 = %q/%q, want spaces", fin[45], fin[46])
	}
	if !strings.HasPrefix(fin[47:], "1.00") {
		t.Errorf("MIN not at col 47 in finalized line: %q", fin)
	}

	// Live line: every frame renders at column 45 (inside the DURATION
	// padding), the separator at 46 stays a space, MIN stays at 47, and
	// the frame cycles through the rising bar.
	want := []rune{'▁', '▃', '▅', '▇'}
	for i := 0; i < 8; i++ {
		fr := frameChar(i)
		if fr != want[i%len(want)] {
			t.Errorf("frameChar(%d) = %q, want %q", i, fr, want[i%len(want)])
		}
		live := layout.FormatLiveLine(ln, fr)
		runes := []rune(live)
		if runes[45] != fr {
			t.Errorf("live frame at col 45 = %q, want %q (line %q)", runes[45], fr, live)
		}
		if runes[46] != ' ' {
			t.Errorf("separator at col 46 = %q, want space (bar must not touch MIN): %q", runes[46], live)
		}
		// MIN value must still start at column 47 (animation must not
		// shift the columns).
		if !strings.HasPrefix(string(runes[47:]), "1.00") {
			t.Errorf("MIN shifted by animation: %q", live)
		}
	}
}
