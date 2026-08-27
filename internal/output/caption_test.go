package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"dohping/internal/state"
)

// Resolution caption tests (SPEC §6.1): the `name -> 1.2.3.4` line printed
// once above the header for DNS name targets. The caption is header-like:
// suppressed by --no-header and --quiet, and in window mode it is written
// once at Enter and never part of the block's redraw math (Option A).

func TestDisplayPrintsCaptionAboveHeader(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf, plainLayout("google.com"), "google.com -> 142.250.190.46", false, false, false, nil)
	d.Handle(changeEvent(t0, state.StatusUp))
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) < 2 || lines[0] != "google.com -> 142.250.190.46" {
		t.Errorf("caption not printed above header: %q", buf.String())
	}
	if !strings.Contains(lines[1], "TIME") {
		t.Errorf("header missing below caption: %q", buf.String())
	}
}

func TestDisplayNoHeaderSuppressesCaption(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf, plainLayout("google.com"), "google.com -> 142.250.190.46", false, true, false, nil)
	d.Handle(changeEvent(t0, state.StatusUp))
	if strings.Contains(buf.String(), "->") {
		t.Errorf("caption present despite --no-header: %q", buf.String())
	}
}

func TestDisplayQuietSuppressesCaption(t *testing.T) {
	var buf bytes.Buffer
	d := NewDisplay(&buf, plainLayout("google.com"), "google.com -> 142.250.190.46", true, false, false, nil)
	d.Handle(changeEvent(t0, state.StatusUp))
	if buf.Len() != 0 {
		t.Errorf("quiet mode wrote output: %q", buf.String())
	}
}

func TestWindowEnterPrintsCaptionOnceAboveBlock(t *testing.T) {
	var buf bytes.Buffer
	w := NewWindow(&buf, plainLayout("google.com"), 5, "google.com -> 142.250.190.46", false, false, func() (int, int) { return 120, 24 })
	w.Enter()
	w.Handle(changeEvent(t0, state.StatusUp))
	w.Handle(successEvent(t0.Add(time.Second), state.StatusUp, state.Stats{Count: 1, Min: time.Millisecond, Max: time.Millisecond, Sum: time.Millisecond}, 0))
	// Option A: the caption appears exactly once (the Enter write); block
	// redraws never repeat it and the block rows carry no caption text.
	if got := strings.Count(buf.String(), "-> 142.250.190.46"); got != 1 {
		t.Errorf("caption appears %d times, want 1 (once from Enter, never in the block): %q", got, buf.String())
	}
	if !strings.Contains(buf.String(), "TIME") {
		t.Errorf("window header missing: %q", buf.String())
	}
}

func TestWindowNoHeaderSuppressesCaption(t *testing.T) {
	var buf bytes.Buffer
	w := NewWindow(&buf, plainLayout("google.com"), 5, "google.com -> 142.250.190.46", false, true, func() (int, int) { return 120, 24 })
	w.Enter()
	if buf.Len() != 0 {
		t.Errorf("no-header window printed caption: %q", buf.String())
	}
}

func TestWindowQuietSuppressesCaption(t *testing.T) {
	var buf bytes.Buffer
	w := NewWindow(&buf, plainLayout("google.com"), 5, "google.com -> 142.250.190.46", true, false, func() (int, int) { return 120, 24 })
	w.Enter()
	if buf.Len() != 0 {
		t.Errorf("quiet window printed caption: %q", buf.String())
	}
}
