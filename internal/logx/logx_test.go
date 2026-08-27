package logx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dohping/internal/state"
)

func t0() time.Time { return time.Date(2026, 8, 16, 11, 0, 35, 0, time.FixedZone("BST", 3600)) }

func upEntry() Entry {
	return Entry{
		Time:     t0(),
		Status:   state.StatusUp,
		Duration: 2126 * time.Second,
		Stats:    state.Stats{Count: 3, Min: 1700 * time.Microsecond, Max: 5900 * time.Microsecond, Sum: 8100 * time.Microsecond},
	}
}

func downEntry() Entry {
	return Entry{
		Time:     t0().Add(65 * time.Second),
		Status:   state.StatusDown,
		Duration: 65 * time.Second,
		Fails:    23,
	}
}

func TestOpenCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dohping.log")
	l, err := Open(path, "csv", "192.168.1.23", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

func TestOpenFailsCleanly(t *testing.T) {
	_, err := Open("/nonexistent-dir-xyz/file.log", "csv", "h", "")
	if err == nil {
		t.Fatal("Open succeeded for bad path, want error")
	}
}

func TestAppendNotTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dohping.log")
	l, _ := Open(path, "csv", "192.168.1.23", "")
	if err := l.Log(upEntry()); err != nil {
		t.Fatal(err)
	}
	_ = l.Close()

	l2, _ := Open(path, "csv", "192.168.1.23", "")
	defer func() { _ = l2.Close() }()
	if err := l2.Log(downEntry()); err != nil {
		t.Fatal(err)
	}
	_ = l2.Close()

	data, _ := os.ReadFile(path)
	if strings.Count(string(data), ",192.168.1.23,") != 2 {
		t.Errorf("append lost entries: %q", data)
	}
	if !strings.Contains(string(data), ",up,") || !strings.Contains(string(data), ",down,") {
		t.Errorf("entries missing: %q", data)
	}
}

