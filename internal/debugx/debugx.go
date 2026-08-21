//go:build debug

// Package debugx is the optional diagnostic logger. It is compiled OUT of
// release builds — this file exists only under `-tags debug` (see
// debugx_stub.go for the inert release counterpart, and
// debugx_stub_test.go for the CI gate proving release binaries can't
// write a debug log). A debug build is enabled by the DOHPING_DEBUG
// environment variable (a file path) or in code via SetWriter. It exists
// because terminal-resize forensics need the app's OWN observations: no
// terminal displays the width it is resizing to, so a drag's sweep —
// every width passed, every freeze/defer decision — is reconstructable
// only from the app's log, never from screenshots or the user's eyes
// (user direction 2026-08-18).
//
// The display owns the terminal, so debug output NEVER goes there: it is
// file-only by design (or an injected writer, in tests). A debug log
// that cannot be opened disables the facility with a stderr warning —
// diagnostics must never break a run.
package debugx

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Env is the environment variable that enables debug logging; its value
// is the file path to append to (created 0600, like the status log).
const Env = "DOHPING_DEBUG"

var (
	mu      sync.Mutex
	w       io.Writer
	file    *os.File
	enabled bool
)

// Init enables debug logging from the environment: DOHPING_DEBUG=<path>
// appends to that file. A path that cannot be opened disables logging
// with a warning — diagnostics never break a run. No-op when the
// variable is unset, and leaves a code-enabled writer untouched.
func Init() {
	path := os.Getenv(Env)
	if path == "" {
		return
	}
	// The path is the operator's own environment variable (a CLI, not a
	// server: no untrusted boundary to cross) — #nosec G304, G703 (G703
	// is the taint-analysis twin of G304 on the same line: the operator
	// who sets DOHPING_DEBUG is the operator who runs the binary).
	// 0600, not 0644: the log carries host/timestamp/width observations
	// and must not be world-readable by default (gosec G302; same rule
	// as logx, safe-by-default).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304,G703 -- operator-controlled env path
	if err != nil {
		fmt.Fprintf(os.Stderr, "dohping: warning: cannot open debug log %q: %v (debug disabled)\n", path, err)
		return
	}
	mu.Lock()
	file = f
	w = f
	enabled = true
	mu.Unlock()
}

// Close flushes and closes the env-opened debug log and disables the
// facility. Idempotent; pairs with Init (deferred by the app entry
// point). A writer injected via SetWriter is NOT closed — tests own it.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
	w = nil
	enabled = false
}

// Enabled reports whether debug logging is active.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// SetWriter redirects debug output to wr and enables logging — the
// in-code enablement for tests and embedding. Pass nil to disable. Do
// not mix with Init in the same process (tests pair SetWriter with
// SetWriter(nil), never Close).
func SetWriter(wr io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	w = wr
	enabled = wr != nil
}

// Debugf writes one tagged diagnostic line when logging is enabled:
//
//	2026-08-18T14:30:00.123+01:00 [tag] message
//
// The tag keeps forensics greppable (resize, winch, redraw, tick,
// display). The timestamp carries milliseconds — a resize drag fires
// many width observations per second, and the ordering matters.
// No-op when disabled.
func Debugf(tag, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !enabled || w == nil {
		return
	}
	ts := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	fmt.Fprintf(w, "%s [%s] %s\n", ts, tag, fmt.Sprintf(format, args...))
}
