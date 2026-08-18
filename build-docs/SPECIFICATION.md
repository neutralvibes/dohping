# dohping Specification

Version: 0.1.0-draft
Date: 2026-08-16
Status: Proposed

## 1. Overview

`dohping` is a ping/status utility written in Go — a *better ping* for interactive use. Its purpose is to present the information most people actually want from ping — is the host up, for how long, and how fast — as a compact, stateful terminal display, rather than an endless scroll buffer of per-packet lines. It is designed for short interactive sessions (minutes to a few days), not long-term monitoring; there are better tools for that.

Unlike traditional ping tools that print one line per packet, `dohping` presents stateful status information:

- current status
- how long the host has remained in that status
- minimum, maximum, and average RTT while up
- consecutive failures while down
- graceful handling of interrupts and termination signals
- optional plain log output without colors

The tool should be suitable for:

- interactive monitoring
- small terminal dashboards
- lightweight status displays
- scripted or logged status events

## 2. Design Principles

`dohping` should follow these principles:

1. Production-level code quality.
2. Easily maintainable architecture.
3. D.R.Y. implementation except where duplication improves clarity.
4. Professional terminal user experience.
5. Predictable behavior for both humans and scripts.
6. Safe defaults for interactive and non-interactive use.
7. Clear separation between:
   - probe execution
   - state tracking
   - display rendering
   - logging
   - signal handling
8. Sensible shortcuts for long form args where not specified.

## 3. Scope

### 3.1 Initial Scope

The initial version should support:

- monitoring a single host
- probe abstraction with ICMP (default) and TCP connect probe types
- configurable interval and timeout
- up/down status tracking
- live-updating plain line mode
- optional fixed-size auto-scrolling window mode
- optional plain log file output
- graceful shutdown (signals, or interactive `q` quit)
- colorized terminal output with standard overrides

### 3.2 Non-Goals for Initial Release

The following are not required for the first release:

- multi-host dashboards
- full TUI with mouse support
- historical graphs
- persistent local database storage
- web UI
- Prometheus metrics endpoint
- HTTP/UDP probe modes
- distributed monitoring

These may be considered later.

## 4. Probe Behavior

### 4.1 Probe Types

`dohping` probes through an abstracted `Probe` interface with two implementations:

| Type | Value | Notes |
|---|---|---|
| ICMP | `icmp` (default) | ICMP echo request; ICMPv6 for IPv6 targets. Requires `CAP_NET_RAW`/root on Linux; permission errors are operational errors, never host-down. |
| TCP | `tcp[:PORT]` | TCP connect probe, default port `443`. No privileges required; works through most firewalls that filter ICMP. |

TCP connect semantics:

- connection established — host alive, port open → `up`
- connection refused — host alive (it answered "no", which proves presence) → `up`
- timeout (SYN silently dropped) — unreachable or filtered → `down`
- DNS or routing failure → `error`

The hostname is resolved once at startup; probes then dial the resolved address. `--timeout` applies per probe (as the dial timeout for TCP).

Operational errors (permission denied, unsupported platform, invalid host, DNS resolution failure, network unreachable) must be clearly reported with helpful guidance and must never be silently treated as ordinary host-down conditions.

### 4.2 Probe Timing

The probe loop should be controlled by:

- `--interval`
- `--timeout`

Recommended defaults:

```text
--interval 1s
--timeout  2s
```

Rules:

- Probes should not overlap by default.
- If a probe exceeds the timeout, it should be treated as failed.
- If timeout is greater than interval, the next probe should start after the previous probe completes.
- Timing should be context-aware and cancellable.

### 4.3 Status Hysteresis

To avoid flapping, `dohping` may support transition thresholds:

```text
--down-after N
--up-after N
```

Recommended defaults:

```text
--down-after 1
--up-after 1
```

Meaning:

- host becomes down after N consecutive failed probes
- host becomes up after N consecutive successful probes

Implementations may choose higher defaults, but the behavior must be documented.

Effective detection latency: probes do not overlap, and a failing probe takes up to `--timeout`. When the host is failing, `--down-after N` therefore means roughly `N × timeout` of wall-clock time before the status flips to `down` (e.g. `--down-after 3` with a 2s timeout ≈ 6s).

## 5. Status Model

