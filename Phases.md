

# dohping Implementation Phases

I would suggest **6 implementation phases**, plus optional polish work after the main specification is complete.

The recommended order is:

1. Project foundation and CLI contract
2. Probe engine and state engine
3. Plain line display and color support
4. Logging, quiet mode, signals, and shutdown behavior
5. Fixed auto-scrolling window mode
6. Production hardening, documentation, and release

This order reduces risk. The state engine should be stable before display work, plain display should be stable before window mode, and window mode should be built on top of reliable logging and signal handling.

---

## Phase 1 — Project Foundation and CLI Contract

### Goal

Create the basic Go project structure, configuration parsing, validation, and command-line interface.

This phase proves that the tool can parse options cleanly, reject invalid input, and provide a stable foundation for later features.

### Deliverables

- Basic Go module and project layout
- `dohping [options] HOST` command form
- CLI flag parsing
- Help output
- Version output
- Configuration struct
- Input validation
- Exit codes for usage errors
- Basic logging of diagnostic messages to stderr

Suggested project layout:

    cmd/dohping
    internal/app
    internal/cli
    internal/version

### Options to define in this phase

The flags can exist before all features are fully implemented, but their parsing and validation should be stable.

Important flags (short forms in parentheses):

    --help (-h)
    --version (-V)
    --interval (-i)
    --timeout (-t)
    --count (-c)
    --quiet (-q)
    --no-header
    --no-color
    --color
    --live
    --no-live
    --log-file (-l)
    --log-format
    --probe (-p)
    --window (-w)
    --no-window
    --window-lines
    --down-after (-d)
    --up-after (-u)
    --timestamp-format

### Tests for success

Phase 1 is complete when the following are true.

#### 1. Help works

Command:

    dohping --help

Expected:

- Usage is displayed.
- All major options are listed.
- Default values are visible where useful.
- The help text explains that `--window-lines` implies `--window`.

#### 2. Version works

Command:

    dohping --version

Expected:

- Tool name is shown.
- Version, commit, or build information is shown.
- Exit code is 0.
- `dohping -V` behaves identically.

#### 3. Missing host fails cleanly

Command:

    dohping

Expected:

- Usage error is shown.
- Exit code is 2.
- No panic occurs.

#### 4. Invalid values fail cleanly

Examples:

    dohping --interval nonsense 192.168.1.23
    dohping --timeout -1s 192.168.1.23
    dohping --window-lines 0 192.168.1.23
    dohping --color bogus 192.168.1.23

Expected:

- Clear error message.
- Exit code 2.
- No partial monitoring starts.

#### 5. Conflicting options are detected

Examples:

    dohping --no-window --window-lines 5 192.168.1.23
    dohping --no-color --color=always 192.168.1.23
    dohping --no-live --live=on 192.168.1.23

Expected:

- Clear usage error message identifying the conflicting flags.
- Exit code 2.
- No silent ambiguity, no partial monitoring starts.

#### 6. Automated checks pass

The following should pass:

    go build ./...
    go test ./...
    go vet ./...
    gofmt -l .
    golangci-lint run

#### Phase 1 success summary

| Area | Success condition |
|---|---|
| CLI parsing | Flags parse correctly |
| Help | Clear and complete |
| Validation | Invalid input rejected |
| Exit codes | Usage errors return predictable code |
| Project structure | Maintainable layout exists |
| Tooling | Build, lint, and tests pass |

---

## Phase 2 — Probe Engine and State Engine

### Goal

Build the core monitoring logic without depending on terminal UI.

This phase should produce a reliable state machine:

    probe result -> state engine -> status events

The state engine should be fully testable with a fake pinger and fake clock.

### Deliverables

- Probe interface
- Fake/mock pinger for tests
- Real TCP probe implementation (testable against a loopback listener; no privileges required)
- Real ICMP probe implementation (ICMPv6 for IPv6 targets; permission errors reported, never treated as host-down)
- DNS resolved once at startup; probes dial the resolved address
- Context-aware probe loop
- State engine
- Status model:
  - `unknown`
  - `up`
  - `down`
  - `error`
- State duration tracking
- RTT statistics:
  - min
  - max
  - avg
- Consecutive success/failure tracking
- Hysteresis support:
  - `--down-after`
  - `--up-after`
