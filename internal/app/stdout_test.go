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
// stdout carries exactly one parseable JSON event (the final state at
// shutdown), and nothing else — no column header, no summary, no caption.
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
	if len(lines) != 1 {
		t.Fatalf("stdout lines = %d, want exactly 1 (the final event): %q", len(lines), out.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("stdout line not JSON: %v\n%s", err, lines[0])
	}
	if m["host"] != "127.0.0.1" {
		t.Errorf("json host = %v, want 127.0.0.1", m["host"])
	}
	if m["name"] != "" {
		t.Errorf("json name = %v, want empty for an IP literal", m["name"])
	}
	if m["status"] != "up" {
		t.Errorf("json status = %v, want up", m["status"])
	}
}

// TestStdoutCSVShape asserts --stdout-csv emits the same machine-readable
// CSV shape as the log format, not the table.
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
	if len(lines) != 1 {
		t.Fatalf("stdout lines = %d, want exactly 1: %q", len(lines), out.String())
	}
	fields := strings.Split(lines[0], ",")
	if len(fields) != 9 {
		t.Fatalf("csv fields = %d, want 9: %q", len(fields), lines[0])
	}
	// timestamp,address,name,state,duration_seconds,min,max,avg,fails
	if fields[1] != "127.0.0.1" || fields[2] != "" || fields[3] != "up" {
		t.Errorf("csv cells wrong (address/name/state): %q", lines[0])
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
	// The stdout stream and the log file carry the same single event.
	stdoutLines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(stdoutLines) != 1 {
		t.Fatalf("stdout lines = %d, want 1: %q", len(stdoutLines), out.String())
	}
	fileData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	fileLines := strings.Split(strings.TrimSuffix(string(fileData), "\n"), "\n")
	if len(fileLines) != 1 {
		t.Fatalf("log file lines = %d, want 1: %q", len(fileLines), fileData)
	}
	// The flags select the display, the log file is independent: the
	// stdout stream is JSON (from --stdout-json) while the file stays in
	// --log-format's default CSV. Same single event on both.
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
