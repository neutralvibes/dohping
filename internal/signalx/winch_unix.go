//go:build !windows

package signalx

import (
	"os"
	"os/signal"
	"syscall"
)

// Winch subscribes to SIGWINCH (terminal resize). The returned channel
// receives on every resize; stop unsubscribes (call via defer). Only
// meaningful on Unix; see winch_windows.go for the no-op variant.
func Winch() (ch <-chan os.Signal, stop func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGWINCH)
	return c, func() { signal.Stop(c) }
}