func TestTextFormatUpLiteral(t *testing.T) {
	l := &Logger{format: "csv", address: "192.168.1.23"}
	got := l.textLine(upEntry())
	want := "2026-08-16T11:00:35+01:00,192.168.1.23,,up,2126,1.70,5.90,2.70,0\n"
	if got != want {
		t.Errorf("csv up (literal) mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestTextFormatUpWithName(t *testing.T) {
	l := &Logger{format: "csv", address: "142.250.190.46", name: "google.com"}
	got := l.textLine(upEntry())
	want := "2026-08-16T11:00:35+01:00,142.250.190.46,google.com,up,2126,1.70,5.90,2.70,0\n"
	if got != want {
		t.Errorf("csv up (name) mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestTextFormatDown(t *testing.T) {
	l := &Logger{format: "csv", address: "192.168.1.23"}
	got := l.textLine(downEntry())
	want := "2026-08-16T11:01:40+01:00,192.168.1.23,,down,65,,,,23\n"
	if got != want {
		t.Errorf("csv down mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestTextFormatError(t *testing.T) {
	l := &Logger{format: "csv", address: "192.168.1.23"}
	e := Entry{Time: t0(), Status: state.StatusError, Duration: 3 * time.Second}
	got := l.textLine(e)
	want := "2026-08-16T11:00:35+01:00,192.168.1.23,,error,3,,,,\n"
	if got != want {
		t.Errorf("csv error mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestCSVParseable verifies the CSV text format is genuinely machine-readable:
// every produced line parses back into exactly 9 fields, and the unavailable
// cells on a down line come back as empty strings.
func TestCSVParseable(t *testing.T) {
	l := &Logger{format: "csv", address: "192.168.1.23"}
	for _, e := range []Entry{upEntry(), downEntry(), {Time: t0(), Status: state.StatusError, Duration: 3 * time.Second}} {
		line := strings.TrimSuffix(l.textLine(e), "\n")
		fields := strings.Split(line, ",")
		if len(fields) != 9 {
			t.Errorf("csv line has %d fields, want 9: %q", len(fields), line)
		}
		if fields[1] != "192.168.1.23" || fields[0] == "" {
			t.Errorf("csv address/timestamp wrong: %q", line)
		}
		if fields[2] != "" {
			t.Errorf("csv name cell should be empty for a literal target: %q", line)
		}
	}
	// Down line: RTT cells empty, fails populated.
	down := strings.TrimSuffix(l.textLine(downEntry()), "\n")
	f := strings.Split(down, ",")
	if f[4] != "65" || f[5] != "" || f[6] != "" || f[7] != "" || f[8] != "23" {
		t.Errorf("down csv cells wrong: %q", down)
	}
	// Up line: RTT + fails populated.
	up := strings.TrimSuffix(l.textLine(upEntry()), "\n")
	f = strings.Split(up, ",")
	if f[5] != "1.70" || f[6] != "5.90" || f[7] != "2.70" || f[8] != "0" {
		t.Errorf("up csv cells wrong: %q", up)
	}
}

// TestSameShapeByNameAndLiteral pins the DECISIONS #98 invariant: the log
// must have the same shape whether the target was given as a DNS name or an
// IP literal — same columns, and only the name cell differs.
func TestSameShapeByNameAndLiteral(t *testing.T) {
	byName := (&Logger{format: "csv", address: "142.250.190.46", name: "google.com"}).textLine(upEntry())
	byLiteral := (&Logger{format: "csv", address: "142.250.190.46"}).textLine(upEntry())

	nameFields := strings.Split(strings.TrimSuffix(byName, "\n"), ",")
	literalFields := strings.Split(strings.TrimSuffix(byLiteral, "\n"), ",")
	if len(nameFields) != len(literalFields) {
		t.Fatalf("name and literal runs produced different field counts: %d vs %d", len(nameFields), len(literalFields))
	}
	if nameFields[2] != "google.com" || literalFields[2] != "" {
		t.Errorf("name cell wrong: name run %q, literal run %q", nameFields[2], literalFields[2])
	}
	for i := range nameFields {
		if i != 2 && nameFields[i] != literalFields[i] {
			t.Errorf("field %d differs between name and literal runs: %q vs %q", i, nameFields[i], literalFields[i])
		}
	}
}

func TestJSONFormatParseable(t *testing.T) {
	l := &Logger{format: "json", address: "192.168.1.23"}
	got := strings.TrimSpace(l.jsonLine(upEntry()))
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("json not parseable: %v\n%s", err, got)
	}
	if m["status"] != "up" || m["duration_seconds"] != float64(2126) {
		t.Errorf("json fields wrong: %v", m)
	}
	if m["min_ms"] != 1.7 || m["max_ms"] != 5.9 || m["avg_ms"] != 2.7 {
		t.Errorf("json rtt fields wrong: %v", m)
	}
	if m["name"] != "" {
		t.Errorf("json name should be empty for a literal target: %v", m)
	}
	// Two-decimal rounding.
	sub := Entry{Time: t0(), Status: state.StatusUp, Duration: time.Second,
		Stats: state.Stats{Count: 1, Min: 199399 * time.Nanosecond, Max: 199399 * time.Nanosecond, Sum: 199399 * time.Nanosecond}}
	got = strings.TrimSpace(l.jsonLine(sub))
	if !strings.Contains(got, `"min_ms":0.2`) {
		t.Errorf("json rtt not rounded to 2 decimals: %s", got)
	}
	// Down: RTT fields omitted.
	got = strings.TrimSpace(l.jsonLine(downEntry()))
	m = map[string]any{}
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("down json not parseable: %v\n%s", err, got)
	}
	if _, ok := m["min_ms"]; ok {
		t.Errorf("down entry carries min_ms: %v", m)
	}
	if m["fails"] != float64(23) {
		t.Errorf("down fails = %v, want 23", m["fails"])
	}
	if _, ok := m["name"]; !ok {
		t.Errorf("down entry missing name field: %v", m)
	}
	// Error: no RTT, no fails.
	e := Entry{Time: t0(), Status: state.StatusError, Duration: 3 * time.Second}
	got = strings.TrimSpace(l.jsonLine(e))
	m = map[string]any{}
	_ = json.Unmarshal([]byte(got), &m)
	if _, ok := m["fails"]; ok {
		t.Errorf("error entry carries fails: %v", m)
	}
	if m["status"] != "error" {
		t.Errorf("error status = %v", m["status"])
	}
}

// TestJSONFormatNameAndHost verifies the JSON shape for a name target:
// host carries the resolved IP, name carries the DNS name, both present.
func TestJSONFormatNameAndHost(t *testing.T) {
	l := &Logger{format: "json", address: "142.250.190.46", name: "google.com"}
	got := strings.TrimSpace(l.jsonLine(upEntry()))
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("json not parseable: %v\n%s", err, got)
	}
	if m["host"] != "142.250.190.46" {
		t.Errorf("json host = %v, want the resolved IP", m["host"])
	}
	if m["name"] != "google.com" {
		t.Errorf("json name = %v, want google.com", m["name"])
	}
}

func TestNoANSIInLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dohping.log")
	l, err := Open(path, "csv", "h", "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if err := l.Log(upEntry()); err != nil {
		t.Fatal(err)
	}
	if err := l.Log(downEntry()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range data {
		if b == 0x1b {
			t.Error("ANSI escape in log file")
		}
	}
	if strings.Contains(string(data), "\r") {
		t.Error("carriage return in log file")
	}
}

func TestIPv6Bracketing(t *testing.T) {
	l := &Logger{format: "csv", address: bracketIPv6("::1")}
	got := l.textLine(Entry{Time: t0(), Status: state.StatusUp, Duration: time.Second})
	if !strings.Contains(got, ",up,") || !strings.Contains(got, "::1") {
		t.Errorf("IPv6 not written as address: %q", got)
	}
	// Already bracketed passes through.
	if got := bracketIPv6("[::1]"); got != "[::1]" {
		t.Errorf("double bracketing: %q", got)
	}
	// Non-IPv6 untouched.
	if got := bracketIPv6("192.168.1.23"); got != "192.168.1.23" {
		t.Errorf("IPv4 modified: %q", got)
	}
	if got := bracketIPv6("example.com"); got != "example.com" {
		t.Errorf("hostname modified: %q", got)
	}
}

func TestCloseFlushes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dohping.log")
	l, _ := Open(path, "csv", "h", "")
	if err := l.Log(upEntry()); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), ",up,") {
		t.Errorf("entry not durable after Close: %q", data)
	}
}