- Event emission for state changes and statistics updates

### Important design rule

The state engine should not know about terminal colors, window mode, or log files.

It should only emit semantic events such as:

    status changed
    probe succeeded
    probe failed
    statistics updated
    error occurred

Display and logging layers consume these events.

### Tests for success

Phase 2 is complete when the state engine can be tested deterministically without real network access.

#### 1. First successful probe becomes up

Given:

    state = unknown
    probe result = success

Expected:

    status = up

#### 2. First failed probe becomes down

Given:

    state = unknown
    probe result = failure

Expected:

    status = down

#### 3. Down threshold works

Given:

    --down-after 3

Probe sequence:

    fail
    fail
    success
    fail
    fail
    fail

Expected:

- Host does not become down after the first two failures.
- Successful probe resets failure progression.
- Host becomes down after three consecutive failures.

#### 4. Up threshold works

Given:

    --up-after 2

Probe sequence while down:

    success
    fail
    success
    success

Expected:

- Host does not become up after the first success.
- Failure resets success progression.
- Host becomes up after two consecutive successes.

#### 5. RTT statistics are correct

Given probe results:

    1.0 ms
    2.0 ms
    3.0 ms

Expected:

    min = 1.0
    max = 3.0
    avg = 2.0

#### 6. RTT statistics reset on status change

Given:

    status = up
    min/max/avg have existing values

When:

    status changes to down
    later changes back to up

Expected:

- Previous up-state RTT stats do not leak into the new up state.
- New state starts with clean statistics.

#### 7. Failure count increments while down

Given:

    status = down

Each failed probe should increase `FAILS`.

#### 8. Duration is tracked per state

Given:

    status changes to up at T0

Expected:

    duration = now - T0

When:

    status changes to down

Expected:

- Previous up duration is finalized.
- New down duration starts from the transition time.

#### 9. Error state is handled

Examples:

- DNS failure
- socket creation failure
- permission denied
- network unreachable

Expected:

- State engine emits an error event.
- Tool does not panic.
- Error is distinguishable from ordinary host-down state.

#### 10. Race detector passes

Run:

    go test -race ./...

Expected:

- No data races.
- No goroutine leaks.
- Context cancellation stops probe loop cleanly.

#### 11. TCP probe semantics are correct

Given a probe target that refuses connections (e.g. a closed loopback port):

- `connection refused` is treated as `up` (host alive, port closed).
- A dial timeout is treated as `down`.
- DNS failure / network unreachable produce an `error` event, not `down`.

Expected:

- TCP probing requires no elevated privileges.
- Deterministic tests use a `net.Listen` loopback listener.
- RTT reflects the TCP handshake time.

#### Phase 2 success summary

| Area | Success condition |
|---|---|
| Probe abstraction | Fake pinger can replace real pinger |
| State transitions | Deterministic and tested |
| Thresholds | `--down-after` and `--up-after` work |
| RTT stats | Min/max/avg are correct |
| Duration | Per-state duration is correct |
| Errors | Operational errors are explicit |
| Concurrency | Race detector passes |

---

## Phase 3 — Plain Line Display and Color Support

### Goal

Implement the default display mode.

This is the plain line mode where:

- history appears in normal terminal scrollback
- the current line is live-updated until status changes
- finalized lines are immutable
- color is optional and standards-aware

This phase should make the tool usable interactively.

### Deliverables

- Header rendering
- Column alignment
- Timestamp rendering
- Duration formatting
- RTT formatting
- Failure count formatting
- Live-updating current line
- Finalization on status change
- `--no-header`
- `--quiet`
- `--live auto|on|off`
- `--no-live`
- `--no-color`
- `--color auto|always|never`
- `NO_COLOR` support
- Non-TTY fallback behavior

### Recommended default behavior

For plain line mode:

    default mode = plain line mode
    live updates = auto
    color = auto

Live updates should be enabled when stdout is a terminal.

Live updates should be disabled when stdout is piped or redirected, unless explicitly forced.

### Tests for success

Phase 3 is complete when plain line mode is stable in both interactive and non-interactive environments.

#### 1. Header appears by default

Command:

    dohping 192.168.1.23

