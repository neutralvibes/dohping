package ping

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// rttRe matches the round-trip time in ping output:
//
//	64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=8.04 ms      (Linux iputils)
//	64 bytes from ::1: icmp_seq=0 hlim=64 time=0.030 ms        (macOS)
//	64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time<0.001 ms   (sub-millisecond)
var rttRe = regexp.MustCompile(`time[=<]\s*([0-9.]+)\s*ms`)

// pingCmdProbe probes via the system ping command. It is the last tier of
// the ICMP fallback chain, used when no ICMP socket is permitted (e.g.
// restricted containers where /bin/ping is elevated but the process has
// no CAP_NET_RAW and no ping-group coverage).
//
// The output parser is Linux-iputils-oriented (time=N.NN ms); platforms
// where the socket tiers work (macOS unprivileged ICMP, Linux with
// CAP_NET_RAW) never reach this tier.
type pingCmdProbe struct {
	ip      net.IP
	timeout time.Duration
}

// newPingCmdProbe validates the ping command exists and resolves the host
// once (consistent with the other probe types).
func newPingCmdProbe(host string, timeout time.Duration) (*pingCmdProbe, error) {
	if _, err := exec.LookPath("ping"); err != nil {
		return nil, fmt.Errorf("no ping command found: %w", err)
	}
	ip, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	return &pingCmdProbe{ip: ip.IP, timeout: timeout}, nil
}

// Probe runs `ping -c 1` against the resolved address.
func (p *pingCmdProbe) Probe(ctx context.Context) Result {
	args := []string{"-c", "1", "-W", strconv.Itoa(pingWaitSeconds(p.timeout))}
	if p.ip.To4() == nil {
		args = append(args, "-6")
	}
	args = append(args, p.ip.String())

	// #nosec G204 -- not a shell: exec.CommandContext runs "ping" directly
	// with argv, and the only variable element is p.ip.String(), a
	// canonical net.IP literal (digits/dots/colons — cannot begin with
	// "-" or carry metacharacters). No injection surface.
	cmd := exec.CommandContext(ctx, "ping", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	start := time.Now()
	err := cmd.Run()
	rtt := time.Since(start)

	if err == nil {
		return Result{Outcome: OutcomeUp, RTT: parseRTT(out.String(), rtt)}
	}
	if ee, ok := err.(*exec.ExitError); ok {
		switch ee.ExitCode() {
		case 1: // no reply within the wait window: unreachable or filtered
			return Result{Outcome: OutcomeDown}
		case 2: // usage / no route / no permission: operational, never host-down
			msg := strings.TrimSpace(errb.String())
			if permissionMessage(msg) {
				// e.g. "ping: socket: Operation not permitted" — a
				// privilege problem, classed so callers can print guidance
				// and abort (never keep probing in error state).
				return Result{Outcome: OutcomeError, Err: fmt.Errorf("ping failed: %s: %w", msg, syscall.EPERM)}
			}
			return Result{Outcome: OutcomeError, Err: fmt.Errorf("ping failed: %s", msg)}
		}
	}
	if ctx.Err() != nil {
		return Result{Outcome: OutcomeDown} // shutdown in progress; the loop drops it
	}
	return Result{Outcome: OutcomeError, Err: fmt.Errorf("ping failed: %v (%s)", err, strings.TrimSpace(errb.String()))}
}

// Close is a no-op (no persistent resource).
func (p *pingCmdProbe) Close() error { return nil }

// pingWaitSeconds maps the probe timeout to ping's -W (whole seconds,
// minimum 1).
func pingWaitSeconds(d time.Duration) int {
	s := int((d + time.Second - 1) / time.Second) // ceil
	if s < 1 {
		s = 1
	}
	return s
}

// permissionMessage reports whether ping stderr describes a privilege
// problem (the process cannot open a ping socket at all).
func permissionMessage(msg string) bool {
	return strings.Contains(msg, "Operation not permitted") ||
		strings.Contains(msg, "permission denied")
}

// parseRTT extracts the round-trip time from ping output, falling back to
// the measured command duration when the line is unparseable.
func parseRTT(out string, fallback time.Duration) time.Duration {
	m := rttRe.FindStringSubmatch(out)
	if m == nil {
		return fallback
	}
	ms, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return fallback
	}
	return time.Duration(ms * float64(time.Millisecond))
}
