# dohping

A better interactive ping. `dohping` shows what you actually want from a
ping — is the host up, for how long, and how fast — as a compact, stateful
terminal display, instead of an endless scroll of per-packet lines.

Built for short interactive sessions (minutes to a few days), not
long-term monitoring.

- **Stateful, not noisy** — one line per status period, live-updating in
  place, finalized into history on change.
- **Two views** — plain line mode (default) and a fixed window dashboard.
- **No privileges required** — ICMP via three tiers (raw socket → ping
  socket → system `ping`), or plain TCP connect.
- **Resize-aware** — repaints in place; reflowing terminals are handled
  without fighting them.

## Installation

### From source

Requires Go ≥ 1.26.

```sh
go build -o dohping ./cmd/dohping
```

### Release binaries

Build all supported targets (linux/darwin/windows × amd64/arm64)
reproducibly and generate checksums:

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
misreports a permission problem as a host being down. Note that the ping
fallback's output parser is Linux-iputils-oriented; platforms where the
socket tiers work (macOS unprivileged ICMP, Linux with `CAP_NET_RAW`)
never reach it. Use `--probe tcp` to probe with no ICMP dependency at all
(e.g. `--probe tcp 443`, default port `443`).

## Quick start

```sh
dohping 192.168.1.23              # default: ICMP, plain line mode
dohping example.com -c 5          # 5 probes, then exit 0 (scripted)
dohping --probe tcp example.com   # TCP connect probe, no privileges
dohping --window example.com      # fixed dashboard window
dohping --window-lines 8 example.com   # taller dashboard
dohping -q -l events.log example.com   # quiet + append events to a log
```

## Options

```
Usage:
  dohping [options] HOST

Flags may appear before or after HOST (e.g. `dohping HOST -c 5`).

Probe a single host and display its status as a compact, stateful view:
current status, how long it has held, and RTT statistics — instead of an
endless scroll of per-packet lines.

Options:
  -h, --help                 Show this help and exit
  -V, --version              Show version information and exit
  -i, --interval TIME        Probe interval: seconds (e.g. 5) or a
                             duration (e.g. 500ms, 1m30s) (default 1s)
  -t, --timeout TIME         Probe timeout: seconds (e.g. 5) or a
                             duration (e.g. 500ms, 1m30s) (default 2s)
  -c, --count N              Stop after N probes (default: unlimited)
  -p, --probe TYPE           Probe type: icmp | tcp[:PORT]
                             (default icmp; tcp port default 443)
  -d, --down-after N         Consecutive failures before status flips to
                             down (default 1)
  -u, --up-after N           Consecutive successes before status flips to
                             up (default 1)

Display:
  -q, --quiet                Suppress display output (logging still works)
      --no-header            Do not print the column header
      --no-color             Disable color output
      --color MODE           Color mode: auto, always, never (default auto)
      --live MODE            Live updates: auto, on, off (default auto)
      --no-live              Disable live updating
  -w, --window               Use fixed auto-scrolling window mode
      --no-window            Disable window mode (conflicts with
                             --window-lines)
      --window-lines N       Number of visible lines in window mode.
                             Implies --window. (default 10)
      --timestamp-format F   Display timestamp format: HH:MM:SS | rfc3339
                             (default HH:MM:SS)

Logging:
  -l, --log-file PATH        Append finalized status events to PATH
      --log-format FORMAT    Log format: text | json (default text)

Exit codes:
  0    normal completion (--count exhausted or interactive q quit)
  1    general error
  2    usage or configuration error
  3    probe initialization or permission error
  130  interrupted by SIGINT (Ctrl-C; Unix)
  143  terminated by SIGTERM (Unix)
```

Timing model: the first probe fires immediately, then probes start one
`--interval` apart — so `-c 5 -i 1` completes in about 4 seconds
(5 probes, 4 gaps), exactly like `ping -c 5 -i 1`. The summary's
`run duration` is measured wall-clock time, not `count × interval`.

## Examples

### Watch a host until you interrupt it

```sh
dohping 192.168.1.23
```

ICMP probe every second, plain line mode. The live line updates in place;
`q` quits cleanly (exit 0), `Ctrl-C` also quits (exit 130).

### Scripted check — 5 probes

```sh
dohping -c 5 example.com; echo "exit: $?"
```

Runs 5 probes at 1s intervals (~4s wall time), prints the finalized lines
and a summary, exits 0. In a script or cron, `--quiet --no-color` gives
clean, parseable output.

### TCP probe, no privileges

```sh
dohping --probe tcp example.com        # port 443
dohping --probe tcp:22 example.com     # explicit port
```

Connection established *or refused* means up; a SYN that silently times
out means down; DNS/routing failure is an error (exit 3, not a false
"down").

### Dashboard window

```sh
dohping --window --window-lines 8 example.com
```

A fixed, auto-scrolling dashboard drawn in place on the normal terminal.
Resize it while it runs — it repaints in place.

### Log to a file, watch nothing

```sh
dohping -q -l events.log -i 5 example.com
```

Quiet display, but every status change is appended to `events.log` (text
or `--log-format json`), fsync'd, 0600.

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
  lines on resize; dohping never fights the reflow — it pauses in-place
  updates until the width settles, then continues on a fresh row below
  the frozen (re-wrapped) line, which stays in scrollback as history.
  No cursor queries, works identically on every terminal. Finalized and
  piped/`--no-live` output is unaffected — plain lines.
- when stdout is piped, output is finalized lines only — no ANSI, no
  carriage returns

```text
TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS
11:00:35  192.168.1.23    up    0d 00:35:26    1.70    5.90    2.70
11:05:23  192.168.1.23    down  0d 00:01:05                                23
```

