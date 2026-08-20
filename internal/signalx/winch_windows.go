//go:build windows

package signalx

import (
	"os"
	"os/signal"
)

// Winch is a no-op on Windows: there is no SIGWINCH, and resize is not
// tracked. The channel never delivers.
func Winch() (ch <-chan os.Signal, stop func()) {
	c := make(chan os.Signal)
	return c, func() { signal.Stop(c) }
}