### 5.1 States

`dohping` should maintain at least the following states:

| State | Meaning |
|---|---|
| `unknown` | No successful or failed probe has established status yet |
| `up` | Host is considered reachable |
| `down` | Host is considered unreachable |
| `error` | Operational error prevents probing or interpretation |

### 5.2 State Duration

Each status has a start time and duration.

The duration displayed in the main output should mean:

> the amount of time the host has remained in the current status

When the status changes, the previous state duration becomes final and a new state begins.

### 5.3 RTT Statistics

RTT statistics are maintained per current status.

For an `up` state, track:

- minimum RTT
- maximum RTT
- average RTT

For a `down` state, RTT fields should be blank or omitted.

RTT values should be displayed in milliseconds.

### 5.4 Failure Count

For a `down` state, display the number of consecutive failed probes.

This field should be named `FAILS`, not `RETRIES`, to avoid ambiguity.

## 6. Display Output

Display output is a core feature of `dohping`.

There are two display modes:

1. Plain line mode.
2. Fixed auto-scrolling window mode.

Plain line mode is the default.

If --window-lines is present and window mode has not been explicitly disabled, window mode is enabled automatically.



## 7. Plain Line Mode

Plain line mode is the default display mode.

### 7.1 Behavior

In plain line mode:

- A header is displayed unless disabled.
- Each status period is represented by one line.
- The last/current line is live-updated while the status remains unchanged.
- When the status changes, the previous line is finalized.
- Finalized lines become normal scrollback history.
- Live updating should be enabled by default when stdout is an interactive terminal.
- Live updating should automatically disable when stdout is piped or redirected, unless explicitly forced.

### 7.2 Live Updating

The live line updates the following fields while the status remains unchanged:

- status duration
- minimum RTT
- maximum RTT
- average RTT
- failure count

The `TIME` field should remain stable and represent the time the current status began.

The live line should not continuously change the `TIME` field merely because the display refreshed.

### 7.3 Finalization

A line is finalized when:

- status changes
- the program exits gracefully
- an error terminates monitoring

When finalized, the line should be preserved as historical output.

### 7.4 Plain Line Mode Example

```text
TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS
11:00:35  192.168.1.23    up    0d 00:35:26    1.70    5.90    2.70
11:05:23  192.168.1.23    down  0d 00:01:05                                23
13:34:11  192.168.1.23    up    2d 00:45:26    1.00    2.50    1.70
```

Only the last line is live-updated.

## 8. Window Mode

Window mode is an optional fixed-size display mode.

### 8.1 Purpose

Window mode provides a small auto-scrolling, non-scrollable status window.

It keeps only a limited number of the latest visible lines.

It is intended as a compact dashboard display.

```text
--window
    Enables fixed auto-scrolling window mode.
    When enabled without --window-lines, the default visible line count is used.

--window-lines N
    Sets the number of visible lines in window mode.
    This option implies --window.
    Default: 10.
```

### 8.2 Activation

Window mode is enabled with:

```text
--window
```

Window mode should not be the default.

### 8.3 Visible Line Limit

Window mode should display a limited number of lines:

```text
--window-lines N
```

Recommended default:

```text
--window-lines 10
```

The visible window contains:

- optional header
- latest finalized status lines
- current live line
- optional summary/status bar

The exact number of visible status lines depends on header and summary usage.

### 8.4 Auto-Scrolling Behavior

Window mode should maintain a rolling view of the latest entries.

When a new status line is added:

1. The current live line becomes finalized.
2. The finalized line enters the visible history.
3. A new live line is created.
4. If the visible history exceeds the configured limit, the oldest visible line is removed.

The user should not be able to scroll back through full history in window mode.

The screen display is intentionally limited to the most recent entries.

### 8.5 Rendering

Window mode should redraw only the fixed display region.

It should not rely on normal terminal scrollback.

Implementations should:

- use cursor positioning carefully
- avoid flicker
- clear stale characters
- handle terminal resize gracefully
- restore terminal state on exit
- preferably use an alternate screen or dedicated display region

If the terminal is too small, `dohping` should either:

- reduce the number of visible lines, or
- fall back to plain line mode with a diagnostic message

### 8.6 Non-Interactive Output

Window mode should not be used when stdout is not a terminal.

