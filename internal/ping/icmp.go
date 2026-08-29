package ping

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"dohping/internal/debugx"
)

// ICMPProbe sends ICMP echo requests over a raw ICMP socket. ICMPv6 is used
// automatically for IPv6 targets.
//
// The probe is STATELESS with respect to its transport: it resolves the
// target once at construction (the stable-address contract) and opens a
// FRESH socket for every Probe() call, closing it when the probe
// completes. No socket is held across probes — a socket that goes stale
// after a network shift (opens but silently stops delivering replies) can
// therefore affect at most one probe; the next probe starts clean. The
// ICMP identity (ID and sequence) lives on the probe object so it stays
// continuous across the fresh sockets.
type ICMPProbe struct {
	network string // "ip4:icmp"/"ip6:ipv6-icmp" (raw) or "udp4"/"udp6" (ping socket)
	ip      net.IP
	id      int
	seq     int
	timeout time.Duration
	isV6    bool
}

// NewICMPProbe resolves host once and builds the ICMP fallback chain. A
// non-nil error is operational (permission denied, unsupported network,
// DNS failure) and must be reported as such — never as host-down.
//
// Three tiers are tried in order:
//
//  1. privileged raw socket ("ip4:icmp"/"ip6:ipv6-icmp", CAP_NET_RAW on
//     Linux),
//  2. unprivileged ping socket ("udp4"/"udp6", governed by
//     net.ipv4.ping_group_range),
//  3. the system ping command (works in restricted environments where
//     /bin/ping is elevated but the process holds no privileges).
//
// Tier availability is probed ONCE here (each probe socket is opened and
// immediately closed); every Probe() then opens its own fresh socket of
// the tier's network type. The system ping tier stays in the chain even
// when a socket opened, because a socket can open and still fail every
// probe — the fallback covers that by escalating to ping.
//
// When every tier is unavailable, the error carries the socket permission
// failure so callers can print helpful guidance.
func NewICMPProbe(host string, timeout time.Duration) (Probe, error) {
	ip, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	isV6 := ip.IP.To4() == nil
	rawNet := "ip4:icmp"
	unprivNet := "udp4"
	if isV6 {
		rawNet = "ip6:ipv6-icmp"
		unprivNet = "udp6"
	}

	tiers := make([]*fallbackTier, 0, 3)
	var socketErr error
	// Tier 1: privileged raw socket.
	if conn, e := icmp.ListenPacket(rawNet, ""); e == nil {
		_ = conn.Close() // availability check only; Probe() opens fresh
		tiers = append(tiers, &fallbackTier{name: "raw socket", probe: newICMPProbe(rawNet, ip.IP, isV6, timeout)})
	} else {
		socketErr = e
		debugx.Debugf("probe", "tier raw socket unavailable: %v", e)
	}
	// Tier 2: unprivileged ping socket.
	if conn, e := icmp.ListenPacket(unprivNet, ""); e == nil {
		_ = conn.Close() // availability check only; Probe() opens fresh
		tiers = append(tiers, &fallbackTier{name: "ping socket", probe: newICMPProbe(unprivNet, ip.IP, isV6, timeout)})
	} else {
		if socketErr == nil {
			socketErr = e
		}
		debugx.Debugf("probe", "tier ping socket unavailable: %v", e)
	}
	// Tier 3: system ping command.
	if p, perr := newPingCmdProbe(ip.IP.String(), timeout); perr == nil {
		tiers = append(tiers, &fallbackTier{name: "system ping", probe: p})
	} else {
		debugx.Debugf("probe", "tier system ping unavailable: %v", perr)
	}
	if len(tiers) == 0 {
		return nil, fmt.Errorf("unable to create ICMP socket: %w", socketErr)
	}
	debugx.Debugf("probe", "ICMP fallback chain: %d tier(s), starting on %q", len(tiers), tiers[0].name)
	return &fallbackProbe{tiers: tiers}, nil
}

func newICMPProbe(network string, ip net.IP, isV6 bool, timeout time.Duration) *ICMPProbe {
	return &ICMPProbe{
		network: network,
		ip:      ip,
		id:      os.Getpid() & 0xffff,
		timeout: timeout,
		isV6:    isV6,
	}
}

// Probe opens a fresh socket, sends one echo request, and waits for the
// matching reply. The socket is created and released within this call, so
// no connection state survives between probes.
func (p *ICMPProbe) Probe(ctx context.Context) Result {
	conn, err := icmp.ListenPacket(p.network, "")
	if err != nil {
		// A fresh socket that cannot be created is operational, not a
		// host-down condition (the fallback may escalate a never-worked
		// tier).
		return Result{Outcome: OutcomeError, Err: fmt.Errorf("open ICMP socket: %w", err)}
	}
	defer func() { _ = conn.Close() }()

	p.seq++
	var typ icmp.Type
	if p.isV6 {
		typ = ipv6.ICMPTypeEchoRequest
	} else {
		typ = ipv4.ICMPTypeEcho
	}
	msg := icmp.Message{
		Type: typ,
		Code: 0,
		Body: &icmp.Echo{ID: p.id, Seq: p.seq, Data: []byte("dohping")},
	}
	wire, err := msg.Marshal(nil)
	if err != nil {
		return Result{Outcome: OutcomeError, Err: fmt.Errorf("marshal ICMP echo: %w", err)}
	}

	start := time.Now()
	if _, err := conn.WriteTo(wire, &net.IPAddr{IP: p.ip}); err != nil {
		// A write failure after a successful socket open is operational
		// (e.g. network unreachable reported by the stack).
		return Result{Outcome: OutcomeError, Err: fmt.Errorf("write ICMP echo: %w", err)}
	}

	deadline := start.Add(p.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		return Result{Outcome: OutcomeError, Err: fmt.Errorf("set ICMP read deadline: %w", err)}
	}

	// Loop until the deadline, ignoring packets that are not our echo reply
	// (raw sockets receive unrelated ICMP traffic).
	buf := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return Result{Outcome: OutcomeDown} // no answer: unreachable or filtered
			}
			if ctx.Err() != nil {
				// Shutdown in progress; the loop drops this result.
				return Result{Outcome: OutcomeDown}
			}
			return Result{Outcome: OutcomeError, Err: fmt.Errorf("read ICMP reply: %w", err)}
		}
		if !peerIPEquals(peer, p.ip) {
			continue
		}
		rm, err := icmp.ParseMessage(protoNum(p.isV6), buf[:n])
		if err != nil {
			continue
		}
		if !isEchoReply(rm, p.isV6) {
			continue
		}
		echo, ok := rm.Body.(*icmp.Echo)
		if !ok || echo.ID != p.id || echo.Seq != p.seq {
			continue
		}
		return Result{Outcome: OutcomeUp, RTT: time.Since(start)}
	}
}

// Close is a no-op: the probe holds no persistent socket between probes.
// Kept to satisfy the Probe interface (the fallback chain calls it).
func (p *ICMPProbe) Close() error { return nil }

// ResolvedAddr returns the resolved target address in canonical form.
func (p *ICMPProbe) ResolvedAddr() string { return p.ip.String() }

func protoNum(v6 bool) int {
	if v6 {
		return 58 // ICMPv6
	}
	return 1 // ICMP
}

func isEchoReply(m *icmp.Message, v6 bool) bool {
	if v6 {
		return m.Type == ipv6.ICMPTypeEchoReply
	}
	return m.Type == ipv4.ICMPTypeEchoReply
}

func peerIPEquals(peer net.Addr, want net.IP) bool {
	ip, ok := peer.(*net.IPAddr)
	if !ok {
		return false
	}
	return ip.IP.Equal(want)
}
