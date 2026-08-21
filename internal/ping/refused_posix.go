//go:build !windows

package ping

import (
	"errors"
	"syscall"
)

// isRefused reports whether err means "the host answered no" — a refused
// or reset connection, which proves the host is alive. POSIX surfaces
// this as ECONNREFUSED/ECONNRESET.
func isRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET)
}
