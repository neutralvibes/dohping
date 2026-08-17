# dohping

A better interactive ping. `dohping` shows what you actually want from a
ping — is the host up, for how long, and how fast — as a compact, stateful
terminal display, instead of an endless scroll of per-packet lines.

Built for short interactive sessions (minutes to a few days), not
long-term monitoring.

## Installation

### From source

Requires Go ≥ 1.26.

```sh
go build -o dohping ./cmd/dohping
```

### Release binaries

Build all supported targets reproducibly and generate checksums:

```sh
./scripts/release.sh dist
```

Artifacts land in `dist/` as `dohping-<os>-<arch>` plus `SHA256SUMS`.

### ICMP permissions

The default ICMP probe tries, in order:

1. a privileged raw socket (`CAP_NET_RAW` on Linux, admin on Windows),
2. an unprivileged ping socket (`net.ipv4.ping_group_range`),
3. the system `ping` command (works in restricted environments where the
   sandbox elevates `/bin/ping` but the process holds no privileges).

The first tier that works is used; IPv6 targets use ICMPv6 (the ping
fallback passes `-6`). When every tier is unavailable, `dohping` reports a
clear permission error with a hint and exits with code `3` — it never
misreports a permission problem as a host being down, and it aborts as
soon as a probe reports a privilege problem rather than repeating error
lines. Note that the ping
fallback's output parser is Linux-iputils-oriented; platforms where the
socket tiers work (macOS unprivileged ICMP, Linux with `CAP_NET_RAW`)
never reach it. Use `--probe tcp` to probe with no ICMP dependency at all
(e.g. `--probe tcp 443`, default port `443`).

## Usage

```text
dohping [options] HOST
```

Flags may appear before or after `HOST` (e.g. `dohping google.com -c 5`).
`-i`/`-t` accept a bare number of seconds (`-i 5` = 5s, like ping) or a
duration (`-t 500ms`).

```sh
dohping 192.168.1.23              # default: ICMP, plain line mode
dohping --probe tcp example.com   # TCP connect probe, no privileges
dohping --window-lines 8 host     # compact dashboard window
dohping -q -l events.log host     # quiet + append status events to a log
dohping -c 5 host                 # scripted: exactly 5 probes, exit 0
```

Timing model: the first probe fires immediately, then probes start one
`--interval` apart — so `-c 5 -i 1` completes in about 4 seconds
(5 probes, 4 gaps), exactly like `ping -c 5 -i 1`. The summary's
`run duration` is measured wall-clock time, not `count × interval`.

## Display modes

### Plain line mode (default)

- a header is printed unless `--no-header`
- each status period is one line; the current line live-updates in place
  (TTY only) while the status is unchanged
- when the status changes, the previous line is finalized into scrollback
  history and a new line begins
- the live line is width-aware: if the terminal is narrower than the line
  it wraps, and it stays anchored in place across updates (resizes
  self-heal within a second, or immediately on Unix via SIGWINCH).
  Reflowing terminals (Windows Terminal, Terminal.app, iTerm2) re-wrap
  lines on resize; dohping re-anchors by querying the terminal's cursor
  position, so the line never creeps and stale wrap text is cleared.
  Finalized and piped/`--no-live` output is unaffected — plain lines.
- when stdout is piped, output is finalized lines only — no ANSI, no
  carriage returns

```text
TIME      HOST            STATUS  DURATION       MIN     MAX     AVG     FAILS
11:00:35  192.168.1.23    up      0d 00:35:26    1.70    5.90    2.70
11:05:23  192.168.1.23    down    0d 00:01:05                                23
```

Columns: `TIME` (status start), `HOST`, `STATUS`, `DURATION` (`Nd
HH:MM:SS`, capped at `99d+`), `MIN`/`MAX`/`AVG` RTT in ms (blank when
down), `FAILS` (consecutive failed probes while down). The HOST column is
elastic: as wide as the host needs (min 15, max 40, truncated with `…`),
and in window mode it also re-measures against the terminal width on every
resize — long hosts expand when room exists, and a narrow terminal forces
the column to retract rather than wrap.