Expected header:

    TIME      HOST            STATUS  DURATION       MIN     MAX     AVG     FAILS

#### 2. `--no-header` suppresses header

Command:

    dohping --no-header 192.168.1.23

Expected:

- No header line.
- Data lines still displayed.

#### 3. Columns are stable

Expected:

- Columns remain aligned.
- RTT values use consistent decimal formatting.
- Duration formatting is consistent.
- Blank fields are used where appropriate.

Example up line:

    11:00:35  192.168.1.23    up      0d 00:35:26    1.70    5.90    2.70

Example down line:

    11:05:23  192.168.1.23    down    0d 00:01:05                                23

#### 4. Current line live-updates in a TTY

Given an interactive terminal:

Expected:

- The latest line updates in place.
- `TIME` remains the state start time.
- `DURATION` updates.
- RTT fields update when up.
- `FAILS` updates when down.

#### 5. Status change finalizes line

Given:

    current status = up

When:

    status changes to down

Expected:

- The previous up line is finalized.
- A new down line begins.
- The finalized line no longer changes.

#### 6. Non-TTY output disables live updates automatically

Command:

    dohping 192.168.1.23 | cat

Expected:

- No carriage-return line overwriting.
- No ANSI cursor-control sequences.
- Finalized status lines are emitted.

#### 7. `--no-live` disables live updates

Command:

    dohping --no-live 192.168.1.23

Expected:

- No live line overwriting.
- Output is appended only when state is finalized.

#### 8. `--quiet` suppresses display

Command:

    dohping --quiet 192.168.1.23

Expected:

- No normal stdout display.
- Fatal errors still go to stderr.
- Logging still occurs if `--log-file` is enabled.

#### 9. Color appears only when appropriate

Color should be disabled when any of these are true:

    --no-color is used
    --color=never is used
    NO_COLOR is set and non-empty
    stdout is not a terminal
    TERM=dumb

Tests:

    NO_COLOR=1 dohping 192.168.1.23
    dohping --no-color 192.168.1.23
    dohping --color=never 192.168.1.23
    dohping 192.168.1.23 | cat

Expected:

- No ANSI color codes in output.

#### 10. `NO_COLOR` overrides forced color

Command:

    NO_COLOR=1 dohping --color=always 192.168.1.23

Expected:

- Color remains disabled.

#### 11. Golden output tests pass

Use golden tests for:

- header output
- no-header output
- up line formatting
- down line formatting
- error line formatting
- color-disabled output
- non-TTY output
- host column width with long hostnames and IPv6 literals (width computed at startup, never changes mid-run)
- `99d+` duration overflow guard

#### Phase 3 success summary

| Area | Success condition |
|---|---|
| Plain line mode | Default display works |
| Live updating | Current line updates in TTY |
| Finalization | Old lines stop changing after status change |
| Quiet | `--quiet` suppresses display |
| No header | `--no-header` works |
| Non-TTY | No ANSI/cursor output when piped |
| Color | Color rules are correct |
| Formatting | Stable, readable columns |

---

## Phase 4 — Logging, Signals, and Graceful Shutdown

### Goal

Make `dohping` safe for long-running use and useful for scripted/automated environments.

This phase adds durable logging and graceful termination behavior.

### Deliverables

- `--log-file`
- `--log-format text|json`
- append-only log writing
- no ANSI colors in log output
- no cursor-control sequences in log output
- graceful handling of:
  - Ctrl-C
  - `SIGINT`
  - `SIGTERM`
  - interactive `q` quit (TTY sessions; same shutdown path, exit 0)
- normal completion via `--count` (exit 0)
- finalization of current display line on exit
- log flushing before exit
- predictable exit codes
- optional exit summary

### Recommended log semantics

The log file should record finalized events.

It should not attempt to mirror live line overwriting.

Display:

    live and visual

Log file:

    append-only and durable

### Tests for success

Phase 4 is complete when logging and shutdown behavior are reliable.

#### 1. Log file is created

Command:

    dohping --log-file dohping.log 192.168.1.23

Expected:

- File is created if missing.
- Events are appended.
- No ANSI codes are present.

#### 2. Log file append behavior works

Run:

    dohping --log-file dohping.log 192.168.1.23

Stop, then run again.

Expected:

