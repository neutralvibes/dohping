//go:build !windows

// Package console is a no-op on non-Windows platforms: ANSI escape
// processing is already native in Unix terminals, and their fonts render
// the Unicode glyphs.
package console

// EnableVT is a no-op outside Windows.
func EnableVT() {}

// SupportsUnicodeGlyphs is always true outside Windows: Unix terminals
// and their fonts render the block glyphs natively.
func SupportsUnicodeGlyphs() bool { return true }
