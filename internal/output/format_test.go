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
	want := "TIME      HOST            STATUS  DURATION       MIN     MAX     AVG     FAILS"
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
	// Column-starts (0-based): TIME@0 HOST@10 STATUS@26 DURATION@34 MIN@49 MAX@57 AVG@65 FAILS@73.
	// Values are left-aligned in their fields, starting directly under the
	// header labels — this line is byte-identical to the spec §7.4 example.
	want := "11:00:35  192.168.1.23    up      0d 00:35:26    1.70    5.90    2.70"
	if got != want {
		t.Errorf("up line mismatch:\n got: %q\nwant: %q", got, want)
	}
	// Alignment invariants: values under their header labels.
	for _, col := range []struct {
		s string
		i int
	}{
		{"1.70", 49}, {"5.90", 57}, {"2.70", 65},
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
	// FAILS left-aligned under its header at column 73.
	want := "11:05:23  192.168.1.23    down    0d 00:01:05" + strings.Repeat(" ", 28) + "23"
	if got != want {
		t.Errorf("down line mismatch:\n got: %q\nwant: %q", got, want)
	}
	if !strings.HasPrefix(got[73:], "23") {
		t.Errorf("FAILS value not aligned at column 73: %q", got)
	}
}

func TestErrorLineGolden(t *testing.T) {
	ln := Line{
		Time:     time.Date(2026, 8, 16, 13, 0, 0, 0, time.FixedZone("BST", 3600)),
		Status:   state.StatusError,
		Duration: 3 * time.Second,
	}
	got := plainLayout("192.168.1.23").FormatLine(ln)
	want := "13:00:00  192.168.1.23    error   0d 00:00:03"
	if got != want {
		t.Errorf("error line mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestUnknownLineGolden(t *testing.T) {
	ln := Line{
		Time:     t0,
		Status:   state.StatusUnknown,
		Duration: 1 * time.Second,
	}
	got := plainLayout("192.168.1.23").FormatLine(ln)
	want := "11:00:35  192.168.1.23    unknown 0d 00:00:01"
	if got != want {
		t.Errorf("unknown line mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestHostWidthMin15(t *testing.T) {
	// A short host still gets the 15-char column (header parity).
	got := plainLayout("a").Header()
	want := "TIME      HOST            STATUS  DURATION       MIN     MAX     AVG     FAILS"
	if got != want {
		t.Errorf("min-width header mismatch:\n got: %q\nwant: %q", got, want)
	}
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got = plainLayout("a").FormatLine(ln)
	// "a" in a 15-wide field + 1 sep → 15 spaces before "up"; RTT values
	// left-aligned under their headers.
	want = "11:00:35  a" + strings.Repeat(" ", 15) + "up" +
		strings.Repeat(" ", 6) + "0d 00:00:01" +
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
	// HOST field 39 wide + 1 sep → 36 spaces between "HOST" and "STATUS".
	want := "TIME      HOST" + strings.Repeat(" ", 36) + "STATUS  DURATION       MIN     MAX     AVG     FAILS"
	if got != want {
		t.Errorf("wide header mismatch:\n got: %q\nwant: %q", got, want)
	}
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)}
	got = layout.FormatLine(ln)
	// STATUS column must sit at 26 + (39-15) = 50.
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
		strings.Repeat(" ", 6) + "0d 00:00:01" +
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
	// widths + separators, independent of content.
	layout := plainLayout("192.168.1.23")
	short := layout.FormatLine(Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1)})
	_ = short
	if layout.FullWidth() != 81 {
		t.Errorf("FullWidth = %d, want 81", layout.FullWidth())
	}
}

func TestLiveLineAnimationFrame(t *testing.T) {
	// The liveness animation occupies the last cell of the DURATION
	// field's padding (column 47): finalized lines keep a plain space
	// (byte-identical to spec §7.4), live lines show the rising bar there,
	// and the DURATION↔MIN separator (column 48) stays a space so the bar
	// floats between the values with whitespace on both sides.
	layout := plainLayout("192.168.1.23")
	ln := Line{Time: t0, Status: state.StatusUp, Duration: time.Second, Stats: buildStats(time.Millisecond, 3*time.Millisecond, 2*time.Millisecond, 2)}

	// Finalized line: plain spaces at columns 47-48, MIN at column 49.
	fin := layout.FormatLine(ln)
	if fin[47] != ' ' || fin[48] != ' ' {
		t.Errorf("finalized separator cols 47-48 = %q/%q, want spaces", fin[47], fin[48])
	}
	if !strings.HasPrefix(fin[49:], "1.00") {
		t.Errorf("MIN not at col 49 in finalized line: %q", fin)
	}

	// Live line: every frame renders at column 47 (inside the DURATION
	// padding), the separator at 48 stays a space, MIN stays at 49, and
	// the frame cycles through the rising bar.
	want := []rune{'▁', '▃', '▅', '▇'}
	for i := 0; i < 8; i++ {
		fr := frameChar(i)
		if fr != want[i%len(want)] {
			t.Errorf("frameChar(%d) = %q, want %q", i, fr, want[i%len(want)])
		}
		live := layout.FormatLiveLine(ln, fr)
		runes := []rune(live)
		if runes[47] != fr {
			t.Errorf("live frame at col 47 = %q, want %q (line %q)", runes[47], fr, live)
		}
		if runes[48] != ' ' {
			t.Errorf("separator at col 48 = %q, want space (bar must not touch MIN): %q", runes[48], live)
		}
		// MIN value must still start at column 49 (animation must not
		// shift the columns).
		if !strings.HasPrefix(string(runes[49:]), "1.00") {
			t.Errorf("MIN shifted by animation: %q", live)
		}
	}
}
