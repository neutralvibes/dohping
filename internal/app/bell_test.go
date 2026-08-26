package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bell (\a, 0x07) must never appear in piped/redirected stdout.
// The child harness runs non-TTY, which is exactly the suppressed case
// (SPEC 16.5: no \a in piped or redirected output).
func TestBellPipedNoBell(t *testing.T) {
	cmd := spawnChild(t, "--bell", "--probe", "tcp", "--count", "3", "127.0.0.1")
	out, err := cmd.CombinedOutput()
	if code := exitCode(err); code != ExitOK {
		t.Fatalf("exit = %d, want 0 (out=%q)", code, out)
	}
	if bytes.Contains(out, []byte{0x07}) {
		t.Errorf("bell byte appeared in piped stdout: %q", out)
	}
}

// Quiet mode suppresses the bell even with --bell and a real terminal
// (SPEC 12 + 16.5): quiet means no output, and the bell is output.
func TestBellQuietNoBell(t *testing.T) {
	// In-process with a TTY so the bell would be enabled if quiet were
	// not suppressing it. Quiet wins: no output at all.
	var out, errb bytes.Buffer
	code := Main([]string{"--bell", "-q", "--probe", "tcp", "--count", "3", "127.0.0.1"},
		&out, &errb, TTY{Stdout: true, Stdin: false})
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (out=%q err=%q)", code, out.String(), errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("quiet run produced output: %q", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte{0x07}) {
		t.Errorf("bell byte appeared in quiet output: %q", out.String())
	}
}

// The bell must never reach the log file: the log writer is a separate
// stream fed by a separate call, and the bell writes only to stdout.
func TestBellLogFileClean(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "dohping.log")
	cmd := spawnChild(t, "--bell", "--log-file", logPath, "--probe", "tcp", "--count", "3", "127.0.0.1")
	out, err := cmd.CombinedOutput()
	if code := exitCode(err); code != ExitOK {
		t.Fatalf("exit = %d, want 0 (out=%q)", code, out)
	}
	data, rerr := os.ReadFile(logPath)
	if rerr != nil {
		t.Fatalf("cannot read log: %v", rerr)
	}
	if bytes.Contains(data, []byte{0x07}) {
		t.Errorf("bell byte leaked into log file: %q", data)
	}
	if len(data) == 0 {
		t.Errorf("log file empty; expected the final-state entry")
	}
}

// TestBellTTYNoBellOnEstablishment: with --bell on a real terminal, the
// wiring must construct the bell and stay silent while the status only
// establishes (unknown -> up is not a change, SPEC 16.5). This exercises
// the full TTY construction path in-process without needing a live
// state flip (the ring path itself is covered by the bellx unit tests).
func TestBellTTYNoBellOnEstablishment(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--bell", "--probe", "tcp", "--count", "3", "127.0.0.1"},
		&out, &errb, TTY{Stdout: true, Stdin: false})
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 (out=%q err=%q)", code, out.String(), errb.String())
	}
	if bytes.Contains(out.Bytes(), []byte{0x07}) {
		t.Errorf("bell rang on startup establishment: %q", out.String())
	}
	if !strings.Contains(out.String(), "summary") {
		t.Errorf("interactive run missing summary: %q", out.String())
	}
}
