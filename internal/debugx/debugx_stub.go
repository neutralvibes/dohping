//go:build !debug

// Package debugx is the INERT release-build counterpart of the diagnostic
// logger. Release binaries (no `-tags debug`) carry no debug facility at
// all: DOHPING_DEBUG has no effect, nothing can enable logging, and no
// file is ever opened or written. The real implementation lives in
// debugx.go under `//go:build debug`; debugx_stub_test.go proves this
// stub is inert (the CI gate for release builds).
package debugx

import "io"

// Env names the (ignored) environment variable, kept for API parity with
// the debug build. It has no effect in release builds.
const Env = "DOHPING_DEBUG"

// Init is a no-op in release builds: the env var is ignored, no file is
// opened, no warning is printed.
func Init() {}

// Close is a no-op in release builds.
func Close() {}

// Enabled always reports false in release builds — the facility does not
// exist.
func Enabled() bool { return false }

// SetWriter is a no-op in release builds: logging can never be enabled.
func SetWriter(io.Writer) {}

// Debugf is a no-op in release builds.
func Debugf(string, string, ...any) {}
