# dohping Engineering Principles

Version: 0.1.0-draft
Status: Draft — house standard for implementation, distilled from SPECIFICATION.md (§2, §20) and the decision log (DECISIONS.md). Edit as implementation clarifies.

## 1. Architecture

- Layering is strict: probe execution → state engine → semantic events → display/logging.
- The state engine knows nothing about terminals, colors, windows, or log files. It emits semantic events only: `status changed`, `probe succeeded`, `probe failed`, `statistics updated`, `error occurred`.
- D.R.Y. for shared logic (duration/RTT formatting, column layout, color theme, state transitions, field rendering); clarity beats forced abstraction.

## 2. Testability

- State engine tests use a fake pinger and a fake clock — deterministic, no network access.
- TCP probe tests use a loopback `net.Listen` listener — no privileges.
- Golden tests for all output shapes (header, up/down/error lines, color-disabled, non-TTY, host-width edge cases).
- `go test -race ./...` must pass: no data races, no goroutine leaks; context cancellation stops the probe loop cleanly.

## 3. Probe contract

- The `Probe` interface returns a typed result: OK / RTT / error class.
- Two implementations: ICMP (default; ICMPv6 for IPv6 targets) and TCP connect (`--probe tcp[:port]`, default 443).
- TCP semantics: connection established or refused → `up`; timeout → `down`; DNS/routing failure → `error`.
- DNS resolved once at startup; probes dial the resolved address.
- `--timeout` applies per probe; probes never overlap; a failing probe takes up to `--timeout`, so `--down-after N` ≈ `N × timeout` wall-clock.

## 4. Output rules

- Fixed-width columns; widths computed once at startup (single host per run, so they never change mid-run).
- Live updates only where appropriate (TTY, auto/on/off); never ANSI or cursor control when piped; never in log files.
- Color obeys the disable rules incl. `NO_COLOR` as override; meaning is never color-only.

## 5. Concurrency & lifecycle

- `context.Context` everywhere; cancellable probe loops; clean shutdown paths.
- One stdin key-reader goroutine (interactive `q` quit) feeding the same shutdown path as signals.
- No global mutable state; synchronized shared state or event channels; monotonic time for durations, wall-clock for timestamps.

## 6. CLI contract

- Mutually opposed explicit flags are a usage error (exit 2) — see SPECIFICATION.md §16.4. Never silent ambiguity.
- Predictable exit codes; `130`/`143` are Unix conventions (documented per platform).
- Short flags: `-h -V -i -t -c -q -l -p -w -d -u`; `-v` is deliberately avoided (verbose convention elsewhere).

## 7. Dependencies

- Minimal and justifiable, actively maintained: `golang.org/x/net` (ICMP), `golang.org/x/term` (terminal detection, raw-mode key reading). Everything else standard library.

## 8. Cross-platform

- Linux/macOS/Windows builds must succeed; platform-specific behavior (exit codes, ICMP privileges) is documented, not papered over.