If `--window` is specified and stdout is redirected or piped, `dohping` should fall back to plain line mode and should emit a diagnostic warning unless quiet mode is active.

### 8.7 Window Mode Example

With `--window-lines 5`:

```text
TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS
11:05:23  192.168.1.23    down  0d 00:01:05                                23
13:34:11  192.168.1.23    up    2d 00:45:26    1.00    2.50    1.70
15:02:44  192.168.1.23    down  0d 00:00:03                                 1
15:02:58  192.168.1.23    up    0d 00:00:07    0.98    1.12    1.05
15:03:10  192.168.1.23    up    0d 00:00:19    0.98    1.30    1.11
```

When another status line appears, the oldest visible line is removed.

## 9. Output Columns

The preferred column set is:

```text
TIME HOST STATE DURATION MIN MAX AVG FAILS
```

### 9.1 Column Definitions

| Column | Meaning |
|---|---|
| `TIME` | Time when the current status began |
| `HOST` | Target host or address |
| `STATE` | Current state: `up`, `down`, `?` (not yet established), or `error` |
| `DURATION` | Time spent in the current status |
| `MIN` | Minimum RTT in milliseconds during current status |
| `MAX` | Maximum RTT in milliseconds during current status |
| `AVG` | Average RTT in milliseconds during current status |
| `FAILS` | Consecutive failed probes during current down state |

### 9.2 Deprecated or Discouraged Names

The following names are discouraged:

```text
SHORT
LONG
RETRIES
```

Preferred replacements:

```text
MIN
MAX
FAILS
```

### 9.3 Units

RTT values are milliseconds.

Duration should be human-readable and stable.

Recommended duration format:

```text
Nd HH:MM:SS
```

Examples:

```text
0d 00:35:26
2d 00:45:26
```

For machine-readable logs, duration should also be available as raw seconds.

### 9.4 Column Width Policy

- `HOST` width is computed once at startup from the actual target string (minimum 15 characters, maximum 40). An IPv6 literal may therefore widen the column for that run; the width never changes mid-run because the tool monitors a single host.
- Hosts longer than 40 characters are truncated with `…` in the display; the full value is preserved in logs.
- `DURATION` uses the fixed form `Nd HH:MM:SS`; as a guard, values at or beyond 100 days display as `99d+`. (The tool targets short interactive sessions; extended monitoring is out of scope.)

## 10. Output Formatting Rules

Display output should be visually stable and professional.

### 10.1 Alignment

Columns should be fixed-width or consistently aligned.

Numeric fields should align cleanly.

### 10.2 RTT Formatting

RTT values should use consistent decimal formatting.

Recommended:

```text
1.00
2.50
12.34
```

### 10.3 Blank Fields

Fields that do not apply should be blank.

Example:

```text
11:05:23  192.168.1.23    down    0d 00:01:05                                23
```

RTT fields are blank when down.

Failure count may be blank when up.

### 10.4 Timestamp Formatting

Default terminal timestamp format:

```text
HH:MM:SS
```

Log files should use a more complete timestamp format, preferably RFC 3339.

Example:

```text
2026-08-16T13:34:11+01:00
```

## 11. Color Support

`dohping` should support colorized terminal output.

### 11.1 Semantic Colors

Colors should be defined by semantic role, not hardcoded throughout output logic.

Suggested roles:

```text
status_up
status_down
status_unknown
status_error
header
timestamp
duration
value
warning
```

### 11.2 Default Color Theme

A reasonable default theme:

| Element | Suggested Color |
|---|---|
| `up` | green |
| `down` | red |
| `unknown` | yellow |
| `error` | magenta |
| header | bold |
| timestamp | dim |
| duration | cyan |
| RTT values | default |
| failure count | red when non-zero |

The theme should be easily modifiable in the source.

A later version may support user-defined themes.

### 11.3 Color Disable Rules

Color output must be disabled if any of the following apply:

1. `--no-color` is specified.
2. `NO_COLOR` environment variable is set to a non-empty value.
3. stdout is not a terminal.
4. `TERM` is `dumb`.
5. `--color=never` is specified.

`NO_COLOR` should act as an override.

If `NO_COLOR` is set, color should be disabled even if `--color=always` is supplied, unless an explicit override flag is intentionally provided in a future version.

