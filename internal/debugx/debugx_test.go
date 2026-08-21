//go:build debug

package debugx

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// reset restores the package's pristine disabled state between tests.
func reset() {
	mu.Lock()
	defer mu.Unlock()
	enabled = false
	w = nil
	if file != nil {
		_ = file.Close()
		file = nil
	}
}

func TestDisabledByDefault(t *testing.T) {
	defer reset()
	if Enabled() {
		t.Fatal("debug logging must be off by default")
	}
}

func TestDisabledDebugfWritesNothing(t *testing.T) {
	defer reset()
	var buf strings.Builder
	Debugf("resize", "60→55 rows %d→%d", 12, 12)
	if buf.Len() != 0 {
		t.Fatalf("disabled debug wrote output: %q", buf.String())
	}
}

func TestDebugfWritesTaggedLine(t *testing.T) {
	defer reset()
	var buf strings.Builder
	SetWriter(&buf)
	if !Enabled() {
		t.Fatal("SetWriter must enable logging")
	}
	Debugf("resize", "60→55 rows %d→%d", 12, 12)
	out := buf.String()
	if !strings.Contains(out, "[resize]") || !strings.Contains(out, "60→55 rows 12→12") {
		t.Fatalf("debug line missing tag or message: %q", out)
	}
	tsRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}(?:Z|[+-]\d{2}:\d{2}) `)
	if !tsRe.MatchString(out) {
		t.Fatalf("debug line must start with an RFC3339-ms timestamp: %q", out)
	}
}

func TestSetWriterNilDisables(t *testing.T) {
	defer reset()
	var buf strings.Builder
	SetWriter(&buf)
	SetWriter(nil)
	if Enabled() {
		t.Fatal("SetWriter(nil) must disable logging")
	}
	Debugf("resize", "60→55 rows %d→%d", 12, 12)
	if buf.Len() != 0 {
		t.Fatalf("disabled debug wrote output: %q", buf.String())
	}
}

func TestInitFromEnv(t *testing.T) {
	defer reset()
	path := filepath.Join(t.TempDir(), "dohping-debug.log")
	t.Setenv(Env, path)
	Init()
	if !Enabled() {
		t.Fatal("Init with DOHPING_DEBUG set must enable logging")
	}
	Debugf("winch", "SIGWINCH received")
	Close()
	if Enabled() {
		t.Fatal("Close must disable logging")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	if !strings.Contains(string(b), "[winch] SIGWINCH received") {
		t.Fatalf("debug log missing line: %q", string(b))
	}
}

func TestInitUnwritablePathDisables(t *testing.T) {
	defer reset()
	t.Setenv(Env, filepath.Join(t.TempDir(), "no-such-dir", "x.log"))
	Init()
	if Enabled() {
		t.Fatal("Init with an unwritable path must leave logging disabled")
	}
}

func TestInitNoEnvLeavesState(t *testing.T) {
	defer reset()
	t.Setenv(Env, "")
	Init()
	if Enabled() {
		t.Fatal("Init without DOHPING_DEBUG must not enable logging")
	}
	// ...and must not clobber a code-enabled writer.
	var buf strings.Builder
	SetWriter(&buf)
	Init()
	if !Enabled() {
		t.Fatal("Init without DOHPING_DEBUG must not disable an active writer")
	}
}
