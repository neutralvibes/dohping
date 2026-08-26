//go:build windows

// Package console enables ANSI escape processing on the Windows console at
// startup. Classic cmd.exe and PowerShell do not interpret color escape
// codes by default; switching on virtual terminal processing makes them
// render color exactly like Windows Terminal and VS Code, so the tool can
// use ANSI color everywhere on Windows with no per-terminal detection.
package console

import (
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

// EnableVT turns on ANSI virtual terminal processing for stdout and stderr
// on Windows. It is a no-op when stdout is not a console (e.g. piped
// output) and when the console does not support the mode, in which case
// the tool simply falls back to no color. Safe to call once at startup.
func EnableVT() {
	if runtime.GOOS != "windows" {
		return
	}
	enable(os.Stdout)
	enable(os.Stderr)
}

// SupportsUnicodeGlyphs reports whether the console can render the Unicode
// block glyphs used by the liveness animation. Classic cmd.exe and
// PowerShell use codepage fonts without them, so they return false and the
// app falls back to an ASCII spinner. Windows Terminal and VS Code set a
// marker and render the glyphs fine.
func SupportsUnicodeGlyphs() bool {
	if os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != "" || os.Getenv("TERM") != "" {
		return true
	}
	return false
}

func enable(f *os.File) {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return // not a console
	}
	mode |= windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	_ = windows.SetConsoleMode(h, mode)
}