Mutually opposed color flags are a usage error: `--no-color` (or `--color=never`) combined with `--color=always` exits with code `2` and a clear message (see §16.4).

### 11.4 Log Files

Log file output must never contain ANSI color codes or terminal cursor-control sequences.

## 12. Quiet Mode

Quiet mode is selected with:

```text
-q, --quiet
```

When quiet mode is enabled:

- normal display output is suppressed
- window mode is suppressed
- plain line mode is suppressed
- live updates are suppressed
- log file output still occurs if configured
- fatal errors should still be reported to stderr

Quiet mode does not mean “ignore errors”.

## 13. No Header Mode

Header display can be disabled with:

```text
--no-header
```

This suppresses the visible table header in display modes.

It applies to terminal display.

It does not prevent log files from using structured fields or timestamps.

## 14. Logging

Logging is enabled with:

```text
-l, --log-file PATH
```

### 14.1 Log Purpose

The log file records durable events.

It is not a live-updating terminal view.

It should be append-only.

It should not attempt to overwrite previous lines.

### 14.2 Log Content

The log file should record finalized status events.

At minimum, each log event should include:

- timestamp
- host
- status
- duration in seconds
- failure count where applicable
- min/max/avg RTT where applicable

### 14.3 Log Format

Default log format:

```text
text
```

Optional future or implemented format:

```text
json
```

Suggested option:

```text
--log-format text|json
```

Default:

```text
--log-format text
```

### 14.4 Text Log Example

```text
2026-08-16T11:00:35+01:00 host=192.168.1.23 status=up duration_seconds=2126 min_ms=1.70 max_ms=5.90 avg_ms=2.70 fails=0
2026-08-16T11:05:23+01:00 host=192.168.1.23 status=down duration_seconds=65 fails=23
2026-08-16T13:34:11+01:00 host=192.168.1.23 status=up duration_seconds=175226 min_ms=1.00 max_ms=2.50 avg_ms=1.70 fails=0
```

### 14.5 JSON Log Example

```json
{"time":"2026-08-16T11:00:35+01:00","host":"192.168.1.23","status":"up","duration_seconds":2126,"min_ms":1.70,"max_ms":5.90,"avg_ms":2.70,"fails":0}
{"time":"2026-08-16T11:05:23+01:00","host":"192.168.1.23","status":"down","duration_seconds":65,"fails":23}
{"time":"2026-08-16T13:34:11+01:00","host":"192.168.1.23","status":"up","duration_seconds":175226,"min_ms":1.00,"max_ms":2.50,"avg_ms":1.70,"fails":0}
```

### 14.6 Logging Rules

The log file:

- must not contain ANSI colors
- must not contain cursor-control sequences
- must not rely on line overwriting
- must be flushed reliably
- must remain independent of quiet mode
- must fail cleanly if the file cannot be opened
- must bracket IPv6 hosts (`[::1]`) so host values remain unambiguous

## 15. Signals and Graceful Shutdown

`dohping` must handle termination gracefully.

### 15.1 Signals to Handle

At minimum:

- `SIGINT`
- `SIGTERM`
- OS interrupt equivalent

### 15.2 Graceful Shutdown Behavior

On receiving a termination signal, `dohping` should:

1. stop active probing
2. finalize the current display line
3. flush log output
4. restore terminal state if using window mode
5. optionally print a short summary
6. exit with a predictable exit code

### 15.3 Summary on Exit

A short summary may be printed on graceful shutdown.

Example:

```text
--- dohping summary ---
host:            192.168.1.23
current status:  up
run duration:    02:14:09
total probes:    1523
successful:      1490
failed:          33
loss:            2.17%
```

Summary output should be suppressed in quiet mode.

### 15.4 Interactive Quit

When stdin is a terminal, pressing `q` (or `Q`) triggers the same graceful shutdown path as `SIGINT`/`SIGTERM`: stop probing, finalize the current display line, flush the log, restore terminal state, print the summary (unless quiet), and exit with code `0`.

- `q` works in both plain line mode and window mode.
- Exit code `0` — a deliberate user quit is normal completion, like `--count`.
- Key reading runs in a separate goroutine and never blocks the probe loop.
- With piped/redirected stdin there is nothing to press; no key handling occurs.

