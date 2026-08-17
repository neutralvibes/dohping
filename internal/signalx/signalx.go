// Package signalx centralizes termination-signal handling (spec §15.1):
// SIGINT and SIGTERM are delivered on one channel so the app can map them
// to the Unix exit codes 130/143.
package signalx

import (
	"os"
	"os/signal"
	"syscall"
)

// Signals is the handled termination set.
var Signals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Listen subscribes to termination signals. The returned channel receives
// the triggering signal; stop unsubscribes (call via defer).
func Listen() (ch <-chan os.Signal, stop func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, Signals...)
	return c, func() { signal.Stop(c) }
}
