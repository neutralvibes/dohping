package ping

import (
	"strings"
	"testing"
	"time"
)

func TestParseRTT(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want time.Duration
	}{
		{"linux iputils", "64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=8.04 ms", 8040 * time.Microsecond},
		{"macos", "64 bytes from ::1: icmp_seq=0 hlim=64 time=0.030 ms", 30 * time.Microsecond},
		{"sub-millisecond", "64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time<0.001 ms", time.Microsecond},
		{"no match falls back", "ping: unknown host", 123 * time.Millisecond},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRTT(c.out, 123*time.Millisecond)
			// Float parsing can land a microsecond off (8.04 → 8.039999).
			if d := got - c.want; d < -time.Microsecond || d > time.Microsecond {
				t.Errorf("parseRTT(%q) = %v, want %v", c.out, got, c.want)
			}
		})
	}
}

func TestPingWaitSeconds(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want int
	}{
		{2 * time.Second, 2},
		{500 * time.Millisecond, 1},
		{time.Second + 100*time.Millisecond, 2},
		{0, 1},
	}
	for _, c := range cases {
		if got := pingWaitSeconds(c.d); got != c.want {
			t.Errorf("pingWaitSeconds(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

func TestPingCmdProbeErrorOnBadHost(t *testing.T) {
	// Resolution failure surfaces at construction, like the other probes.
	_, err := newPingCmdProbe("nonexistent-host.invalid", time.Second)
	if err == nil {
		t.Fatal("newPingCmdProbe succeeded, want resolution error")
	}
	if !strings.Contains(err.Error(), "resolve") {
		t.Errorf("error = %q, want resolve failure", err)
	}
}

func TestPermissionMessage(t *testing.T) {
	for _, msg := range []string{
		"ping: socket: Operation not permitted",
		"ping: socket: permission denied",
	} {
		if !permissionMessage(msg) {
			t.Errorf("permissionMessage(%q) = false, want true", msg)
		}
	}
	for _, msg := range []string{
		"connect: Network is unreachable",
		"ping: unknown host",
		"",
	} {
		if permissionMessage(msg) {
			t.Errorf("permissionMessage(%q) = true, want false", msg)
		}
	}
}