- New events are appended.
- Old events are not truncated or overwritten.

#### 3. Text log format is stable

Example expected line:

    2026-08-16T11:00:35+01:00 host=192.168.1.23 status=up duration_seconds=2126 min_ms=1.70 max_ms=5.90 avg_ms=2.70 fails=0

Expected fields:

    timestamp
    host
    status
    duration_seconds
    min_ms where applicable
    max_ms where applicable
    avg_ms where applicable
    fails where applicable

#### 4. JSON log format is parseable

Command:

    dohping --log-file dohping.log --log-format json 192.168.1.23

Expected:

- Each line is valid JSON.
- Fields are consistent.
- Down events omit or nullify RTT fields as documented.

Example:

    {"time":"2026-08-16T11:00:35+01:00","host":"192.168.1.23","status":"up","duration_seconds":2126,"min_ms":1.70,"max_ms":5.90,"avg_ms":2.70,"fails":0}

#### 5. Quiet mode still logs

Command:

    dohping --quiet --log-file dohping.log 192.168.1.23

Expected:

- No normal stdout display.
- Log file still receives events.

#### 6. Log file contains no terminal control sequences

Check log file for:

- ANSI color codes
- carriage returns used for live updating
- cursor movement sequences

Expected:

- None are present.

#### 7. Ctrl-C is handled gracefully

Run:

    dohping 192.168.1.23

Then press Ctrl-C.

Expected:

- Current line is finalized.
- Terminal state is clean.
- Log file is flushed if enabled.
- No panic.
- Predictable exit code, preferably 130.

#### 8. SIGTERM is handled gracefully

Run:

    dohping 192.168.1.23 &
    kill $!

Expected:

- Tool stops cleanly.
- Current line is finalized.
- Log file is flushed if enabled.
- Predictable exit code, preferably 143.

#### 9. Exit summary behaves correctly

If summary output is implemented:

Expected on graceful exit:

- Summary is printed.
- Summary is suppressed when `--quiet` is active.
- Summary does not corrupt log files.

#### 10. Log file errors fail cleanly

Examples:

    dohping --log-file /nonexistent/path/file.log host
    dohping --log-file /root/forbidden.log host

Expected:

- Clear error message.
- Non-zero exit code.
- No silent loss of logs.

#### 11. Interactive `q` quits cleanly

Given an interactive terminal:

Command:

    dohping 192.168.1.23

Then press `q`.

Expected:

- Probe loop stops.
- Current line is finalized.
- Log file is flushed if enabled.
- Exit code is 0.
- `Q` (uppercase) behaves identically.
- With piped stdin, no key handling occurs.

#### Phase 4 success summary

| Area | Success condition |
|---|---|
| Log creation | File is created/appended |
| Log format | Text/JSON output is stable |
| Quiet | Quiet suppresses display but not logs |
| No ANSI in logs | Log file is clean |
| SIGINT | Ctrl-C exits gracefully |
| SIGTERM | Termination exits gracefully |
| Exit codes | Predictable |
| Flushing | Logs are flushed before exit |

---

## Phase 5 — Fixed Auto-Scrolling Window Mode

### Goal

Implement the optional fixed-size, auto-scrolling, non-scrollable window mode.

This phase should reuse the same state engine and event stream as plain line mode.

Window mode should not duplicate state logic.

### Deliverables

- `--window`
- `--window-lines N`
- fixed visible history
- live current line inside window
- rolling window behavior
- terminal redraw logic
- terminal resize handling
- fallback when stdout is not a terminal
- terminal state restoration on exit
- help text showing default window line count

### Recommended semantics

    --window
        Enables window mode using the default line count.

    --window-lines N
        Sets visible line count and implies --window.

Recommended default:

    --window-lines default = 10

### Tests for success

Phase 5 is complete when window mode is stable, bounded, and visually clean.

#### 1. `--window` enables window mode

Command:

    dohping --window 192.168.1.23

Expected:

- Fixed window display is used.
- Default visible line count is used.
- Current line live-updates.

#### 2. `--window-lines` implies window mode

Command:

    dohping --window-lines 5 192.168.1.23

Expected:

- Window mode is enabled automatically.
- Visible line count is 5.

#### 3. Help shows default window line count