## 16. Command-Line Options

Proposed CLI surface:

```text
dohping [options] HOST
```

### 16.1 Basic Options

| Option | Description | Default |
|---|---|---:|
| `-h`, `--help` | Show help |  |
| `-V`, `--version` | Show version |  |
| `-i`, `--interval DURATION` | Probe interval | `1s` |
| `-t`, `--timeout DURATION` | Probe timeout | `2s` |
| `-c`, `--count N` | Stop after N probes | unlimited |
| `-p`, `--probe TYPE` | Probe type: `icmp` or `tcp[:PORT]` (default port `443`) | `icmp` |
| `-d`, `--down-after N` | Consecutive failures required to mark down | `1` |
| `-u`, `--up-after N` | Consecutive successes required to mark up | `1` |

### 16.2 Display Options

| Option | Description | Default |
|---|---|---|
| `-q`, `--quiet` | Suppress normal display output | off |
| `--no-header` | Do not display header | off |
| `--no-color` | Disable color output | off |
| `--color MODE` | Color mode: `auto`, `always`, or `never` | `auto` |
| `--live MODE` | Live update mode: `auto`, `on`, or `off` | `auto` |
| `--no-live` | Disable live updating |  |
| `-w`, `--window` | Use fixed auto-scrolling window mode | off |
| `--no-window` | Disable window mode (conflicts with `--window-lines`) | off |
| `--window-lines N` | Number of visible lines in window mode (implies `--window`) | `10` |
| `--timestamp-format FORMAT` | Display timestamp format | `HH:MM:SS` |

### 16.3 Logging Options

| Option | Description | Default |
|---|---|---|
| `-l`, `--log-file PATH` | Append events to log file | none |
| `--log-format FORMAT` | Log format: `text` or `json` | `text` |

### 16.4 Flag Conflict Policy

Mutually opposed explicit flags are a usage error: a clear message identifying the conflicting flags and exit code `2`, never silent ambiguity.

Current conflicts:

- `--no-window` with `--window-lines`
- `--no-color` with `--color=always`
- `--no-live` with `--live=on`

Non-conflicts by design:

- `--window` with `--window-lines` (compatible; `--window-lines` implies `--window`)
- `--quiet` with `--window` (quiet wins; documented)
- `NO_COLOR` with `--color=always` (environment overrides flag; not an error)

## 17. Display Mode Selection Rules

Recommended precedence:

1. If `--quiet` is active, no normal display is shown.
2. If `--window` is active and stdout is a terminal, use window mode.
3. If `--window` is active and stdout is not a terminal, fall back to plain line mode and warn.
4. Otherwise use plain line mode.
5. In plain line mode, live updating is enabled automatically when stdout is a terminal unless disabled.
6. In non-interactive output, live updating is disabled automatically unless explicitly enabled.

## 18. Exit Codes

Recommended exit codes:

| Code | Meaning |
|---:|---|
| `0` | Normal completion (`--count` exhausted, or interactive `q` quit) |
| `1` | General error |
| `2` | Usage or configuration error |
| `3` | Probe initialization or permission error |
| `130` | Terminated by interrupt / Ctrl-C |
| `143` | Terminated by SIGTERM |

Implementations may simplify, but exit behavior must be documented.

Note: `130` and `143` follow the Unix `128 + signal` convention and apply on Unix-like systems. On Windows, signal-based codes do not map; the closest conventional values are used and documented per platform.

## 19. Error Handling

`dohping` should fail clearly for invalid usage and operational problems.

Examples of errors requiring clear messages:

- missing host argument
- invalid interval
- invalid timeout
- unable to open log file
- DNS resolution failure
- ICMP socket permission denied
- unsupported platform
- terminal initialization failure

Permission errors should include helpful guidance where appropriate.

Example:

```text
error: unable to create ICMP socket: permission denied
hint: run with elevated privileges or grant CAP_NET_RAW
```

Errors should not cause panics in normal operation.

## 20. Engineering Requirements

### 20.1 Language

`dohping` should be written in Go.

Use a currently supported stable Go release.

### 20.2 Maintainability

The codebase should be modular and readable.

Recommended separation:

