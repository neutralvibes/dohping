package app

import "testing"

// resolutionCaption builds the ping-style caption line (SPEC §6.1): only
// for DNS name targets that resolved to an IP. IP literals and unresolvable
// targets produce no caption.

func TestResolutionCaption(t *testing.T) {
	cases := []struct {
		host, resolved, want string
	}{
		{"google.com", "142.250.190.46", "google.com -> 142.250.190.46"},
		{"192.168.1.23", "192.168.1.23", ""}, // IP literal: address already in HOST
		{"::1", "::1", ""},                    // IPv6 literal: same
		{"google.com", "", ""},                // nothing resolved: no caption
		{"", "142.250.190.46", ""},            // empty host: no caption
	}
	for _, c := range cases {
		if got := resolutionCaption(c.host, c.resolved); got != c.want {
			t.Errorf("resolutionCaption(%q, %q) = %q, want %q", c.host, c.resolved, got, c.want)
		}
	}
}
