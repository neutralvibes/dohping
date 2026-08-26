package ping

import (
	"context"

	"dohping/internal/debugx"
)

// fallbackTier is one probe in the escalation chain: a name (for debug
// logs) and the probe itself.
type fallbackTier struct {
	name       string
	probe      Probe
	everWorked bool // true once the tier produced a clean up/down result
}

// fallbackProbe tries an ordered chain of tiers and escalates when one is
// broken. It exists for the "works wherever ping works" contract: a socket
// tier can OPEN for an unprivileged user and still fail every probe (for
// example a ping socket the kernel lets us create but not use), and the
// old code neither fell through to the system ping command nor explained
// itself — the user got a bare "error" with exit 0 on a host where `ping`
// worked fine (user report 2026-08-25, ARM).
//
// Escalation policy: a tier escalates to the next only when it fails on a
// probe AND has never produced a clean up/down. A tier that has proven
// itself is trusted thereafter — later errors are surfaced as-is, because
// a transient operational error must not permanently downgrade a working
// tier to spawning the ping command on every interval. Once escalated,
// the new tier is sticky for the rest of the run.
type fallbackProbe struct {
	tiers []*fallbackTier
	cur   int
}

// Probe runs the active tier, escalating through the chain until one
// produces a clean up/down or every tier has failed. The escalation
// retries inside the same probe call, so the caller sees at most one
// error, and only when no tier could produce a clean result.
func (f *fallbackProbe) Probe(ctx context.Context) Result {
	for {
		t := f.tiers[f.cur]
		res := t.probe.Probe(ctx)
		switch res.Outcome {
		case OutcomeUp, OutcomeDown:
			t.everWorked = true
			return res
		case OutcomeError:
			if ctx.Err() != nil {
				// Shutdown in progress: the run loop drops this result
				// anyway. Do not churn tiers mid-cancel.
				return res
			}
			if t.everWorked || f.cur >= len(f.tiers)-1 {
				// A proven tier or the last tier: surface the error so
				// the caller can report it (permission-class errors
				// still abort with guidance, decision 51).
				return res
			}
			// The tier never produced a clean result: it is broken, not
			// transient. Escalate and retry through the next tier.
			next := f.tiers[f.cur+1]
			debugx.Debugf("probe", "tier %q failed on first probe (%v); escalating to %q",
				t.name, res.Err, next.name)
			_ = t.probe.Close()
			f.cur++
		}
	}
}

// Close closes the active tier. Tiers escalated past were closed at
// escalation time, so closing them again here would be redundant.
func (f *fallbackProbe) Close() error {
	if f.cur < len(f.tiers) {
		return f.tiers[f.cur].probe.Close()
	}
	return nil
}