```text
cmd/dohping
internal/app
internal/cli
internal/ping
internal/state
internal/output
internal/theme
internal/logx
internal/signalx
internal/version
```

### 20.3 Concurrency

The implementation should use:

- `context.Context`
- cancellable probe loops
- clean shutdown paths
- a single stdin key-reader goroutine (interactive `q` quit) feeding the same shutdown path as signals
- synchronized shared state or event channels

Avoid:

- global mutable state
- unsynchronized access to statistics
- goroutine leaks
- uncontrolled timers

### 20.4 Time Handling

Use monotonic time for duration calculations where possible.

Use wall-clock time for displayed timestamps.

Handle system sleep or clock changes as gracefully as possible.

### 20.5 Dependencies

Dependencies should be minimal and justifiable.

Possible dependency areas:

- ICMP probing
- terminal detection
- terminal size detection
- CLI flag parsing
- color output

Any dependency should be actively maintained.

### 20.6 D.R.Y.

Shared logic should be centralized, especially:

- duration formatting
- RTT formatting
- column layout
- color theme
- status state transitions
- output field rendering

However, clarity is preferred over forced abstraction.

## 21. Testing Requirements

The implementation should include tests for:

### 21.1 State Logic

- up/down transitions
- unknown state
- error state
- consecutive success/failure thresholds
- state duration calculation
- statistics reset on state change

### 21.2 Statistics

- min/max/avg calculation
- zero-sample handling
- single-sample handling
- timeout handling
- division-by-zero safety

### 21.3 Display Formatting

- header visibility
- quiet mode
- no-header mode
- color disabling
- NO_COLOR behavior
- live update enable/disable
- window line limits
- non-TTY fallback
- host column width with long hostnames and IPv6 literals

### 21.4 Logging

- log file creation
- append behavior
- absence of ANSI codes
- JSON/text formatting
- flush on shutdown

### 21.5 Signal Handling

- graceful shutdown on interrupt
- graceful shutdown on SIGTERM
- log flushing before shutdown
- display finalization before shutdown
- interactive `q` quit (TTY only, exit `0`)

## 22. Professional UI Requirements

The terminal experience should feel stable and polished.

Required or recommended:

- fixed-width columns
- no jitter during live updates
- no flicker
- no ANSI output in logs
- no ANSI output when piped
- clean handling of terminal resize
- clean restoration of terminal state
- clear fallback when terminal features are unavailable
- readable default colors
- accessible text labels, not color-only meaning

## 23. Example Behaviors

### 23.1 Default Interactive Use

```sh
dohping 192.168.1.23
```

Expected:

- plain line mode
- header shown
- color enabled if supported
- last line live-updated
- terminal scrollback preserves previous lines
- pressing `q` quits cleanly (exit `0`)

### 23.2 Window Mode

```sh
dohping --window-lines 8 192.168.1.23
```

Expected:

- fixed window display
- latest 8 visible status lines retained
- no user scrollback required
- live current line updated
- terminal state restored on exit

### 23.3 Quiet Logging

```sh
dohping --quiet --log-file dohping.log 192.168.1.23
```

Expected:

- no normal display output
- events appended to `dohping.log`
- no colors in log file
- graceful shutdown still flushes log file

### 23.4 Scripted Output

```sh
dohping --no-color --no-header 192.168.1.23
```

Expected:

- no color
- no header
- plain output
- live updating disabled automatically if piped

## 24. Summary of Display Semantics

Plain line mode:

```text
Default display mode.
Shows history in normal terminal scrollback.
The current line live-updates until status changes.
When status changes, the current line becomes final.
```

Window mode:

```text
Optional display mode.
Shows only a fixed number of the latest lines.
Old lines are removed from the visible window.
Not user-scrollable.
Intended as a compact dashboard.
Log file should be used for full history.
```

## 25. Final Recommendation

The recommended default configuration is:

```text
default display mode:        plain line mode
default live updating:       enabled on TTY
default window mode:         disabled
default color:               auto
NO_COLOR:                    honored as override
default log format:          text
log file:                    append-only, no colors
signal handling:             graceful (SIGINT, SIGTERM, and interactive `q`)
window mode:                 selected explicitly with --window
```

This provides a safe, professional default experience while preserving the optional fixed auto-scrolling window as a dedicated dashboard mode.
