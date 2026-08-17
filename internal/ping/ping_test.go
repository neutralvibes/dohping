package ping

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

// timeoutErr is a net.Error that reports Timeout() == true, simulating a
// silently dropped SYN.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

func TestClassifyDialError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantOut Outcome
		wantUp  bool // expect RTT set
	}{
		{"timeout", timeoutErr{}, OutcomeDown, false},
		{"op error wrapping timeout", &net.OpError{Op: "dial", Net: "tcp", Err: timeoutErr{}}, OutcomeDown, false},
		{"connection refused", syscall.ECONNREFUSED, OutcomeUp, true},
		{"connection reset", syscall.ECONNRESET, OutcomeUp, true},
		{"context canceled", context.Canceled, OutcomeDown, false},
		{"generic error", errors.New("network is unreachable"), OutcomeError, false},
		{"dns error", &net.DNSError{Err: "no such host", Name: "x.invalid"}, OutcomeError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := classifyDialError(tt.err, 5*time.Millisecond)
			if r.Outcome != tt.wantOut {
				t.Errorf("outcome = %v, want %v (err=%v)", r.Outcome, tt.wantOut, tt.err)
			}
			if tt.wantUp && r.RTT != 5*time.Millisecond {
				t.Errorf("RTT = %v, want 5ms", r.RTT)
			}
		})
	}
}

func TestTCPProbeEstablished(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	pr, err := NewTCPProbe("127.0.0.1", ln.Addr().(*net.TCPAddr).Port, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()

	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (err=%v)", r.Outcome, r.Err)
	}
	if r.RTT <= 0 {
		t.Errorf("RTT = %v, want > 0", r.RTT)
	}
}

func TestTCPProbeRefused(t *testing.T) {
	// Grab a port with no listener, then free it: connection refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	pr, err := NewTCPProbe("127.0.0.1", port, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()

	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (refused = alive); err=%v", r.Outcome, r.Err)
	}
}

func TestTCPProbeTimeout(t *testing.T) {
	// TEST-NET-1 (RFC 5737) is routed via the container gateway and
	// silently dropped → dial timeout → down.
	pr, err := NewTCPProbe("192.0.2.1", 443, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()

	r := pr.Probe(context.Background())
	if r.Outcome != OutcomeDown {
		t.Fatalf("outcome = %v, want down; err=%v", r.Outcome, r.Err)
	}
}

func TestTCPProbeDNSFailure(t *testing.T) {
	pr, err := NewTCPProbe("nonexistent-host.invalid", 443, time.Second)
	if err == nil {
		pr.Close()
		t.Fatal("NewTCPProbe succeeded, want DNS resolution error")
	}
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("error = %q, want resolve failure", err)
	}
}

func TestTCPProbeCancellation(t *testing.T) {
	pr, err := NewTCPProbe("192.0.2.1", 443, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	r := pr.Probe(ctx)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("probe took %v after ctx cancel, want prompt return", elapsed)
	}
	// Result may be Down (deadline) — but it must be prompt and not Error.
	if r.Outcome == OutcomeError && r.Err != nil && !errors.Is(r.Err, context.DeadlineExceeded) {
		t.Errorf("outcome = %v (%v), want prompt down/error without hang", r.Outcome, r.Err)
	}
}

// TestICMPProbeLoopback probes the loopback address through whatever ICMP
// tier the environment permits (raw socket, unprivileged ping socket, or
// the system ping command fallback). All three must report up.
func TestICMPProbeLoopback(t *testing.T) {
	pr, err := NewICMPProbe("127.0.0.1", time.Second)
	if err != nil {
		t.Fatalf("NewICMPProbe: %v", err)
	}
	defer pr.Close()

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
	defer pr.Close()
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
		pr.Close()
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

func TestICMPProbeDNSFailure(t *testing.T) {
	_, err := NewICMPProbe("nonexistent-host.invalid", time.Second)
	if err == nil {
		t.Fatal("NewICMPProbe succeeded, want DNS resolution error")
	}
}
