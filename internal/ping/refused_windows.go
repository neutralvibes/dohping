package ping

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// isRefused reports whether err means "the host answered no" — a refused
// connection, which proves the host is alive. On Windows a refused dial
// surfaces as WSAECONNREFUSED (10061) — the stdlib's syscall.ECONNREFUSED
// is an invented value that never matches the real winsock error, so it
// must be checked explicitly.
func isRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, windows.WSAECONNREFUSED)
}
