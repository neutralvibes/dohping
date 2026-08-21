package cli

import (
	"fmt"
	"io"
)

// WriteHelp prints the full help text. It is also the usage text shown
// after usage errors (the caller prints the error line first).
func WriteHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `dohping — a better interactive ping

Usage:
  dohping [options] HOST

Flags may appear before or after HOST (e.g. `+"`dohping HOST -c 5`"+`).

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

Signals and quit:
  SIGINT and SIGTERM trigger a graceful shutdown (finalize the current
  line, flush the log). In an interactive terminal, pressing q quits
  cleanly with exit code 0.

Color policy:
  Color is disabled when stdout is not a terminal, TERM=dumb, --no-color
  or --color=never is given. NO_COLOR is honored as an override: if it is
  set and non-empty, color stays off even with --color=always.
`)
}
