//go:build !debug

// The CI gate for release builds: prove the debug facility is COMPILED
// OUT. These tests run only when debugx is the inert stub (no -tags
// debug) — exactly the code a release binary ships. If the stub ever
// leaks behavior, or a future refactor accidentally builds the real
// debugx into release, these fail.
package debugx

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReleaseBuildHasNoDebugFacility proves DOHPING_DEBUG is completely
// inert in a release build: Init must not open a file, must not warn, and
// nothing must be enabled afterwards.
func TestReleaseBuildHasNoDebugFacility(t *testing.T) {
	// Point DOHPING_DEBUG at a path that must NOT be created.
	dir := t.TempDir()
	path := filepath.Join(dir, "must-not-exist.log")
	t.Setenv(Env, path)

	Init() // must be a silent no-op
	defer Close()

	if Enabled() {
		t.Fatal("release build: debug facility must be compiled out")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("release build: DOHPING_DEBUG created a file at %s", path)
	}
}

// TestReleaseSetWriterCannotEnable proves even an explicit in-code enable
// is impossible in a release build — the facility literally does not
// exist.
func TestReleaseSetWriterCannotEnable(t *testing.T) {
	SetWriter(os.Stderr)
	defer SetWriter(nil)
	if Enabled() {
		t.Fatal("release build: SetWriter must not enable logging")
	}
}

// TestReleaseDebugfWritesNothing proves Debugf never touches any writer.
func TestReleaseDebugfWritesNothing(t *testing.T) {
	// Would panic in the real implementation only if w were non-nil; the
	// stub must simply not write anywhere.
	Debugf("resize", "x → y (release must not log)")
}
