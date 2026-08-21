// Package logx writes the durable status-event log (spec §14): append-only,
// never colors or cursor control, RFC 3339 timestamps, IPv6 hosts
// bracketed, independent of quiet mode.
package logx

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"dohping/internal/state"
)

// Entry is one finalized status event to log.
type Entry struct {
	Time     time.Time
	Host     string // full host value (untruncated)
	Status   state.Status
	Duration time.Duration
	Fails    int
	Stats    state.Stats
}

// Logger appends entries to a log file.
type Logger struct {
	f      *os.File
	format string // text | json
	host   string // bracketed when IPv6
}

// Open opens (creating if needed, always appending) the log file.
// Format is "text" or "json". A non-nil error means the file could not be
// used — callers must fail cleanly, never silently drop logs.
func Open(path, format, host string) (*Logger, error) {
	// The path is the operator's own --log-file argument (a CLI, not a
	// server: no untrusted boundary to cross) — #nosec G304.
	// 0600, not 0644: the log carries host/timestamp/status data and must
	// not be world-readable by default (gosec G302; safe-by-default).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- user-supplied CLI log path
	if err != nil {
		return nil, err
	}
	return &Logger{f: f, format: format, host: bracketIPv6(host)}, nil
}

// Log writes one entry and flushes (append-only durability: a completed
// event is never lost to a later crash).
func (l *Logger) Log(e Entry) error {
	var line string
	if l.format == "json" {
		line = l.jsonLine(e)
	} else {
		line = l.textLine(e)
	}
	if _, err := l.f.WriteString(line); err != nil {
		return err
	}
	return l.f.Sync()
}

// Close flushes and closes the file.
func (l *Logger) Close() error {
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// textLine renders the text format (spec §14.4). Fields are omitted when
// not applicable; fails is always present for up/down (0 for up).
func (l *Logger) textLine(e Entry) string {
	ts := e.Time.Format(time.RFC3339)
	base := fmt.Sprintf("%s host=%s status=%s duration_seconds=%d",
		ts, l.host, e.Status.String(), int64(e.Duration.Seconds()))
	switch e.Status {
	case state.StatusUp:
		return fmt.Sprintf("%s min_ms=%.2f max_ms=%.2f avg_ms=%.2f fails=%d\n",
			base, ms(e.Stats.Min), ms(e.Stats.Max), ms(e.Stats.Avg()), e.Fails)
	case state.StatusDown:
		return fmt.Sprintf("%s fails=%d\n", base, e.Fails)
	default: // unknown / error: no RTT, no fails
		return base + "\n"
	}
}

// jsonLine renders the JSON format (spec §14.5): one object per line,
// RTT fields omitted for down/error, fails omitted for error.
func (l *Logger) jsonLine(e Entry) string {
	je := jsonEntry{
		Time:            e.Time.Format(time.RFC3339),
		Host:            l.host,
		Status:          e.Status.String(),
		DurationSeconds: int64(e.Duration.Seconds()),
	}
	switch e.Status {
	case state.StatusUp:
		m, x, a := ms2(e.Stats.Min), ms2(e.Stats.Max), ms2(e.Stats.Avg())
		je.MinMS, je.MaxMS, je.AvgMS = &m, &x, &a
		f := e.Fails
		je.Fails = &f
	case state.StatusDown:
		f := e.Fails
		je.Fails = &f
	}
	b, err := json.Marshal(je)
	if err != nil {
		// Cannot happen for this fixed struct; keep the log durable anyway.
		return fmt.Sprintf("{\"error\":%q}\n", err.Error())
	}
	return string(b) + "\n"
}

type jsonEntry struct {
	Time            string   `json:"time"`
	Host            string   `json:"host"`
	Status          string   `json:"status"`
	DurationSeconds int64    `json:"duration_seconds"`
	MinMS           *float64 `json:"min_ms,omitempty"`
	MaxMS           *float64 `json:"max_ms,omitempty"`
	AvgMS           *float64 `json:"avg_ms,omitempty"`
	Fails           *int     `json:"fails,omitempty"`
}

// bracketIPv6 wraps bare IPv6 literals in brackets so host values are
// unambiguous in logs (spec §14.6). Already-bracketed or non-IPv6 values
// pass through.
func bracketIPv6(host string) string {
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return "[" + host + "]"
	}
	return host
}

func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// ms2 rounds an RTT to two decimals for JSON output (spec §14.5 shows
// 2-decimal values like "min_ms":1.70).
func ms2(d time.Duration) float64 {
	return math.Round(ms(d)*100) / 100
}