### Window mode

`--window` (or `--window-lines N`, which implies it) switches to a fixed,
auto-scrolling, non-scrollable dashboard drawn **in place on the normal
terminal** — no alternate screen, no screen clearing. The block shows the
header, the latest finalized lines, and the current live line; the oldest
lines fall off and the block height never grows. When stdout is not a
terminal, window mode falls back to plain line mode with a warning on
stderr (unless `--quiet`). If the terminal is too small, the visible line
count is reduced to fit.

**Terminal resizes are handled** — on Unix a SIGWINCH repaints the block
immediately; on Windows (no SIGWINCH) the 1-second tick re-measures, so
resizes self-heal within a second. Every repaint re-reads the terminal
size: the HOST column retracts/expands to fit (minimum 15 + the fixed
columns = 81 cells), and if the terminal is narrower than that minimum the
lines wrap but the block stays a coherent, non-interleaved stack. On
reflowing terminals (Windows Terminal, Terminal.app, iTerm2) the block
re-anchors via a cursor-position query after a resize, so it never creeps
up the page. The plain line mode's single live line gets the same
treatment (see above).

## Logging

`--log-file PATH` appends one line per finalized status event (append-only,
never overwritten, no ANSI). `--log-format text|json` selects the format
(default `text`). Logging is independent of `--quiet` and is fsync'd after
every event. IPv6 hosts are bracketed (`[::1]`).

```text
2026-08-16T11:00:35+01:00 host=192.168.1.23 status=up duration_seconds=2126 min_ms=1.70 max_ms=5.90 avg_ms=2.70 fails=0
```

```json
{"time":"2026-08-16T11:00:35+01:00","host":"192.168.1.23","status":"up","duration_seconds":2126,"min_ms":1.7,"max_ms":5.9,"avg_ms":2.7,"fails":0}
```

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | normal completion (`--count` exhausted, or interactive `q` quit) |
| 1 | general error (e.g. log file cannot be opened) |
| 2 | usage or configuration error |
| 3 | probe initialization or permission error |
| 130 | terminated by SIGINT / Ctrl-C (Unix) |
| 143 | terminated by SIGTERM (Unix) |

`130` and `143` follow the Unix `128 + signal` convention and apply on
Unix-like systems only.

## Signals and interactive quit

- `SIGINT` / `SIGTERM` trigger a graceful shutdown: probing stops, the
  current line is finalized, the log is flushed, the terminal is restored,
  and the exit code is `130`/`143`.
- In an interactive terminal, pressing `q` (or `Q`) quits through the same
  graceful path with exit code `0`. `Ctrl-C` also works and exits `130`.
  With piped stdin, no key handling occurs.

On interactive exits a short summary is printed (suppressed by
`--quiet`); scripted/piped runs print only finalized status lines.

## Color policy

Color is active only when stdout is a terminal and none of the following
apply:

- `--no-color` or `--color=never`
- `NO_COLOR` set and non-empty (overrides `--color=always`)
- `TERM=dumb`

Colors are semantic (up green, down red, unknown yellow, error magenta,
header bold, timestamp dim, duration cyan, failure count red) and the
theme is one struct in `internal/theme`.

## Probe semantics

- **ICMP** (default): ICMP echo; ICMPv6 for IPv6 targets.
- **TCP** (`--probe tcp[:PORT]`): connection established or refused → up;
  timeout (SYN silently dropped) → down; DNS/routing failure → error.

The hostname is resolved once at startup. Probes never overlap; a failing
probe takes up to `--timeout`, so with `--down-after N` a host flips to
`down` after roughly `N × timeout` of wall-clock time when failing.

## Development

```sh
go build ./...      # build
go vet ./...        # static checks
go test -race ./... # tests with the race detector
gofmt -l .          # formatting (must be empty)
staticcheck ./...   # static analysis
govulncheck ./...   # vulnerability scan (dependency tree)
```

Design decisions are tracked in `DECISIONS.md`; the build contract lives
in `LAUNCH.md` / `Phases.md` / `SPECIFICATION.md`.
