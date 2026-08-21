// Package output renders the state engine's events as the plain line
// display (spec §7, §9, §10): fixed-width columns, live-updating current
// line, finalization, non-TTY hygiene. Window mode (Phase 5) reuses the
// same Layout and Line types.
package output

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dohping/internal/state"
	"dohping/internal/theme"
)

// Column width policy (spec §9.4 + DECISIONS): HOST is the elastic
// column — computed from the host at startup (min 15, max 40, truncated
// with …) and re-computed on terminal resize (DECISIONS #64: it expands
// to fill available width up to the max, and retracts to the min rather
// than letting the line wrap). DURATION capped at 99d+.
// Column starts (verified against the spec §7.4 header, DECISIONS #68):
//
//	0        10       26       32       47       55       63       71
//	TIME      HOST            STATE   DURATION       MIN     MAX     AVG     FAILS
const (
	minHostWidth = 15
	maxHostWidth = 40
	durCap       = 99 * 24 * time.Hour
)

// Layout computes column widths and renders header and lines. Widths are
// cell counts (runes), never bytes — multibyte glyphs (…, the animation
// bars, non-ASCII hosts) must not shift columns (DECISIONS #64).
type Layout struct {
	host        string // raw target (for re-truncation on resize)
	hostWidth   int
	displayHost string // host truncated to hostWidth with …
	timeFormat  string
	timeWidth   int
	retained    int // rightmost columns kept (4 = MIN MAX AVG FAILS … 0 = essentials only)
	theme       *theme.Renderer
}

// rightCols are the four rightmost columns in retention order. On a
// terminal too narrow for the full line the window layout drops them from
// the RIGHT (FAILS first) so the line keeps fitting instead of wrapping
// (DECISIONS #71 — the user's "trim the output line so last columns start
// disappearing"). Each is 8 cells including its leading separator, so a
// retained count of r leaves fixed + 8r cells (fixed = 47 at HH:MM:SS and
// the HOST-15 floor).
var rightCols = []struct {
	label string
	width int
}{
	{"MIN", 7}, {"MAX", 7}, {"AVG", 7}, {"FAILS", 8},
}

// NewLayout builds a layout for the given display host and timestamp
// format. th may be nil for a plain (uncolored) layout.
func NewLayout(host, timeFormat string, th *theme.Renderer) *Layout {
	tw := 8 // HH:MM:SS
	if timeFormat == "rfc3339" {
		tw = 25 // 2026-08-16T13:34:11+01:00
	}
	l := &Layout{
		host:       host,
		timeFormat: timeFormat,
		timeWidth:  tw,
		retained:   4, // full line until a narrow terminal drops columns (DECISIONS #71)
		theme:      th,
	}
	l.setHostWidth(hostWidthFor(host))
	return l
}

// fixedWidth is the width of everything except HOST (the fixed columns +
// separators) — the part of the line that never flexes. This must stay in
// sync with the join in formatLine: TIME + 2sp + STATE(5) + 1sp +
// DURATION(14) + 1sp + MIN(7) + 1sp + MAX(7) + 1sp + AVG(7) + 1sp +
// FAILS(8) = timeWidth + 56 (DECISIONS #68: STATE 5 wide, minimum line
// 79 cells — under 80).
func (l *Layout) fixedWidth() int { return l.timeWidth + 56 }

// Resize re-computes the layout for a terminal of the given width (cells).
// HOST is the elastic column (DECISIONS #64, user-approved 2026-08-17):
// content-fit with a terminal cap — as wide as the host's own length
// (clamped to spec §9.4's [15, 40]) but never wider than the terminal
// leaves after the fixed columns; a narrow terminal retracts the column
// rather than wrapping. Once HOST hits its 15-cell floor, the four
// rightmost columns drop from the RIGHT (DECISIONS #71) so the line still
// fits: at 79 cells all four, then FAILS, AVG, MAX, MIN (~8 cells each) —
// down to the essentials-only floor of 47 cells (HH:MM:SS). Only below
// that floor does the line wrap (documented limitation, DECISIONS #64).
// width ≤ 0 (unknown) leaves the layout unchanged.
func (l *Layout) Resize(width int) {
	if width <= 0 {
		return
	}
	// Fixed part (TIME + 2sp + STATE + 1sp + DURATION + 1sp) plus the
	// separator after HOST = timeWidth + 24.
	fixed := l.timeWidth + 24
	// Rightmost columns drop first at the HOST floor (each 8 cells):
	retained := 4
	for retained > 0 && fixed+minHostWidth+8*retained > width {
		retained--
	}
	w := hostWidthFor(l.host)
	if avail := width - fixed - 8*retained; avail < w {
		w = avail
	}
	if w < minHostWidth {
		w = minHostWidth
	}
	if w > maxHostWidth {
		w = maxHostWidth
	}
	l.retained = retained
	l.setHostWidth(w)
}

