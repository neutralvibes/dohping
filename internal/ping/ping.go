// Package ping implements the Probe abstraction: a typed probe of a single
// target. Two implementations ship: ICMP echo (default) and TCP connect.
//
// The contract (spec §4.1): a probe result is one of
//
//   - OutcomeUp   — reachability proved (ICMP reply; TCP established or
//     connection refused)
//   - OutcomeDown — no answer within the timeout (TCP SYN silently dropped)
//   - OutcomeError — an operational error (DNS/routing failure, permission
//     denied, socket failure) — never a host-down condition
package ping

import (
	"context"
	"errors"
	"syscall"
	"time"
)

// Outcome classifies a single probe result.
type Outcome int

const (
	// OutcomeUp means the probe proved reachability.
	OutcomeUp Outcome = iota
	// OutcomeDown means the probe got no answer within the timeout.
	OutcomeDown
	// OutcomeError means an operational error prevented probing or
	// interpretation.
	OutcomeError
)

// Result is the typed outcome of one probe.
type Result struct {
	Outcome Outcome
	RTT     time.Duration // valid when Outcome == OutcomeUp
	Err     error         // set when Outcome == OutcomeError
}

// Probe performs one probe of the target. Implementations must respect ctx
// cancellation (the probe must return promptly when ctx is done) and the
// configured per-probe timeout.
type Probe interface {
	Probe(ctx context.Context) Result
	Close() error
}

// IsPermissionError reports whether err is a permission-class failure
// (EPERM/EACCES). Used to detect ICMP socket privilege problems so callers
// can print helpful guidance.
func IsPermissionError(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}
