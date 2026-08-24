package app

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"dohping/internal/ping"
	"dohping/internal/state"
)

// TestMain implements the child-process re-exec pattern: when
// DOHPING_CHILD is set, the test binary runs Main directly.
func TestMain(m *testing.M) {
	if os.Getenv("DOHPING_CHILD") == "1" {
		// Args travel via env with a unit-separator delimiter (NUL is not
		// allowed in environment values).
		args := strings.Split(os.Getenv("DOHPING_CHILD_ARGS"), "\x1f")
		code := Main(args, os.Stdout, os.Stderr, TTY{Stdout: false, Stdin: false})
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// spawnChild starts a dohping child process with the given args.
func spawnChild(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		"DOHPING_CHILD=1",
		"DOHPING_CHILD_ARGS="+strings.Join(args, "\x1f"),
	)
	return cmd
}

// exitCode extracts the process exit code from an exec error.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

// TestCountExits0 verifies --count is the normal-completion exit-0 path.
func TestCountExits0(t *testing.T) {
	cmd := spawnChild(t, "--probe", "tcp", "--interval", "50ms", "--count", "3", "127.0.0.1")
	out, err := cmd.CombinedOutput()
	if code := exitCode(err); code != ExitOK {
		t.Fatalf("exit = %d, want 0 (out=%q)", code, out)
	}
	// Piped run: header + finalized lines, no summary.
	if strings.Contains(string(out), "summary") {
		t.Errorf("summary leaked into piped output: %q", out)
	}
	if !strings.Contains(string(out), "TIME") {
		t.Errorf("header missing: %q", out)
	}
}

// TestLogFileErrorExitsNonZero verifies a bad log path fails cleanly.
func TestLogFileErrorExitsNonZero(t *testing.T) {
	cmd := spawnChild(t, "--log-file", "/nonexistent-dir-xyz/dohping.log",
		"--probe", "tcp", "127.0.0.1")
	out, err := cmd.CombinedOutput()
	if code := exitCode(err); code == ExitOK {
		t.Fatalf("exit = 0, want non-zero (out=%q)", out)
	}
	if !strings.Contains(string(out), "log file") {
		t.Errorf("stderr missing log-file error: %q", out)
	}
}

// TestSummaryRendered verifies the summary renderer directly (interactive
// exit is covered manually via PTY).
func TestSummaryRendered(t *testing.T) {
	eng := state.New(1, 1)
	eng.Step(ping.Result{Outcome: ping.OutcomeUp, RTT: time.Millisecond}, time.Now())
	eng.Step(ping.Result{Outcome: ping.OutcomeUp, RTT: 2 * time.Millisecond}, time.Now())
	eng.Step(ping.Result{Outcome: ping.OutcomeDown}, time.Now())

	var buf bytes.Buffer
	printSummary(&buf, "192.168.1.23", eng, 2*time.Hour+14*time.Minute+9*time.Second)
	out := buf.String()
	for _, want := range []string{
		"--- dohping summary ---",
		"host:            192.168.1.23",
		"current status:  down",
		"run duration:    02:14:09",
		"total probes:    3",
		"successful:      2",
		"failed:          1",
		"loss:            33.33%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}