Expected help text should clearly show something like:

    --window-lines N    Number of visible lines in window mode.
                        Implies --window. Default: 10.

#### 4. Window keeps only latest lines

Given:

    --window-lines 5

When more than 5 status lines exist:

Expected:

- Only the latest visible lines remain.
- Oldest visible line is removed.
- Display does not grow indefinitely.

#### 5. Current line still live-updates

Within window mode:

Expected:

- The active current line updates duration and stats.
- Finalized lines remain unchanged while visible.
- The window shifts when a status change occurs.

#### 6. Window mode does not rely on scrollback

Expected:

- The user cannot scroll back through full history using terminal scrollback.
- The visible history is intentionally limited.
- Full history is only preserved if logging is enabled.

#### 7. Non-TTY fallback works

Command:

    dohping --window 192.168.1.23 | cat

Expected:

- Window mode is not used.
- Tool falls back to plain line mode.
- Diagnostic warning is printed to stderr unless quiet mode is active.

#### 8. Conflicting options are handled

Command:

    dohping --no-window --window-lines 5 192.168.1.23

Expected:

- Usage error, or clearly documented behavior.
- No ambiguous state.

Recommended behavior:

    error: --window-lines requires window mode; remove --no-window or use --window

#### 9. Terminal resize is handled

During window mode, resize the terminal.

Expected:

- Display remains readable.
- Window reduces visible lines if needed.
- Or tool falls back safely.
- No corrupted screen state.

#### 10. Terminal state is restored on exit

After exiting window mode with Ctrl-C or SIGTERM:

Expected:

- Terminal is usable.
- Cursor is visible.
- Alternate screen, if used, is exited cleanly.
- No leftover rendering artifacts.

#### 11. Quiet mode suppresses window display

Command:

    dohping --quiet --window 192.168.1.23

Expected:

- No window is displayed.
- Logging still occurs if enabled.

#### 12. Window mode passes PTY-based tests

If possible, use pseudo-terminal tests to verify:

- window height limit
- live updates
- line removal
- fallback behavior
- absence of unexpected scrollback growth

#### 13. Interactive `q` works in window mode

Command:

    dohping --window 192.168.1.23

Press `q`.

Expected:

- Window mode exits cleanly via the same shutdown path.
- Terminal is restored.
- Exit code is 0.

#### Phase 5 success summary

| Area | Success condition |
|---|---|
| Enablement | `--window` works |
| Inference | `--window-lines` implies `--window` |
| Default | Default line count is visible and sensible |
| Rolling view | Only latest N lines remain visible |
| Live line | Current line updates correctly |
| Non-TTY | Falls back safely |
| Resize | Handles terminal resize |
| Exit | Restores terminal cleanly |
| Quiet | Suppresses window display |

---

## Phase 6 — Production Hardening, Documentation, and Release

### Goal

Make `dohping` robust, portable, and safe for real use.

This phase is about turning a working tool into a production-quality tool.

### Deliverables

- Real-probe verification (ICMP and TCP) against live hosts
- Permission error handling
- Clear operational error messages
- Cross-platform support where practical
- End-to-end tests
- Soak testing
- Documentation
- Release process
- CI pipeline
- Security and dependency checks

### Tests for success

Phase 6 is complete when the tool behaves predictably under real-world conditions.

#### 1. ICMP permission errors are clear

If run without permission:

Expected:

- Clear error message.
- Helpful hint.
- Non-zero exit code.
- No confusing “host down” representation for a local permission problem.

Example message style:

    error: unable to create ICMP socket: permission denied
    hint: run with elevated privileges or grant CAP_NET_RAW

#### 2. Real host monitoring works

Command:

    dohping 127.0.0.1

Expected:

- Host becomes up if ICMP is available.
- RTT stats are displayed.
- Duration increases.
- Ctrl-C exits cleanly.
- `dohping ::1` works over ICMPv6 where supported.
- `dohping --probe tcp 127.0.0.1` works without elevated privileges.

#### 3. Down behavior works

Use a host or controlled fake pinger that becomes unreachable.

Expected:

- Status changes to down.
- Failure count increases.
- RTT fields become blank.
- Log records transition.

#### 4. Long-running soak test passes

