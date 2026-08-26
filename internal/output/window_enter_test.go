package output

import (
	"bytes"
	"testing"
)

// Enter/Exit pin the documented decision #53 contract: window mode renders
// in place on the normal terminal, takes over no screen state, and writes
// nothing on enter/exit. A regression here would break the "no alternate
// screen, no cursor-home, no clear-to-end-of-screen" promise.

func TestWindowEnterExitWriteNothing(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWindowSized(&buf, 5, false, false, 120, 24)
	w.Enter()
	w.Exit()
	if buf.Len() != 0 {
		t.Errorf("Enter/Exit wrote %d bytes to the terminal, want 0: %q", buf.Len(), buf.String())
	}
	// Same with a quiet window — Enter/Exit are structural no-ops regardless.
	var buf2 bytes.Buffer
	wq := newTestWindowSized(&buf2, 5, true, true, 120, 24)
	wq.Enter()
	wq.Exit()
	if buf2.Len() != 0 {
		t.Errorf("quiet Enter/Exit wrote %d bytes, want 0", buf2.Len())
	}
}
