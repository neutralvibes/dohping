package ping

import (
	"net"
	"testing"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Tests for the pure ICMP packet-parsing helpers that guard the tier-1 raw
// socket path (previously 0% covered — they are the correctness core of
// echo-reply matching and run without any privileges).

func TestProtoNum(t *testing.T) {
	if got := protoNum(false); got != 1 {
		t.Errorf("protoNum(v4) = %d, want 1 (ICMP)", got)
	}
	if got := protoNum(true); got != 58 {
		t.Errorf("protoNum(v6) = %d, want 58 (ICMPv6)", got)
	}
}

func TestIsEchoReply(t *testing.T) {
	cases := []struct {
		name string
		msg  *icmp.Message
		v6   bool
		want bool
	}{
		{"v4 echo reply", &icmp.Message{Type: ipv4.ICMPTypeEchoReply}, false, true},
		{"v4 echo request is not a reply", &icmp.Message{Type: ipv4.ICMPTypeEcho}, false, false},
		{"v4 destination unreachable is not a reply", &icmp.Message{Type: ipv4.ICMPTypeDestinationUnreachable}, false, false},
		{"v6 echo reply", &icmp.Message{Type: ipv6.ICMPTypeEchoReply}, true, true},
		{"v6 echo request is not a reply", &icmp.Message{Type: ipv6.ICMPTypeEchoRequest}, true, false},
		{"v4 type under v6 is not a reply", &icmp.Message{Type: ipv4.ICMPTypeEchoReply}, true, false},
	}
	for _, c := range cases {
		if got := isEchoReply(c.msg, c.v6); got != c.want {
			t.Errorf("%s: isEchoReply = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPeerIPEquals(t *testing.T) {
	lo := net.IPv4(127, 0, 0, 1)
	other := net.IPv4(127, 0, 0, 2)
	cases := []struct {
		name      string
		peer      net.Addr
		want      net.IP
		wantEqual bool
	}{
		{"matching IPAddr", &net.IPAddr{IP: lo}, lo, true},
		{"different IPAddr", &net.IPAddr{IP: other}, lo, false},
		{"non-IPAddr is never equal", &net.TCPAddr{IP: lo, Port: 443}, lo, false},
		{"nil peer is never equal", nil, lo, false},
	}
	for _, c := range cases {
		if got := peerIPEquals(c.peer, c.want); got != c.wantEqual {
			t.Errorf("%s: peerIPEquals = %v, want %v", c.name, got, c.wantEqual)
		}
	}
}

func TestNewICMPProbeDefaults(t *testing.T) {
	// Pure constructor: pins the tier-1 default wiring without any socket.
	ip := net.IPv4(127, 0, 0, 1)
	p := newICMPProbe(nil, ip, false, 0)
	if p == nil {
		t.Fatal("newICMPProbe returned nil")
	}
	if !p.ip.Equal(ip) {
		t.Errorf("ip = %v, want %v", p.ip, ip)
	}
	if p.isV6 {
		t.Error("isV6 = true for an IPv4 address, want false")
	}
	// id must be a non-zero 16-bit value derived from the process.
	if p.id == 0 || p.id > 0xffff {
		t.Errorf("id = %d, want in (0, 65535]", p.id)
	}
	// IPv6 detection.
	p6 := newICMPProbe(nil, net.ParseIP("::1"), true, 0)
	if !p6.isV6 {
		t.Error("isV6 = false for IPv6, want true")
	}
}

func TestICMPProbeCloseNilConn(t *testing.T) {
	// Close on a probe whose conn was already closed (or never opened) is a
	// safe no-op — guards the "conn set to nil after Close" path.
	p := &ICMPProbe{conn: nil}
	if err := p.Close(); err != nil {
		t.Errorf("Close on nil conn = %v, want nil", err)
	}
}