// setHostWidth applies a new HOST column width and re-truncates the
// display host. No-op when the width is unchanged (avoids re-slicing).
func (l *Layout) setHostWidth(w int) {
	if w == l.hostWidth {
		return
	}
	l.hostWidth = w
	l.displayHost = truncateHost(l.host, w)
}

// hostWidthFor is the startup policy (spec §9.4): the host's own length,
// clamped — the column is as wide as the target needs.
func hostWidthFor(host string) int {
	w := utf8.RuneCountInString(host)
	if w < minHostWidth {
		w = minHostWidth
	}
	if w > maxHostWidth {
		w = maxHostWidth
	}
	return w
}

// Line is one renderable status line.
type Line struct {
	Time     time.Time // when the status began
	Status   state.Status
	Duration time.Duration
	Stats    state.Stats
	Fails    int
}

// Header renders the column header, right-padded to the layout width and
// colored (bold) when the theme is active.
func (l *Layout) Header() string {
	s := fmt.Sprintf("%-*s  %-*s %-5s %-14s",
		l.timeWidth, "TIME", l.hostWidth, "HOST", "STATE", "DURATION")
	for i := 0; i < l.retained; i++ {
		s += " " + fmt.Sprintf("%-*s", rightCols[i].width, rightCols[i].label)
	}
	s = strings.TrimRight(s, " ")
	if l.theme != nil {
		s = l.theme.Paint(s, theme.RoleHeader)
	}
	return s
}

// liveFrames is the liveness animation: a rising bar drawn in the
// DURATION field's trailing padding cell (column 45 at the HOST-15
// minimum), one frame per probe event (user request 2026-08-17). The bar
// rises then resets — the reset jump is the visible "tick" that draws the
// eye.
var liveFrames = []rune{'▁', '▃', '▅', '▇'}

// frameChar returns the animation frame for counter n (cycles).
func frameChar(n int) rune { return liveFrames[n%len(liveFrames)] }

// FormatLine renders one status line with fixed-width columns. RTT fields
// are blank unless up; FAILS is blank unless down. Trailing whitespace is
// trimmed. Colors are applied per field when the theme is active. The
// DURATION↔MIN separator is a plain space: finalized/history lines and
// piped output carry no animation (spec §7.4 byte-identical).
func (l *Layout) FormatLine(ln Line) string { return l.formatLine(ln, 0) }

// FormatLiveLine renders the LIVE line: identical to FormatLine except the
// DURATION↔MIN separator cell (column 48) shows the liveness animation
// frame instead of a space. Only the current line uses this — finalized
// lines keep FormatLine so history stays static and non-TTY output stays
// parseable.
func (l *Layout) FormatLiveLine(ln Line, frame rune) string { return l.formatLine(ln, frame) }

