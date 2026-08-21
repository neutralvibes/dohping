//go:build windows

package ping

import (
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestClassifyDialErrorWindowsRefused verifies the Windows-specific refused
// path: a dial refused by winsock surfaces as WSAECONNREFUSED (10061),
// wrapped in *net.OpError and *os.SyscallError. It must classify as up
// (a refusal proves the host is alive), not as an operational error.
func TestClassifyDialErrorWindowsRefused(t *testing.T) {
	base := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("connectex: %w", windows.WSAECONNREFUSED),
	}
	r := classifyDialError(base, 5*time.Millisecond)
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (refused = alive); err=%v", r.Outcome, r.Err)
	}
	if r.RTT != 5*time.Millisecond {
		t.Errorf("RTT = %v, want 5ms", r.RTT)
	}
}

// TestClassifyDialErrorWindowsReset mirrors the POSIX ECONNRESET case for
// parity: a reset connection also proves the host answered.
func TestClassifyDialErrorWindowsReset(t *testing.T) {
	base := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: fmt.Errorf("connectex: %w", syscall.ECONNRESET),
	}
	r := classifyDialError(base, time.Millisecond)
	if r.Outcome != OutcomeUp {
		t.Fatalf("outcome = %v, want up (reset = alive); err=%v", r.Outcome, r.Err)
	}
}
