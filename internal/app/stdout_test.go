package app

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
)

// startLocalListener opens a TCP listener on 127.0.0.1 with an OS-assigned
// port, so a TCP probe run in-process has a deterministic up target.
func startLocalListener(t *testing.T) (host string, port int, close func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port = ln.Addr().(*net.TCPAddr).Port
	return "127.0.0.1", port, func() { _ = ln.Close() }
}

// TestStdoutJSONReplacesDisplay runs a short TCP-probe session with
// --stdout-json and asserts the table display is superseded entirely:
// stdout carries exactly two parseable JSON events — the establishment
// announcement (unknown → up, the live line appearing) and the final state
// at shutdown — and nothing else (no column header, no summary, no caption).
func TestStdoutJSONReplacesDisplay(t *testing.T) {
	host, port, close := startLocalListener(t)
	defer close()

	var out, errb bytes.Buffer
	code := Main([]string{
		"--probe", "tcp:" + strconv.Itoa(port), "--interval", "50ms", "--count", "2",
		"--stdout-json", host,
	}, &out, &errb, noTTY)

	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if strings.Contains(out.String(), "TIME") {
		t.Errorf("table header leaked into stdout stream: %q", out.String())
	}
	if strings.Contains(out.String(), "summary") {
		t.Errorf("exit summary leaked into stdout stream: %q", out.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	// Two events for a stable run: the establishment announcement (the live
	// line appearing — the stream must not wait for shutdown) and the final
	// record. Both must be up.
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %d, want exactly 2 (establishment + final): %q", len(lines), out.String())
	}
	for i, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("stdout line %d not JSON: %v\n%s", i, err, ln)
		}
		if m["host"] != "127.0.0.1" {
			t.Errorf("json[%d] host = %v, want 127.0.0.1", i, m["host"])
		}
		if m["name"] != "" {
			t.Errorf("json[%d] name = %v, want empty for an IP literal", i, m["name"])
		}
		if m["status"] != "up" {
			t.Errorf("json[%d] status = %v, want up", i, m["status"])
		}
	}
	// The FIRST line is the establishment announcement: its stats carry the
	// single triggering success (min == max == avg), proving it is the live
	// announcement, not a completed-period record.
	var first map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &first)
	if first["min_ms"] != first["max_ms"] {
		t.Errorf("establishment line should carry one sample (min==max), got %v/%v", first["min_ms"], first["max_ms"])
	}
}

// TestStdoutCSVShape asserts --stdout-csv emits the same machine-readable
// CSV shape as the log format, not the table, and that it announces
// establishment (two lines for a stable run).
func TestStdoutCSVShape(t *testing.T) {
	host, port, close := startLocalListener(t)
	defer close()

	var out, errb bytes.Buffer
	code := Main([]string{
		"--probe", "tcp:" + strconv.Itoa(port), "--interval", "50ms", "--count", "2",
		"--stdout-csv", host,
	}, &out, &errb, noTTY)

	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if strings.Contains(out.String(), "TIME") || strings.Contains(out.String(), "summary") {
		t.Errorf("table/summary leaked into stdout stream: %q", out.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %d, want exactly 2 (establishment + final): %q", len(lines), out.String())
	}
	// Both lines carry the 9-column shape; both are the up state.
	for _, ln := range lines {
		fields := strings.Split(ln, ",")
		if len(fields) != 9 {
			t.Fatalf("csv fields = %d, want 9: %q", len(fields), ln)
		}
		// timestamp,address,name,state,duration_seconds,min,max,avg,fails
		if fields[1] != "127.0.0.1" || fields[2] != "" || fields[3] != "up" {
			t.Errorf("csv cells wrong (address/name/state): %q", ln)
		}
	}
}

// TestStdoutJSONConflict verifies the two output-stream formats are
// mutually opposed: a clear usage error and exit 2, nothing on stdout.
func TestStdoutJSONConflict(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--stdout-json", "--stdout-csv", "h"}, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "stdout-json") || !strings.Contains(errb.String(), "stdout-csv") {
		t.Errorf("stderr = %q, want both flags named", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on usage error: %q", out.String())
	}
}

// TestStdoutJSONWithLogFile verifies --log-file is compatible with a
// stdout mode: the same events stream to stdout AND persist to the file
// (display vs persistence are independent).
func TestStdoutJSONWithLogFile(t *testing.T) {
	host, port, close := startLocalListener(t)
	defer close()

	logPath := t.TempDir() + "/dohping.log"
	var out, errb bytes.Buffer
	code := Main([]string{
		"--probe", "tcp:" + strconv.Itoa(port), "--interval", "50ms", "--count", "2",
		"--stdout-json", "--log-file", logPath, host,
	}, &out, &errb, noTTY)

	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if strings.Contains(out.String(), "TIME") || strings.Contains(out.String(), "summary") {
		t.Errorf("table/summary leaked into stdout stream: %q", out.String())
	}
	// The stream and the log file differ BY DESIGN: the stream is live
	// reporting (announces establishment → 2 lines for a stable run), the
	// log file is append-only history (completed periods → 1 final line).
	stdoutLines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(stdoutLines) != 2 {
		t.Fatalf("stdout lines = %d, want 2 (establishment + final): %q", len(stdoutLines), out.String())
	}
	fileData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	fileLines := strings.Split(strings.TrimSuffix(string(fileData), "\n"), "\n")
	if len(fileLines) != 1 {
		t.Fatalf("log file lines = %d, want 1 (completed period only, no establishment): %q", len(fileLines), fileData)
	}
	// The flags select the display, the log file is independent: the
	// stdout stream is JSON (from --stdout-json) while the file stays in
	// --log-format's default CSV. Both carry the up state.
	var m map[string]any
	if err := json.Unmarshal([]byte(stdoutLines[0]), &m); err != nil {
		t.Fatalf("stdout event not JSON: %v\n%s", err, stdoutLines[0])
	}
	if m["status"] != "up" || m["host"] != "127.0.0.1" {
		t.Errorf("stdout event wrong: %v", m)
	}
	fileFields := strings.Split(fileLines[0], ",")
	if len(fileFields) != 9 || fileFields[1] != "127.0.0.1" || fileFields[3] != "up" {
		t.Errorf("log file event wrong (want CSV address/state): %q", fileLines[0])
	}
}

// TestStdoutJSONQuietStillStreams asserts --quiet is redundant with a
// stdout mode but not an error, and the stream still emits (the decision
// #99 contract: quiet suppresses the table, which is already superseded).
func TestStdoutJSONQuietStillStreams(t *testing.T) {
	host, port, close := startLocalListener(t)
	defer close()

	var out, errb bytes.Buffer
	code := Main([]string{
		"--probe", "tcp:" + strconv.Itoa(port), "--interval", "50ms", "--count", "2",
		"--stdout-json", "--quiet", host,
	}, &out, &errb, noTTY)

	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (stderr=%q)", code, errb.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatalf("--quiet suppressed the stdout stream: %q", out.String())
	}
}
