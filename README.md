# dohping

> *"Doh!"* Homer Simpson, every time `ping` scrolls him into oblivion.

**dohping** is a status-line based ping for network monitoring. It tells you if a host is up, for how long, and how fast, as one clean, live-updating line. When it flips to **down**, you see exactly when and for how long. No root required in some modes.

![dohping watching a flapping host](assets/dohping-demo.gif)

*One clean line per status. Above it watches a real host go up, down, up, and down again.*

## Why not just `ping`?

`ping` prints one line per packet. To find an outage you may have to grep through hundreds of identical lines. `dohping` prints one line per *status*: the lines you actually care about.

**Before:**

```text
$ ping 1.1.1.1
64 bytes from 1.1.1.1: icmp_seq=1 ttl=59 time=2.30 ms
64 bytes from 1.1.1.1: icmp_seq=2 ttl=59 time=2.10 ms
64 bytes from 1.1.1.1: icmp_seq=3 ttl=59 time=2.50 ms
... (scroll for ages to find the state change)
```

**After:**

```text
$ dohping 1.1.1.1
TIME      HOST       STATE DURATION       MIN     MAX     AVG     FAILS
11:00:35  1.1.1.1    up    0d 00:35:26    1.70    5.90    2.70
11:05:23  1.1.1.1    down  0d 00:01:05                                23
11:06:28  1.1.1.1    up    0d 00:00:01    1.60    1.60    1.60
```

Built for short interactive sessions, minutes to a few days, not for long-term monitoring.

## Quick start

```sh
# See it live
dohping 1.1.1.1

# Check a host 5 times, then exit cleanly (great for scripts)
dohping -c 5 example.com

# TCP mode, no root needed
dohping --probe tcp example.com

# Dashboard view
dohping --window example.com
```

Press `q` to quit cleanly. `Ctrl-C` works too.

## What makes it different

- **Stateful, not noisy**: one line per status, period. Read your scrollback like a log, not a haystack.
- **Two views**: plain line mode (default) or a fixed dashboard window.
- **No privileges required**: ICMP via raw socket, unprivileged ping socket, or system `ping` fallback. Use `--probe tcp` to skip ICMP entirely.
- **Resize-aware**: drag your terminal corner around while it runs. It repaints cleanly without fighting your terminal.
- **Scriptable**: predictable exit codes, optional JSON logging, and `--quiet` for clean output.
- **Cross-platform**: Linux, macOS, Windows (amd64 and arm64).

## Installation

### Release binaries