Run for an extended period, for example:

    dohping --log-file dohping.log 192.168.1.23

Duration:

- 1 hour minimum (the tool targets short interactive sessions; extended monitoring is out of scope)

Expected:

- No memory leak.
- No goroutine leak.
- No display corruption.
- Log remains consistent.
- Duration fields remain correct.

#### 5. Cross-platform builds succeed

Target platforms may include:

- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64
- Windows amd64

Expected:

    go build

succeeds for supported targets.

Platform-specific behavior should be documented (including the Unix-only nature of exit codes `130`/`143`).

#### 6. Static analysis passes

Run:

    go vet ./...
    staticcheck ./...
    golangci-lint run
    gosec ./...
    govulncheck ./...

Expected:

- No unresolved high-severity issues.
- No lint regressions.

#### 7. Race tests pass

Run:

    go test -race ./...

Expected:

- No data races.

#### 8. Coverage is meaningful

The state engine and output formatting should have high test coverage.

Important areas to cover:

- state transitions
- duration calculation
- RTT statistics
- color disable rules
- CLI validation
- log formatting
- window line limits

#### 9. Documentation is complete

Documentation should include:

- installation
- usage examples
- flag reference
- display modes
- window mode behavior
- log format
- exit codes
- signal behavior
- permission requirements
- color policy
- NO_COLOR support

#### 10. Release is reproducible

Expected:

- Versioned build
- Tagged release
- Reproducible binaries or documented build process
- Checksums if distributing binaries

#### Phase 6 success summary

| Area | Success condition |
|---|---|
| ICMP | Real probing works or errors clearly |
| Permissions | Helpful guidance provided |
| Stability | Long-running test passes |
| Platforms | Supported platforms build and run |
| Lint | Static analysis passes |
| Race | No data races |
| Docs | User and developer docs complete |
| Release | Reproducible release process exists |

---

## Optional Phase 7 — Enhancements

After the core specification is stable, possible enhancements can be considered.

These should not block the initial release.

Possible enhancements:

- multiple hosts
- CSV output
- JSON display output
- custom themes
- user-defined color configuration
- summary bar in window mode
- packet loss percentage
- p50/p95/p99 RTT statistics
- warning/critical thresholds
- UDP/HTTP probe modes
- alternate screen mode toggle
- sound or desktop notification on status change
- metrics endpoint
- historical sparkline

Each enhancement should have its own phase or ticket.

---

## Overall Acceptance Test

The final tool should pass this manual acceptance flow.

### Test 1 — Default plain mode

Command:

    dohping 192.168.1.23

Expected:

- Header shown.
- Current line live-updates.
- Previous lines remain in terminal scrollback.
- Ctrl-C exits cleanly.

### Test 2 — Window mode

Command:

    dohping --window --window-lines 5 192.168.1.23

Expected:

- Window mode is enabled.
- Only latest visible lines remain.
- Current line live-updates.
- Old lines fall off the display.
- Ctrl-C exits cleanly and restores terminal.

### Test 3 — Window inference

Command:

    dohping --window-lines 5 192.168.1.23

Expected:

- Window mode is enabled automatically.
- Visible line count is 5.

### Test 4 — Quiet logging

Command:

    dohping --quiet --log-file dohping.log 192.168.1.23

Expected:

- No display output.
- Log file receives events.
- Ctrl-C exits cleanly and flushes log.

### Test 5 — Piped output

Command:

    dohping 192.168.1.23 | cat

Expected:

- No ANSI escape sequences.
- No live line overwriting.
- Plain finalized output.

### Test 6 — Color policy

Commands:

    NO_COLOR=1 dohping 192.168.1.23
    dohping --no-color 192.168.1.23
    dohping --color=never 192.168.1.23

Expected:

- No color output in any case.

---

## Recommended Completion Criteria

The project can be considered complete when:

1. Plain line mode works reliably.
2. Live updating works only where appropriate.
3. Window mode works as an explicit optional display mode.
4. `--window-lines` implies `--window`.
5. Logging is clean, append-only, and color-free.
6. Signals are handled gracefully.
7. Exit codes are predictable.
8. Color policy obeys `NO_COLOR`.
9. Tests cover state, formatting, logging, and shutdown.
10. Documentation explains behavior clearly.