Columns: `TIME` (status start), `HOST`, `STATE`, `DURATION` (`Nd
HH:MM:SS`, capped at `99d+`), `MIN`/`MAX`/`AVG` RTT in ms (blank when
down), `FAILS` (consecutive failed probes while down). The HOST column is
elastic: as wide as the host needs (min 15, max 40, truncated with `…`),
and in window mode it also re-measures against the terminal width on every
resize — long hosts expand when room exists, and a narrow terminal forces
the column to retract rather than wrap. The minimum line is 79 cells
(64 fixed + 15 HOST) — it fits an 80-column terminal. The state column
shows `?` while no probe has established the state yet (the word
`unknown` is used in the exit summary).

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
columns = 79 cells), and if the terminal is narrower than that minimum the
four rightmost columns (MIN, MAX, AVG, FAILS) drop one at a time from the
right, so the line keeps fitting instead of wrapping — down to the
essentials (TIME/HOST/STATE/DURATION, 46 cells). Only below that floor do
lines wrap, and they stay a coherent, non-interleaved stack. On reflowing
terminals (Windows Terminal, Terminal.app, iTerm2) a resize re-wraps the
block in a way the program cannot observe; dohping pauses redraws while
the width is settling (300ms of stability) — but only when the resize
actually re-wraps the block: resizes within the same wrap band repaint in
place with no frozen copy (the freeze is conditional). When a re-wrap does
happen, the block restarts on a fresh row below the frozen rendering —
the old block stays in scrollback as history, and the live data is always
correct from the next repaint. Plain line mode's single live line has its
own freeze treatment (see above).

**A note on resizing — resizing terminals can briefly fracture output.**
On reflowing terminals (Windows Terminal, Terminal.app, iTerm2) a resize
re-wraps the whole screen in ways the program cannot observe mid-reflow.
The window block therefore writes nothing while the width is moving
(300 ms settle after the last change) and then repaints — reclaiming its
own region in place when the resize crossed a wrap boundary, so the
screen settles to exactly one clean block. During the drag itself the
terminal re-wraps the on-screen rendering (transient, unavoidable
without writing mid-reflow). Below ~46 columns even the essentials
(TIME/HOST/STATE/DURATION) cannot fit and the lines wrap — coherently,
but wrapped, and a resize that *settles* there leaves one frozen copy
above the fresh block. Hosts with RTT ≥ 10,000 ms overflow their RTT
columns and widen the line.

## Logging

`--log-file PATH` appends one line per finalized status event (append-only,
never overwritten, no ANSI, fsync'd, 0600). `--log-format text|json`
selects the format (default `text`). Logging is independent of `--quiet`.
IPv6 hosts are bracketed (`[::1]`).

```text
2026-08-16T11:00:35+01:00 host=192.168.1.23 status=up duration_seconds=2126 min_ms=1.70 max_ms=5.90 avg_ms=2.70 fails=0
```

```json
{"time":"2026-08-16T11:00:35+01:00","host":"192.168.1.23","status":"up","duration_seconds":2126,"min_ms":1.7,"max_ms":5.9,"avg_ms":2.7,"fails":0}
```

## Debug logging

The debug facility is **compiled in only when the binary is built with
`-tags debug`** — release builds do not contain it at all. A release
binary ignores `DOHPING_DEBUG` (no file, no warning); CI verifies this on
every build. This keeps diagnostics out of shipped binaries by
construction, not by convention.

To build a debug binary:

```sh
go build -tags debug -o dohping-debug ./cmd/dohping
DOHPING_DEBUG=/tmp/dohping-debug.log ./dohping-debug --window HOST
```

`DOHPING_DEBUG=PATH` enables an optional diagnostic log appended to PATH
(created 0600, same permissions as `--log-file`; a path that cannot be
opened disables the facility with a warning — diagnostics never break a
run). Off by default; enabled only by the environment variable (or in
code, for tests).

The display owns the terminal, so debug output never goes there — it is
file-only by design. Each line is `RFC3339-milliseconds [tag] message`.
Tags: `display` (mode selection), `winch` (SIGWINCH received), `tick`
(1s repaint), `resize` (width observations and the freeze/defer
decision), `redraw` (suppressed, deferred, released, restarted, and
completed repaints).

This is the app's own width telemetry. No terminal displays the width it
is resizing to, so a drag's sweep — every width passed, whether the
block froze or deferred, and where the settle repaint landed — is
reconstructable from this file alone. When diagnosing resize fractures:
run `DOHPING_DEBUG=/tmp/dohping-debug.log dohping --window HOST`, drag
the window around, then inspect the log.

```text
2026-08-18T15:14:46.276+01:00 [winch] SIGWINCH received → repaint
2026-08-18T15:14:46.276+01:00 [resize] 60→55 rows 11→11 → defer (in-place after settle)
2026-08-18T15:14:46.276+01:00 [redraw] deferred (settle, 299.9ms left)
2026-08-18T15:14:47.276+01:00 [redraw] defer released → in-place repaint
2026-08-18T15:14:47.276+01:00 [redraw] repainted tw=55 phys=11 (was 11) rows=11
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
bash scripts/check.sh          # full gate: gofmt, vet, race tests,
                               # golangci-lint, staticcheck, gosec, govulncheck
bash scripts/release.sh        # reproducible cross-compiled binaries + SHA256SUMS
python3 scripts/pty-resize-probe.py   # real-PTY resize scenarios (see header)
```

`scripts/check.sh` is the one definition of "green" — run locally, run by
CI, and run first by `release.sh` (a red gate refuses to build). CI
(GitHub Actions, `.github/workflows/ci.yml`) runs the same gate plus the
PTY probe scenarios and the release matrix on every push to `main` and
every pull request.
