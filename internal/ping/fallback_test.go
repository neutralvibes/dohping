package ping

import (
	"context"
	"errors"
	"syscall"
	"testing"
)

// stubProbe is a scriptable Probe for fallback-chain tests: it replays
// the configured results in order, repeating the last one.
type stubProbe struct {
	results []Result
	closes  int
}

func (s *stubProbe) Probe(context.Context) Result {
	if len(s.results) == 0 {
		return Result{Outcome: OutcomeError, Err: errors.New("stub exhausted")}
	}
	r := s.results[0]
	if len(s.results) > 1 {
		s.results = s.results[1:]
	}
	return r
}

func (s *stubProbe) Close() error {
	s.closes++
	return nil
}

func stubTier(name string, results ...Result) *fallbackTier {
	return &fallbackTier{name: name, probe: &stubProbe{results: results}}
}

// TestFallbackEscalatesOnFirstError is the ARM report in miniature: the
// active tier opens but its first probe fails, so the chain escalates and
// the next tier answers. After escalation the new tier is sticky.
func TestFallbackEscalatesOnFirstError(t *testing.T) {
	t1 := stubTier("broken", Result{Outcome: OutcomeError, Err: syscall.EPERM})
	t2 := stubTier("working", Result{Outcome: OutcomeUp, RTT: 1})
	f := &fallbackProbe{tiers: []*fallbackTier{t1, t2}}

	r := f.Probe(context.Background())
	if r.Outcome != OutcomeUp {
		t.Fatalf("probe = %v, want up (err=%v)", r.Outcome, r.Err)
	}
	if p := t1.probe.(*stubProbe); p.closes != 1 {
		t.Errorf("broken tier closed %d times, want 1", p.closes)
	}

	// Second probe must stay on tier 2 — tier 1 must not be consulted.
	if r := f.Probe(context.Background()); r.Outcome != OutcomeUp {
		t.Fatalf("second probe = %v, want up (sticky tier)", r.Outcome)
	}
}

// TestFallbackKeepsWorkingTier: a tier that answers cleanly is never
// escalated away from.
func TestFallbackKeepsWorkingTier(t *testing.T) {
	t1 := stubTier("working", Result{Outcome: OutcomeDown})
	t2 := stubTier("never-used", Result{Outcome: OutcomeUp})
	f := &fallbackProbe{tiers: []*fallbackTier{t1, t2}}

	for i := 0; i < 2; i++ {
		if r := f.Probe(context.Background()); r.Outcome != OutcomeDown {
			t.Fatalf("probe %d = %v, want down", i, r.Outcome)
		}
	}
	if p := t1.probe.(*stubProbe); p.closes != 0 {
		t.Errorf("working tier closed %d times, want 0", p.closes)
	}
}

// TestFallbackSurfacesTransientErrorOnProvenTier: once a tier has produced
// a clean result, a later error is a transient operational failure and is
// surfaced, NOT escalated (a working tier must not be permanently
// downgraded to the ping command over one hiccup).
func TestFallbackSurfacesTransientErrorOnProvenTier(t *testing.T) {
	t1 := stubTier("working-then-hiccup",
		Result{Outcome: OutcomeUp, RTT: 1},
		Result{Outcome: OutcomeError, Err: errors.New("network is unreachable")})
	t2 := stubTier("fallback", Result{Outcome: OutcomeUp})
	f := &fallbackProbe{tiers: []*fallbackTier{t1, t2}}

	if r := f.Probe(context.Background()); r.Outcome != OutcomeUp {
		t.Fatalf("first probe = %v, want up", r.Outcome)
	}
	r := f.Probe(context.Background())
	if r.Outcome != OutcomeError {
		t.Fatalf("second probe = %v, want error (surfaced, not escalated)", r.Outcome)
	}
	if p := t1.probe.(*stubProbe); p.closes != 0 {
		t.Errorf("proven tier closed %d times, want 0 (no escalation)", p.closes)
	}
}

// TestFallbackAllTiersFail: when every tier errors, the last error is
// surfaced so the caller can report it (permission-class still aborts in
// the app via decision 51).
func TestFallbackAllTiersFail(t *testing.T) {
	t1 := stubTier("a", Result{Outcome: OutcomeError, Err: errors.New("first")})
	t2 := stubTier("b", Result{Outcome: OutcomeError, Err: errors.New("second")})
	t3 := stubTier("c", Result{Outcome: OutcomeError, Err: syscall.EPERM})
	f := &fallbackProbe{tiers: []*fallbackTier{t1, t2, t3}}

	r := f.Probe(context.Background())
	if r.Outcome != OutcomeError {
		t.Fatalf("probe = %v, want error", r.Outcome)
	}
	if !IsPermissionError(r.Err) {
		t.Errorf("error = %v, want permission-class (last tier wins)", r.Err)
	}
	// Every tier reached and closed.
	if p := t1.probe.(*stubProbe); p.closes != 1 {
		t.Errorf("tier a closed %d times, want 1", p.closes)
	}
	if p := t2.probe.(*stubProbe); p.closes != 1 {
		t.Errorf("tier b closed %d times, want 1", p.closes)
	}
}

// TestFallbackSingleTierSurfacesError: a one-tier chain has nothing to
// escalate to; its error is returned as-is.
func TestFallbackSingleTierSurfacesError(t *testing.T) {
	t1 := stubTier("only", Result{Outcome: OutcomeError, Err: errors.New("boom")})
	f := &fallbackProbe{tiers: []*fallbackTier{t1}}

	r := f.Probe(context.Background())
	if r.Outcome != OutcomeError {
		t.Fatalf("probe = %v, want error", r.Outcome)
	}
	if p := t1.probe.(*stubProbe); p.closes != 0 {
		t.Errorf("only tier closed %d times, want 0 (never escalated)", p.closes)
	}
}

// TestFallbackCloseClosesUpToActiveTier: Close releases every tier in the
// chain up to and including the active one.
func TestFallbackCloseClosesUpToActiveTier(t *testing.T) {
	t1 := stubTier("broken", Result{Outcome: OutcomeError, Err: syscall.EPERM})
	t2 := stubTier("working", Result{Outcome: OutcomeUp})
	f := &fallbackProbe{tiers: []*fallbackTier{t1, t2}}

	_ = f.Probe(context.Background()) // escalate: t1 closed, t2 active
	_ = f.Close()
	if p := t1.probe.(*stubProbe); p.closes != 1 {
		t.Errorf("tier a closed %d times, want 1", p.closes)
	}
	if p := t2.probe.(*stubProbe); p.closes != 1 {
		t.Errorf("tier b closed %d times, want 1", p.closes)
	}
}
