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
)

// ICMPProbe sends ICMP echo requests over a privileged raw socket
// (CAP_NET_RAW on Linux; admin on Windows). ICMPv6 is used automatically
// for IPv6 targets.
type ICMPProbe struct {
	conn    *icmp.PacketConn
	ip      net.IP
	id      int
	seq     int
	timeout time.Duration
	isV6    bool
}

// NewICMPProbe resolves host once and opens the ICMP socket. A non-nil
// error is operational (permission denied, unsupported network, DNS
// failure) and must be reported as such — never as host-down.
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

	// Tier 1: privileged raw socket.
	conn, err := icmp.ListenPacket(rawNet, "")
	if err == nil {
		return newICMPProbe(conn, ip.IP, isV6, timeout), nil
	}
	// Tier 2: unprivileged ping socket.
	conn, err2 := icmp.ListenPacket(unprivNet, "")
	if err2 == nil {
		return newICMPProbe(conn, ip.IP, isV6, timeout), nil
	}
	// Tier 3: system ping command.
	if p, perr := newPingCmdProbe(ip.IP.String(), timeout); perr == nil {
		return p, nil
	}
	return nil, fmt.Errorf("unable to create ICMP socket: %w", err)
}

func newICMPProbe(conn *icmp.PacketConn, ip net.IP, isV6 bool, timeout time.Duration) *ICMPProbe {
	return &ICMPProbe{
		conn:    conn,
		ip:      ip,
		id:      os.Getpid() & 0xffff,
		timeout: timeout,
		isV6:    isV6,
	}
}

// Probe sends one echo request and waits for the matching reply.
func (p *ICMPProbe) Probe(ctx context.Context) Result {
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
	if _, err := p.conn.WriteTo(wire, &net.IPAddr{IP: p.ip}); err != nil {
		// A write failure after a successful socket open is operational
		// (e.g. network unreachable reported by the stack).
		return Result{Outcome: OutcomeError, Err: fmt.Errorf("write ICMP echo: %w", err)}
	}

	deadline := start.Add(p.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := p.conn.SetReadDeadline(deadline); err != nil {
		return Result{Outcome: OutcomeError, Err: fmt.Errorf("set ICMP read deadline: %w", err)}
	}

	// Loop until the deadline, ignoring packets that are not our echo reply
	// (raw sockets receive unrelated ICMP traffic).
	buf := make([]byte, 1500)
	for {
		n, peer, err := p.conn.ReadFrom(buf)
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

// Close releases the ICMP socket.
func (p *ICMPProbe) Close() error {
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

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