func (l *Layout) formatLine(ln Line, frame rune) string {
	ts := formatTime(ln.Time, l.timeFormat)
	status := ln.Status.String()
	if ln.Status == state.StatusUnknown {
		// The never-established status renders compact ("?") in the
		// table — it can appear during the initial/hysteresis ramp
		// (state.go Step carries the unchanged status on non-transition
		// events) and must fit the 5-cell STATE field. The word survives
		// in prose contexts: the exit summary uses state.String
		// directly (DECISIONS #68).
		status = "?"
	}
	dur := FormatDuration(ln.Duration)

	min, max, avg := "", "", ""
	if ln.Status == state.StatusUp && ln.Stats.Count > 0 {
		min = FormatRTT(ln.Stats.Min)
		max = FormatRTT(ln.Stats.Max)
		avg = FormatRTT(ln.Stats.Avg())
	}
	fails := ""
	if ln.Status == state.StatusDown {
		fails = strconv.Itoa(ln.Fails)
	}

	durField := pad(dur, 14, false)
	if frame != 0 {
		// The liveness bar lives in the DURATION field's trailing padding
		// (its last cell, column 45 at the HOST-15 minimum): durations are
		// at most 11 chars ("98d 23:59:59"), so that cell is always
		// whitespace, and MIN keeps its column. The bar therefore floats
		// between DURATION and MIN with a space on each side (user
		// correction 2026-08-17: it was glued to MIN at column 48).
		r := []rune(durField)
		r[13] = frame
		durField = string(r)
	}

	fields := []string{
		pad(ts, l.timeWidth, true),
		pad(l.displayHost, l.hostWidth, false),
		pad(status, 5, false),
		durField,
	}
	right := []string{min, max, avg, fails}
	for i := 0; i < l.retained; i++ {
		fields = append(fields, pad(right[i], rightCols[i].width, false))
	}
	if l.theme != nil {
		fields[0] = l.theme.Paint(fields[0], theme.RoleTimestamp)
		fields[2] = l.theme.PaintStatus(fields[2], ln.Status)
		fields[3] = l.theme.Paint(fields[3], theme.RoleDuration)
		if l.retained == 4 && ln.Status == state.StatusDown && ln.Fails > 0 {
			fields[7] = l.theme.Paint(fields[7], theme.RoleFails)
		}
	}

	// DURATION and MIN are joined by a plain space; the liveness bar is
	// carried inside the DURATION field's padding (see above), never in
	// this separator.
	s := strings.Join([]string{fields[0], "  ", fields[1], " ", fields[2], " ", fields[3]}, "")
	for i := 4; i < len(fields); i++ {
		s += " " + fields[i]
	}
	return strings.TrimRight(s, " ")
}

// FullWidth returns the untrimmed cell width of a rendered line, used to
// pad live updates so overwrites clear stale characters.
func (l *Layout) FullWidth() int {
	return l.fixedWidth() + l.hostWidth
}

// FormatDuration renders a duration as "Nd HH:MM:SS", capped at "99d+"
// (spec §9.3, §9.4).
func FormatDuration(d time.Duration) string {
	if d >= durCap {
		return "99d+"
	}
	if d < 0 {
		d = 0
	}
	days := int(d / (24 * time.Hour))
	rem := d % (24 * time.Hour)
	h := int(rem / time.Hour)
	m := int(rem / time.Minute % 60)
	s := int(rem / time.Second % 60)
	return fmt.Sprintf("%dd %02d:%02d:%02d", days, h, m, s)
}

// FormatRTT renders an RTT in milliseconds with two decimals (spec §10.2).
func FormatRTT(d time.Duration) string {
	return fmt.Sprintf("%.2f", float64(d)/float64(time.Millisecond))
}

func formatTime(t time.Time, format string) string {
	if format == "rfc3339" {
		return t.Format(time.RFC3339)
	}
	return t.Format("15:04:05")
}

// truncateHost keeps the first w-1 cells of host plus an ellipsis so the
// column stays exactly w cells wide. Cell-counted in runes: multibyte
// hosts must not widen the field or corrupt the slice (byte-slicing could
// cut mid-rune — DECISIONS #64).
func truncateHost(host string, w int) string {
	r := []rune(host)
	if len(r) <= w {
		return host
	}
	return string(r[:w-1]) + "…"
}

// pad left- or right-pads s to w CELLS (runes). Never truncates: every
// caller's value is at most w cells by construction (RTT fields can drift
// with extreme values — same behavior as before, the renderer's wrap math
// counts the actual string width). A multibyte s keeps its full width —
// padding is computed from runes so columns never shift.
func pad(s string, w int, right bool) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	if right {
		return strings.Repeat(" ", w-n) + s
	}
	return s + strings.Repeat(" ", w-n)
}

// cellWidth returns the display-cell width of a rendered line: its runes,
// ignoring ANSI escape sequences (SGR colors add bytes but no cells). The
// window renderer uses this to count physical rows — a line wider than the
// terminal wraps, and the cursor math must count wrapped rows exactly.
func cellWidth(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
					j++
				}
				if j < len(s) {
					j++ // consume the final byte
				}
				i = j
				continue
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		n++
		i += size
	}
	return n
}
