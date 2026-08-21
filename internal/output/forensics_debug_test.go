//go:build debug

package output

// Resize-forensics tests — they assert on the debug facility's output
// (debugx.SetWriter), so they exist only under `-tags debug`, alongside
// the real debugx implementation. In release builds the facility is
// compiled out; debugx_stub_test.go proves the release stub is inert.
// Kept in this dedicated file so the release test files never touch the
// debug API.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"dohping/internal/debugx"
	"dohping/internal/state"
)

// TestDisplayResizeDebugForensics proves plain-mode resize episodes are
// logged through the debug facility (DECISIONS #74) — the width change and
// the settle restart — so a resize episode in plain mode is fully
// reconstructable from the app's own log, the same way window mode is.
func TestDisplayResizeDebugForensics(t *testing.T) {
	var dbg strings.Builder
	debugx.SetWriter(&dbg)
	defer debugx.SetWriter(nil)

	var buf bytes.Buffer
	d, wPtr, _, now := newTestDisplayResizable(&buf, false, false, true, 60, 24)
	*wPtr = 60
	d.Handle(changeEvent(t0, state.StatusUp))
	d.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
		buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
	dbg.Reset()

	*wPtr = 55
	*now = now.Add(100 * time.Millisecond) // mid-reflow: nothing written
	d.Tick()
	*now = now.Add(time.Second) // width stable: fresh row below the frozen line
	d.Tick()

	log := dbg.String()
	if !strings.Contains(log, "plain 60→55") || !strings.Contains(log, "freeze") {
		t.Errorf("plain resize change not logged: %q", log)
	}
	if !strings.Contains(log, "settled") {
		t.Errorf("plain settle restart not logged: %q", log)
	}
}

// TestWindowResizeDebugForensics proves window-mode resize episodes log
// the freeze/defer decision and the settled in-place repaint; a band
// crossing must log the reclaim. This is the app's own width telemetry —
// the only record of the widths a drag passes through, since no terminal
// displays them. The log is the evidence contract for the user's
// real-terminal drag tests.
func TestWindowResizeDebugForensics(t *testing.T) {
	var dbg strings.Builder
	debugx.SetWriter(&dbg)
	defer debugx.SetWriter(nil)

	t.Run("same-band defers then repaints in place", func(t *testing.T) {
		var buf bytes.Buffer
		w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 60, 24)
		*wPtr = 60
		w.Handle(changeEvent(t0, state.StatusUp))
		w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
			buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
		dbg.Reset()

		*wPtr = 55
		*now = now.Add(100 * time.Millisecond) // mid-reflow: nothing written
		buf.Reset()
		w.Tick()
		if buf.Len() != 0 {
			t.Fatalf("same-band mid-reflow write: %q", buf.String())
		}
		*now = now.Add(300 * time.Millisecond) // width stable: in-place repaint
		buf.Reset()
		w.Tick()
		if buf.Len() == 0 {
			t.Fatal("settled same-band resize must repaint")
		}
		log := dbg.String()
		if !strings.Contains(log, "60→55") || !strings.Contains(log, "defer") {
			t.Errorf("resize decision not logged: %q", log)
		}
		if !strings.Contains(log, "repainted tw=55") {
			t.Errorf("settled repaint not logged: %q", log)
		}
		if strings.Contains(log, "FREEZE") || strings.Contains(log, "restart") {
			t.Errorf("same-band resize must not log a freeze/restart: %q", log)
		}
	})

	t.Run("band crossing reclaims in place", func(t *testing.T) {
		var buf bytes.Buffer
		w, wPtr, _, now := newTestWindowResizable(&buf, "frigate.app.home", 5, false, false, 45, 24)
		*wPtr = 45
		w.Handle(changeEvent(t0, state.StatusUp))
		w.Handle(successEvent(t0.Add(2*time.Second), state.StatusUp,
			buildStats(time.Millisecond, time.Millisecond, time.Millisecond, 1), 0))
		dbg.Reset()

		*wPtr = 60
		*now = now.Add(100 * time.Millisecond) // mid-reflow: nothing written
		buf.Reset()
		w.Tick()
		if buf.Len() != 0 {
			t.Fatalf("crossing mid-reflow write: %q", buf.String())
		}
		*now = now.Add(time.Second) // width stable: reclaim in place
		buf.Reset()
		w.Tick()
		log := dbg.String()
		if !strings.Contains(log, "45→60") || !strings.Contains(log, "crossing") {
			t.Errorf("crossing decision not logged: %q", log)
		}
		if !strings.Contains(log, "reclaim in place") {
			t.Errorf("reclaim decision not logged: %q", log)
		}
		if !strings.Contains(log, "reclaimed tw=60") {
			t.Errorf("reclaimed repaint not logged: %q", log)
		}
	})
}
