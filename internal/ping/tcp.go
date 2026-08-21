package ping

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"
)

// TCPProbe connects to a resolved TCP address. No privileges required.
//
// Semantics (spec §4.1): established or refused → up (a refusal proves the
// host answered); timeout → down (SYN silently dropped); DNS/routing or
// other operational errors → error.
type TCPProbe struct {
	addr    *net.TCPAddr
	timeout time.Duration
}

// NewTCPProbe resolves host once at startup; probes then dial the resolved
// address.
func NewTCPProbe(host string, port int, timeout time.Duration) (*TCPProbe, error) {
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	return &TCPProbe{addr: addr, timeout: timeout}, nil
}

// Probe dials the resolved address. The per-probe timeout is the dial
// timeout.
func (p *TCPProbe) Probe(ctx context.Context) Result {
	d := net.Dialer{Timeout: p.timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", p.addr.String())
	if err == nil {
		_ = conn.Close() // probe socket: close error is irrelevant to the result
		return Result{Outcome: OutcomeUp, RTT: time.Since(start)}
	}
	return classifyDialError(err, time.Since(start))
}

// Close is a no-op for TCP probes (no persistent socket).
func (p *TCPProbe) Close() error { return nil }

// classifyDialError maps a dial error to the probe contract. Exposed for
// deterministic unit testing.
func classifyDialError(err error, rtt time.Duration) Result {
	// A timed-out dial means the SYN was silently dropped (or the route is
	// so broken the stack gave up): treat as down.
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return Result{Outcome: OutcomeDown}
	}
	// The host answered "no" — that proves it is alive. On POSIX this is
	// ECONNREFUSED/ECONNRESET; Windows surfaces it as WSAECONNREFUSED.
	if isRefused(err) {
		return Result{Outcome: OutcomeUp, RTT: rtt}
	}
	// Cancellation during shutdown: the loop drops this result anyway.
	if errors.Is(err, context.Canceled) {
		return Result{Outcome: OutcomeDown}
	}
	// DNS, routing, permission, and every other failure is operational —
	// never a host-down condition.
	return Result{Outcome: OutcomeError, Err: err}
}
