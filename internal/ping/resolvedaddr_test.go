package ping

import (
	"net"
	"testing"
	"time"
)

// ResolvedAddr accessor tests: every probe implementation reports the
// canonical IP it resolved at construction — the single source of truth
// for the display's resolution caption (SPEC §6.1). A wrong accessor
// would show an address the probes never used.

func TestICMPProbeResolvedAddr(t *testing.T) {
	p := newICMPProbe(nil, net.ParseIP("127.0.0.1"), false, 0)
	if got := p.ResolvedAddr(); got != "127.0.0.1" {
		t.Errorf("ICMP ResolvedAddr() = %q, want 127.0.0.1", got)
	}
}

func TestFallbackProbeResolvedAddr(t *testing.T) {
	f := &fallbackProbe{tiers: []*fallbackTier{{name: "t", probe: &stubProbe{}}}, resolved: "142.250.190.46"}
	if got := f.ResolvedAddr(); got != "142.250.190.46" {
		t.Errorf("fallback ResolvedAddr() = %q, want 142.250.190.46", got)
	}
}

func TestTCPProbeResolvedAddrLiteral(t *testing.T) {
	pr, err := NewTCPProbe("127.0.0.1", 443, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := pr.ResolvedAddr(); got != "127.0.0.1" {
		t.Errorf("TCP ResolvedAddr() = %q, want 127.0.0.1", got)
	}
}

func TestTCPProbeResolvedAddrHostname(t *testing.T) {
	// A hostname must resolve to an IP literal, not echo the name back.
	pr, err := NewTCPProbe("localhost", 443, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got := pr.ResolvedAddr(); net.ParseIP(got) == nil {
		t.Errorf("TCP ResolvedAddr() for localhost = %q, want an IP literal", got)
	}
}

func TestPingCmdProbeResolvedAddr(t *testing.T) {
	p := &pingCmdProbe{ip: net.ParseIP("127.0.0.1")}
	if got := p.ResolvedAddr(); got != "127.0.0.1" {
		t.Errorf("pingcmd ResolvedAddr() = %q, want 127.0.0.1", got)
	}
}