Pre-built binaries for Linux, macOS, and Windows are available from the [releases page](https://github.com/neutralvibes/dohping/releases).

### From source

Requires Go >= 1.26.

```sh
go build -o dohping ./cmd/dohping
```

### Build all targets

```sh
./scripts/release.sh dist
```

Artifacts land in `dist/` as `dohping-<os>-<arch>` plus `SHA256SUMS`.

### Running without sudo (Linux / Raspberry Pi)

Linux normally blocks unprivileged users from opening raw network sockets, which ICMP timing needs. If you run `dohping` as a normal user you may see a permission error. Rather than use `sudo` every time, you can grant the binary the single privilege it needs:

```sh
sudo mv dohping /usr/local/bin/
sudo setcap cap_net_raw=+ep /usr/local/bin/dohping
```

After that, any user can run `dohping` safely. `dohping` also falls back to the system `ping` command when it cannot open a socket directly, so on systems where `ping` works for your user (for example Raspberry Pi OS, which ships `ping` already privileged) `dohping` usually works with no setup at all. If ICMP is blocked entirely, `--probe tcp` needs no privileges.

## When to use it

- **"When is a host `up` or `down`?"**: run and watch the status change.
- **"Is the wifi flaky right now?"**: leave it running and glance at the duration column.
- **"Did that server just go down?"**: the exact timestamp and failure count are right there.
- **"Is latency getting worse?"**: watch the `AVG` column drift in real time.
- **"Script a health check"**: predictable exit codes, JSON logging, no TTY assumptions.

## Usage

```text
Usage:
  dohping [options] HOST

Options:
  -h, --help                 Show help and exit
  -V, --version              Show version and exit
  -i, --interval TIME        Probe interval (default 1s)
  -t, --timeout TIME         Probe timeout (default 2s)
  -c, --count N              Stop after N probes
  -p, --probe TYPE           Probe type: icmp | tcp[:PORT] (default icmp)
  -d, --down-after N         Failures before marking down (default 1)
  -u, --up-after N           Successes before marking up (default 1)

Display:
  -q, --quiet                Suppress display output
      --no-header            Skip the column header
      --no-color             Disable color output
      --color MODE           Color mode: auto, always, never
      --live MODE            Live updates: auto, on, off
      --no-live              Disable live updating
  -w, --window               Fixed dashboard window
      --no-window            Disable window mode
      --window-lines N       Window height (default 10; implies --window)
      --timestamp-format F   Display timestamp format: HH:MM:SS | rfc3339

Logging:
  -l, --log-file PATH        Append status events to a file
      --log-format FORMAT    Log format: csv | json (default csv)

Exit codes:
  0    Normal completion
  1    General error
  2    Usage or configuration error
  3    Probe initialization or permission error
  130  Interrupted (SIGINT / Ctrl-C)
  143  Terminated (SIGTERM)
```

## Examples

### Watch a host until you stop it

```sh
dohping 192.168.1.23
```

ICMP probe every second, plain line mode. The live line updates in place.

### Scripted check

```sh
dohping -c 5 example.com; echo "exit: $?"
```

Runs 5 probes, prints finalized lines and a summary, exits `0`.

### TCP probe without root

```sh
dohping --probe tcp example.com        # port 443
dohping --probe tcp:22 example.com     # explicit port
```

Connection established or refused means **up**; timeout means **down**; DNS failure means **error** (exit 3, never a false "down").

### Dashboard window

```sh
dohping --window-lines 8 example.com
```

A fixed, auto-scrolling dashboard drawn in place. Resize it while it runs and it repaints cleanly.

### Quiet logging

```sh
dohping -q -l events.log -i 5 example.com
```

No display, but every status change is appended to `events.log` (text or JSON), fsync'd, with `0600` permissions.

## Display modes

### Plain line mode (default)

Each status period is one line. The current line live-updates in place while the status is unchanged; when the status changes, the previous line is finalized into scrollback history and a new line begins.

```text
TIME      HOST            STATE DURATION       MIN     MAX     AVG     FAILS
11:00:35  192.168.1.23    up    0d 00:35:26    1.70    5.90    2.70
11:05:23  192.168.1.23    down  0d 00:01:05                                23
```

### Window mode

A fixed block of the most recent lines plus the current live line, drawn in place on the normal terminal. No alternate screen, no clearing. The block stays in place and handles resizing without leaving stale output behind.

> Resize handling works across Windows Terminal, Terminal.app, and iTerm2. See [docs/terminal-rendering.md](docs/terminal-rendering.md) if you want to know how it works.

## Logging

`--log-file PATH` appends one line per finalized status event. Logging is independent of `--quiet`.

**CSV:**

```text
2026-08-16T11:00:35+01:00,192.168.1.23,up,2126,1.70,5.90,2.70,0
2026-08-16T11:05:23+01:00,192.168.1.23,down,65,,,,23
```

**JSON:**

```json
{"time":"2026-08-16T11:00:35+01:00","host":"192.168.1.23","status":"up","duration_seconds":2126,"min_ms":1.7,"max_ms":5.9,"avg_ms":2.7,"fails":0}
```

## Development

```sh
bash scripts/check.sh          # Full quality gate: format, vet, race tests, linters
bash scripts/release.sh        # Reproducible cross-compiled binaries + checksums
```

CI runs the same checks on every push and pull request.

## License

MIT
