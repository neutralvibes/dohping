//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSystemPingRescueWhenSocketsDenied is the ARM acceptance test: on a
// box where every socket tier is denied but `ping` works for the user,
// dohping must fall through to the system ping command and report up —
// never a bare error, never a demand for sudo (user report 2026-08-25).
func TestSystemPingRescueWhenSocketsDenied(t *testing.T) {
	dir := t.TempDir()
	fakePing := `#!/bin/sh
echo "64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.05 ms"
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "ping"), []byte(fakePing), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := spawnChild(t, "--probe", "icmp", "--interval", "100ms", "--count", "1", "127.0.0.1")
	cmd.Env = append(cmd.Env, "PATH="+dir)
	out, err := cmd.CombinedOutput()
	if code := exitCode(err); code != ExitOK {
		t.Fatalf("exit = %d, want 0 (out=%q)", code, out)
	}
	if !strings.Contains(string(out), "up") {
		t.Errorf("output missing up state (got %q)", out)
	}
}

// TestPermissionHintNamesTCPProbe verifies the no-privileges escape hatch
// is named in the guidance printed when every probe path is denied.
func TestPermissionHintNamesTCPProbe(t *testing.T) {
	h := permissionHint()
	for _, want := range []string{"setcap", "--probe tcp", "no privileges"} {
		if !strings.Contains(h, want) {
			t.Errorf("hint missing %q: %s", want, h)
		}
	}
}
