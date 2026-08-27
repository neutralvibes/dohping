// Package logx writes the durable status-event log: append-only,
// never colors or cursor control, RFC 3339 timestamps, IPv6 addresses
// bracketed, independent of quiet mode.
package logx

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"dohping/internal/state"
)

// Entry is one finalized status event to log.
type Entry struct {
	Time     time.Time
	Status   state.Status
	Duration time.Duration
	Fails    int
	Stats    state.Stats
}

// Logger renders one status event per line (CSV or JSON) and writes it to
// a sink. The sink is a file (durable, fsynced per event) for --log-file,
// or stdout for the structured stdout modes — same renderer, same schema,
// same per-event flush, only the destination differs.
type Logger struct {
	w       io.Writer    // sink
	close   func() error // closes the sink; nil = not owned (stdout)
	sync    func() error // durability flush; nil = not applicable (stdout)
	format  string       // csv | json
	address string       // resolved canonical IP, bracketed when IPv6
	name    string       // DNS name when the target was given as one, else ""
}

// Open opens (creating if needed, always appending) the log file.
// Format is "csv" or "json". address is the resolved canonical IP the
// probes use (bracketed internally when IPv6); name is the DNS name the
// target was given as, or "" for an IP literal. A non-nil error means
// the file could not be used — callers must fail cleanly, never
// silently drop logs.
func Open(path, format, address, name string) (*Logger, error) {
	// The path is the operator's own --log-file argument (a CLI, not a
	// server: no untrusted boundary to cross) — #nosec G304.
	// 0600, not 0644: the log carries host/timestamp/status data and must
	// not be world-readable by default (gosec G302; safe-by-default).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- user-supplied CLI log path
	if err != nil {
		return nil, err
	}
	return &Logger{w: f, close: f.Close, sync: f.Sync, format: format, address: bracketIPv6(address), name: name}, nil
}

// NewStdout returns a logger that writes structured events to w (stdout)
// instead of a file: same renderer and schema as the log file, streaming
// one line per event. The sink is not owned — it is never closed and
// never fsynced (stdout may be a pipe or terminal).
func NewStdout(w io.Writer, format, address, name string) *Logger {
	return &Logger{w: w, format: format, address: bracketIPv6(address), name: name}
}

// Log writes one entry and flushes. File sinks fsync for append-only
// durability (a completed event is never lost to a later crash); stdout
// needs no fsync — the write reaches the pipe/terminal directly.
func (l *Logger) Log(e Entry) error {
	var line string
	if l.format == "json" {
		line = l.jsonLine(e)
	} else {
		line = l.textLine(e)
	}
	if _, err := io.WriteString(l.w, line); err != nil {
		return err
	}
	if l.sync != nil {
		return l.sync()
	}
	return nil
}

// Close closes the sink if the logger owns it (a file). A stdout logger
// has nothing to close. Idempotent.
func (l *Logger) Close() error {
	if l.close == nil {
		return nil
	}
	err := l.close()
	l.close = nil
	l.sync = nil
	return err
}

// textLine renders the CSV format: one event per line,
// comma-separated columns, address before name before state, duration in
// raw seconds, and unavailable fields left as empty cells (machine-
// readable — a CSV parser reads them as "not applicable", same semantics
// as JSON's omitempty). The name column is always present (empty when
// the target was an IP literal), so every run produces the same columns
// whether the target was given as a name or an address. RTT fields are
// present only when up; fails only when down.
func (l *Logger) textLine(e Entry) string {
	ts := e.Time.Format(time.RFC3339)
	f := func(ms float64) string { return strconv.FormatFloat(ms, 'f', 2, 64) }
	base := []string{ts, l.address, l.name, e.Status.String(), strconv.FormatInt(int64(e.Duration.Seconds()), 10)}
	switch e.Status {
	case state.StatusUp:
		base = append(base, f(ms(e.Stats.Min)), f(ms(e.Stats.Max)), f(ms(e.Stats.Avg())), strconv.Itoa(e.Fails))
	case state.StatusDown:
		base = append(base, "", "", "", strconv.Itoa(e.Fails))
	default: // unknown / error: no RTT, no fails
		base = append(base, "", "", "", "")
	}
	return strings.Join(base, ",") + "\n"
}

// jsonLine renders the JSON format: one object per line,
// RTT fields omitted for down/error, fails omitted for error. The host
// field carries the resolved IP (as the CSV address column); name is
// always present, empty when the target was an IP literal.
func (l *Logger) jsonLine(e Entry) string {
	je := jsonEntry{
		Time:            e.Time.Format(time.RFC3339),
		Host:            l.address,
		Name:            l.name,
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
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	DurationSeconds int64    `json:"duration_seconds"`
	MinMS           *float64 `json:"min_ms,omitempty"`
	MaxMS           *float64 `json:"max_ms,omitempty"`
	AvgMS           *float64 `json:"avg_ms,omitempty"`
	Fails           *int     `json:"fails,omitempty"`
}

// bracketIPv6 wraps bare IPv6 literals in brackets so address values are
// unambiguous in logs. Already-bracketed or non-IPv6 values
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

// ms2 rounds an RTT to two decimals for JSON output (the JSON log shows
// 2-decimal values like "min_ms":1.70).
func ms2(d time.Duration) float64 {
	return math.Round(ms(d)*100) / 100
}
