//go:build !windows

// Package console is a no-op on non-Windows platforms: ANSI escape
// processing is already native in Unix terminals.
package console

// EnableVT is a no-op outside Windows.
func EnableVT() {}
