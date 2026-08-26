// Package bellx sounds the terminal bell (\a, BEL) on confirmed status
// changes. It is a peer consumer of the event stream: it knows nothing
// about the display, the layout, or terminal geometry, and writes only
// the single BEL byte to the caller's writer. It never writes to a log
// file — the logger is a separate writer fed by a separate call.
//
// The bell is emitted as a standalone leading write, separate from any
// display escape framing, so BEL (which is cursor-inert and color-inert)
// cannot corrupt the display's output.
package bellx

import (
	"fmt"
	"io"

	"dohping/internal/state"
)

// Bell emits the terminal bell on confirmed status transitions.
type Bell struct {
	w       io.Writer
	enabled bool
}

// New returns a Bell that writes to w while enabled. Disabling is the
// caller's decision (e.g. --bell not given, stdout not a terminal, quiet
// mode); a disabled Bell never writes a byte.
func New(w io.Writer, enabled bool) *Bell {
	return &Bell{w: w, enabled: enabled}
}

// Handle sounds the bell once if ev is a confirmed status change: the
// state engine has flipped between established states (EventStatusChange
// for up/down flips, EventError for entering the error state). The
// initial establishment from the startup unknown state is not a change
// and never rings; a single probe failure during the hysteresis ramp is
// EventProbeFailure, not a change, and never rings.
func (b *Bell) Handle(ev state.Event) {
	if b == nil || !b.enabled {
		return
	}
	if !confirmedChange(ev) {
		return
	}
	_, _ = fmt.Fprint(b.w, "\a")
}

// confirmedChange reports whether ev is a confirmed transition between
// established states. The PrevStatus guard excludes the startup
// establishment event (unknown -> up/down/error), which the engine also
// reports as EventStatusChange/EventError but which is the monitor coming
// online, not a state change.
func confirmedChange(ev state.Event) bool {
	switch ev.Kind {
	case state.EventStatusChange, state.EventError:
		return ev.PrevStatus != state.StatusUnknown
	}
	return false
}
