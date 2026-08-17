package app

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

// TestSIGINTExits130 verifies graceful shutdown on Ctrl-C: exit 130, the
// current display line finalized, log flushed.
func TestSIGINTExits130(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "dohping.log")
	cmd := spawnChild(t, "--probe", "tcp", "--interval", "200ms",
		"--log-file", logPath, "127.0.0.1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // let it probe a few times
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	code := exitCode(err)
	if code != ExitInterrupt {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, ExitInterrupt, stderr.String())
	}
	// Display line finalized (piped run: plain lines with header).
	if !strings.Contains(stdout.String(), "up") {
		t.Errorf("stdout missing finalized line: %q", stdout.String())
	}
	// Log flushed with at least the final state.
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log missing: %v", err)
	}
	if !strings.Contains(string(data), "status=up") {
		t.Errorf("log not flushed: %q", data)
	}
	if strings.Contains(string(data), "\x1b") {
		t.Errorf("ANSI in log: %q", data)
	}
}

// TestSIGTERMExits143 verifies SIGTERM → 143 and flush.
func TestSIGTERMExits143(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "dohping.log")
	cmd := spawnChild(t, "--probe", "tcp", "--interval", "200ms",
		"--log-file", logPath, "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	if code := exitCode(err); code != ExitTerminated {
		t.Fatalf("exit = %d, want %d", code, ExitTerminated)
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "status=up") {
		t.Errorf("log not flushed on SIGTERM: %q", data)
	}
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

// TestQuietStillLogs verifies --quiet suppresses display but not logs.
func TestQuietStillLogs(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "dohping.log")
	cmd := spawnChild(t, "--quiet", "--probe", "tcp", "--interval", "200ms",
		"--log-file", logPath, "127.0.0.1")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		// The child exits 130 on SIGINT; that is the expected non-zero code.
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != ExitInterrupt {
			t.Fatalf("child exit = %v, want %d", err, ExitInterrupt)
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("quiet run wrote stdout: %q", stdout.String())
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "status=up") {
		t.Errorf("quiet run did not log: %q", data)
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

// TestSummaryPrintedOnInteractiveExit is covered manually via PTY; this
// verifies the summary renderer directly.
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

// TestPermissionErrorAbortsRun: when the ping tier reports a permission
// failure at probe time, the app must abort with exit 3 and guidance —
// not keep probing in error state (user report: repeated error lines).
func TestPermissionErrorAbortsRun(t *testing.T) {
	dir := t.TempDir()
	fakePing := `#!/bin/sh
echo "ping: socket: Operation not permitted" >&2
exit 2
`
	if err := os.WriteFile(dir+"/ping", []byte(fakePing), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := spawnChild(t, "--probe", "icmp", "--interval", "100ms", "127.0.0.1")
	cmd.Env = append(cmd.Env, "PATH="+dir)
	out, err := cmd.CombinedOutput()
	if code := exitCode(err); code != ExitProbeInit {
		t.Fatalf("exit = %d, want %d (out=%q)", code, ExitProbeInit, out)
	}
	if !strings.Contains(string(out), "hint:") {
		t.Errorf("missing guidance hint: %q", out)
	}
	// It must not have probed forever in error state — abort is prompt.
	if strings.Count(string(out), "error") > 4 {
		t.Errorf("too many error lines before abort: %q", out)
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
