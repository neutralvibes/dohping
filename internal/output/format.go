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

	"dohping/internal/state"
	"dohping/internal/theme"
)

// Column width policy (spec §9.4 + DECISIONS): HOST computed once at
// startup (min 15, max 40, truncate with …), DURATION capped at 99d+.
// Column starts (verified against the spec §7.4 header):
//
//	0        10       26       34       49       57       65       73
//	TIME      HOST            STATUS  DURATION       MIN     MAX     AVG     FAILS
const (
	minHostWidth = 15
	maxHostWidth = 40
	durCap       = 99 * 24 * time.Hour
)

// Layout computes column widths once per run (single host, fixed formats)
// and renders header and lines.
type Layout struct {
	hostWidth   int
	displayHost string // host truncated to hostWidth with …
	timeFormat  string
	timeWidth   int
	theme       *theme.Renderer
}

// NewLayout builds a layout for the given display host and timestamp
// format. th may be nil for a plain (uncolored) layout.
func NewLayout(host, timeFormat string, th *theme.Renderer) *Layout {
	w := len(host)
	if w < minHostWidth {
		w = minHostWidth
	}
	if w > maxHostWidth {
		w = maxHostWidth
	}
	tw := 8 // HH:MM:SS
	if timeFormat == "rfc3339" {
		tw = 25 // 2026-08-16T13:34:11+01:00
	}
	return &Layout{
		hostWidth:   w,
		displayHost: truncateHost(host, w),
		timeFormat:  timeFormat,
		timeWidth:   tw,
		theme:       th,
	}
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
	s := fmt.Sprintf("%-*s  %-*s %-7s %-14s %-7s %-7s %-7s %-8s",
		l.timeWidth, "TIME", l.hostWidth, "HOST",
		"STATUS", "DURATION", "MIN", "MAX", "AVG", "FAILS")
	s = strings.TrimRight(s, " ")
	if l.theme != nil {
		s = l.theme.Paint(s, theme.RoleHeader)
	}
	return s
}

// FormatLine renders one status line with fixed-width columns. RTT fields
// are blank unless up; FAILS is blank unless down. Trailing whitespace is
// trimmed. Colors are applied per field when the theme is active.
func (l *Layout) FormatLine(ln Line) string {
	ts := formatTime(ln.Time, l.timeFormat)
	status := ln.Status.String()
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

	fields := []string{
		pad(ts, l.timeWidth, true),
		pad(l.displayHost, l.hostWidth, false),
		pad(status, 7, false),
		pad(dur, 14, false),
		pad(min, 7, false),
		pad(max, 7, false),
		pad(avg, 7, false),
		pad(fails, 8, false),
	}
	if l.theme != nil {
		fields[0] = l.theme.Paint(fields[0], theme.RoleTimestamp)
		fields[2] = l.theme.PaintStatus(fields[2], ln.Status)
		fields[3] = l.theme.Paint(fields[3], theme.RoleDuration)
		if ln.Status == state.StatusDown && ln.Fails > 0 {
			fields[7] = l.theme.Paint(fields[7], theme.RoleFails)
		}
	}

	s := strings.Join([]string{
		fields[0], "  ", fields[1], " ", fields[2], " ", fields[3], " ",
		fields[4], " ", fields[5], " ", fields[6], " ", fields[7],
	}, "")
	return strings.TrimRight(s, " ")
}

// FullWidth returns the untrimmed width of a rendered line, used to pad
// live updates so overwrites clear stale characters.
func (l *Layout) FullWidth() int {
	return l.timeWidth + 2 + l.hostWidth + 1 + 7 + 1 + 14 + 1 + 7 + 1 + 7 + 1 + 7 + 1 + 8
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

func truncateHost(host string, w int) string {
	if len(host) <= w {
		return host
	}
	// Keep w-1 runes plus the ellipsis so the column stays exactly w wide.
	return host[:w-1] + "…"
}

func pad(s string, w int, right bool) string {
	if right {
		return fmt.Sprintf("%*s", w, s)
	}
	return fmt.Sprintf("%-*s", w, s)
}
