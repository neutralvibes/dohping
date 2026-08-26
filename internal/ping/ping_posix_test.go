//go:build !windows

package ping

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Tests in this file exercise POSIX-specific behaviour:
//   - the real `ping` command fallback (the output parser is
//     Linux-iputils-oriented; Windows ping.exe output differs and is
//     not parsed)
//   - the ICMP tier model on POSIX (raw socket → ping socket → system ping)
//
// Windows runs only the platform-agnostic tests in ping_test.go /
// pingcmd_test.go (pure-logic: dial-error classification, RTT parsing,
// wait-seconds, permission-message matching).

// TestICMPProbeLoopback probes the loopback address through whatever ICMP
// tier the environment permits (raw socket, unprivileged ping socket, or
// the system ping command fallback). All three must report up.
func TestICMPProbeLoopback(t *testing.T) {
	pr, err := NewICMPProbe("127.0.0.1", time.Second)
	if err != nil {
		t.Fatalf("NewICMPProbe: %v", err)
	}
	defer func() { _ = pr.Close() }()

	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (err=%v)", r.Outcome, r.Err)
	}
	if r.RTT <= 0 {
		t.Errorf("RTT = %v, want > 0", r.RTT)
	}
}

// TestICMPFallbackEngagesPingCommand asserts that in environments without
// ICMP sockets, the ping-command tier is used (so the default probe still
// works instead of failing with a permission error).
func TestICMPFallbackEngagesPingCommand(t *testing.T) {
	pr, err := NewICMPProbe("127.0.0.1", time.Second)
	if err != nil {
		t.Skipf("no ICMP tier available: %v", err)
	}
	defer func() { _ = pr.Close() }()
	if _, ok := pr.(*pingCmdProbe); !ok {
		// Raw or unprivileged socket tier engaged — even better.
		t.Logf("socket tier engaged (%T); ping fallback not exercised", pr)
		return
	}
	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Errorf("ping fallback outcome = %v, want up (err=%v)", r.Outcome, r.Err)
	}
}

// TestICMPPermissionError asserts the operational-error contract when
// EVERY ICMP tier is unavailable: a clear error with guidance, never a
// host-down outcome. The ping command is hidden via PATH so the fallback
// cannot engage.
func TestICMPPermissionError(t *testing.T) {
	t.Setenv("PATH", "/nonexistent-dir-xyz")
	pr, err := NewICMPProbe("127.0.0.1", time.Second)
	if err == nil {
		_ = pr.Close()
		t.Fatal("NewICMPProbe succeeded without any ICMP tier")
	}
	if !strings.Contains(err.Error(), "unable to create ICMP socket") {
		t.Errorf("error = %q, want 'unable to create ICMP socket' framing", err)
	}
	if !IsPermissionError(err) {
		t.Errorf("error = %v, want a permission-class error (EPERM/EACCES)", err)
	}
	// The failure is operational: constructing the probe must never be
	// treated as evidence the host is down.
	if strings.Contains(err.Error(), "down") {
		t.Errorf("error mentions host-down: %q", err)
	}
}

// TestPingCmdProbeLoopbackUp runs the real `ping` against loopback via the
// command fallback and expects up.
func TestPingCmdProbeLoopbackUp(t *testing.T) {
	pr, err := newPingCmdProbe("127.0.0.1", 2*time.Second)
	if err != nil {
		t.Skipf("ping unavailable: %v", err)
	}
	defer func() { _ = pr.Close() }()
	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (err=%v)", r.Outcome, r.Err)
	}
	if r.RTT <= 0 {
		t.Errorf("RTT = %v, want > 0", r.RTT)
	}
}

// TestPingCmdProbeIPv6Up runs the real `ping` against ::1 via the command
// fallback and expects up.
func TestPingCmdProbeIPv6Up(t *testing.T) {
	pr, err := newPingCmdProbe("::1", 2*time.Second)
	if err != nil {
		t.Skipf("ping unavailable: %v", err)
	}
	defer func() { _ = pr.Close() }()
	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (err=%v)", r.Outcome, r.Err)
	}
}

// TestPingCmdProbeTimeoutDown uses TEST-NET-1 (RFC 5737), which is routed
// via the gateway and silently dropped → ping exit 1 → down.
func TestPingCmdProbeTimeoutDown(t *testing.T) {
	pr, err := newPingCmdProbe("192.0.2.1", 300*time.Millisecond)
	if err != nil {
		t.Skipf("ping unavailable: %v", err)
	}
	defer func() { _ = pr.Close() }()
	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeDown {
		t.Fatalf("outcome = %v, want down (err=%v)", r.Outcome, r.Err)
	}
}

// TestPingCmdProbePermissionClassed verifies that a ping exit-2 permission
// failure is classed as a permission error (EPERM), so the app can abort
// with guidance instead of probing in error state.
func TestPingCmdProbePermissionClassed(t *testing.T) {
	dir := t.TempDir()
	fakePing := `#!/bin/sh
echo "ping: socket: Operation not permitted" >&2
exit 2
`
	if err := os.WriteFile(dir+"/ping", []byte(fakePing), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	pr, err := newPingCmdProbe("127.0.0.1", time.Second)
	if err != nil {
		t.Skipf("ping tier unavailable: %v", err)
	}
	defer func() { _ = pr.Close() }()
	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeError {
		t.Fatalf("outcome = %v, want error", r.Outcome)
	}
	if !IsPermissionError(r.Err) {
		t.Errorf("err = %v, want permission-class error", r.Err)
	}
}
