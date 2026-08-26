package bellx

import (
	"bytes"
	"testing"
	"time"

	"dohping/internal/state"
)

// t0 is the fixed test clock: 2026-08-16 11:00:35 +01:00.
var t0 = time.Date(2026, 8, 16, 11, 0, 35, 0, time.FixedZone("BST", 3600))

// change builds a confirmed EventStatusChange from prev to t.
func change(prev, to state.Status) state.Event {
	return state.Event{Kind: state.EventStatusChange, Time: t0, PrevStatus: prev, Status: to}
}

func TestBellRingsOnUpToDown(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(change(state.StatusUp, state.StatusDown))
	if got := buf.String(); got != "\a" {
		t.Errorf("bell output = %q, want %q", got, "\a")
	}
}

func TestBellRingsOnDownToUp(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(change(state.StatusDown, state.StatusUp))
	if got := buf.String(); got != "\a" {
		t.Errorf("bell output = %q, want %q", got, "\a")
	}
}

func TestBellRingsOnErrorTransition(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(state.Event{Kind: state.EventError, Time: t0, PrevStatus: state.StatusUp, Status: state.StatusError})
	if got := buf.String(); got != "\a" {
		t.Errorf("bell output = %q, want %q", got, "\a")
	}
}

// TestBellSilentOnStartupEstablishment: unknown -> up at launch is the
// monitor coming online, not a state change (SPEC 16.5).
func TestBellSilentOnStartupEstablishment(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(change(state.StatusUnknown, state.StatusUp))
	if buf.Len() != 0 {
		t.Errorf("bell rang on startup establishment: %q", buf.String())
	}
}

// TestBellSilentOnRampFailure: a single probe failure before hysteresis
// flips the status is EventProbeFailure, not a change (SPEC 16.5).
func TestBellSilentOnRampFailure(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(state.Event{Kind: state.EventProbeFailure, Time: t0, Status: state.StatusUp, Fails: 1})
	if buf.Len() != 0 {
		t.Errorf("bell rang on a ramp failure: %q", buf.String())
	}
}

func TestBellSilentOnProbeSuccess(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(state.Event{Kind: state.EventProbeSuccess, Time: t0, Status: state.StatusUp})
	if buf.Len() != 0 {
		t.Errorf("bell rang on a probe success: %q", buf.String())
	}
}

func TestBellSilentWhenDisabled(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, false)
	b.Handle(change(state.StatusUp, state.StatusDown))
	if buf.Len() != 0 {
		t.Errorf("disabled bell wrote: %q", buf.String())
	}
}

// TestBellExactlyOnePerChange: a change event is one event; exactly one
// BEL byte, even though the display will both finalize the old line and
// start the new one.
func TestBellExactlyOnePerChange(t *testing.T) {
	var buf bytes.Buffer
	b := New(&buf, true)
	b.Handle(change(state.StatusUp, state.StatusDown))
	b.Handle(change(state.StatusDown, state.StatusUp))
	if got := buf.String(); got != "\a\a" {
		t.Errorf("two changes should ring exactly two bells, got %q", got)
	}
}

// TestBellNilReceiverSafe: a nil *Bell (the app's "no bell" state) must
// be safe to call.
func TestBellNilReceiverSafe(t *testing.T) {
	var b *Bell
	b.Handle(change(state.StatusUp, state.StatusDown)) // must not panic
}
