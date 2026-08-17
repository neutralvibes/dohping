// Package app wires the CLI contract to the run orchestration: probe
// construction, state engine, probe loop, display, logging, signals, and
// graceful shutdown.
package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"dohping/internal/cli"
	"dohping/internal/logx"
	"dohping/internal/output"
	"dohping/internal/ping"
	"dohping/internal/signalx"
	"dohping/internal/state"
	"dohping/internal/theme"
	"dohping/internal/version"
)

// Exit codes (spec §18).
const (
	ExitOK         = 0
	ExitError      = 1
	ExitUsage      = 2
	ExitProbeInit  = 3
	ExitInterrupt  = 130
	ExitTerminated = 143
)

// TTY describes the terminal state of the process, injected so tests can
// exercise both interactive and piped paths.
type TTY struct {
	Stdout    bool     // stdout is a terminal
	Stdin     bool     // stdin is a terminal
	StdinFile *os.File // stdin for the interactive q-quit reader (nil = none)
}

// stopReason classifies how a run ended.
type stopReason int

const (
	stopCount     stopReason = iota // --count exhausted: normal completion
	stopQuit                        // interactive q quit: normal completion
	stopInterrupt                   // SIGINT / Ctrl-C
	stopTerm                        // SIGTERM
	stopPerm                        // permission-class operational error: exit 3
)

func (r stopReason) exitCode() int {
	switch r {
	case stopInterrupt:
		return ExitInterrupt
	case stopTerm:
		return ExitTerminated
	default:
		return ExitOK
	}
}

// keyEvent is a key-reader outcome.
type keyEvent int

const (
	keyQuit  keyEvent = iota // q / Q pressed
	keyCtrlC                 // 0x03 in raw mode (Ctrl-C without ISIG)
	keyEOF                   // stdin closed
)

// cprEvent is a terminal cursor-position report (\x1b[<row>;<col>R), the
// response to a DSR query (\x1b[6n) — used to re-anchor the displays on
// reflowing terminals after a resize (DECISIONS #66). Rows/cols are
// 1-based per the VT spec.
type cprEvent struct {
	row, col int
}

// Main is the process entry point: parse args, dispatch, return exit code.
// Stdout/stderr/tty are injected so tests can capture output.
func Main(args []string, stdout, stderr io.Writer, tty TTY) int {
	opts, action, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "dohping: %v\n", err)
		fmt.Fprintf(stderr, "run 'dohping --help' for usage\n")
		return ExitUsage
	}

	switch action {
	case cli.ActionHelp:
		cli.WriteHelp(stdout)
		return ExitOK
	case cli.ActionVersion:
		fmt.Fprintln(stdout, version.String())
		return ExitOK
	}

	// Probe construction: operational errors (permission, DNS) exit 3
	// with guidance — never a host-down condition.
	pr, err := buildProbe(opts)
	if err != nil {
		fmt.Fprintf(stderr, "dohping: %v\n", err)
		if ping.IsPermissionError(err) {
			fmt.Fprintln(stderr, "hint: run with elevated privileges or grant CAP_NET_RAW (e.g. setcap cap_net_raw+ep on the binary)")
		}
		return ExitProbeInit
	}
	defer pr.Close()

	eng := state.New(opts.DownAfter, opts.UpAfter)

	// Log file: a failure to open is a clean error, never silent loss.
	var logger *logx.Logger
	if opts.LogFile != "" {
		logger, err = logx.Open(opts.LogFile, opts.LogFormat, opts.Host)
		if err != nil {
			fmt.Fprintf(stderr, "dohping: unable to open log file %q: %v\n", opts.LogFile, err)
			return ExitError
		}
		defer logger.Close()
	}

	colorEnabled := theme.Enabled(theme.Config{NoColor: opts.NoColor, ColorMode: opts.ColorMode},
		tty.Stdout, theme.Env{NO_COLOR: os.Getenv("NO_COLOR"), TERM: os.Getenv("TERM")})
	th := theme.NewRenderer(colorEnabled, theme.Default)
	layout := output.NewLayout(opts.Host, opts.TimestampFormat, th)

	live := !opts.NoLive && (opts.LiveMode == "on" || (opts.LiveMode == "auto" && tty.Stdout))

	// Interactive q-quit reader (raw stdin when a terminal). Started
	// BEFORE the displays are built: they capture the reanchor closure at
	// construction time, so the real cursor query must be wired first.
	keyCh := make(chan keyEvent, 1)
	var cprCh chan cprEvent
	reanchor := func() (int, bool) { return 0, false } // no-op unless interactive
	if tty.Stdin && tty.StdinFile != nil {
		cprCh = make(chan cprEvent, 1)
		restore, kerr := startKeyReader(tty.StdinFile, keyCh, cprCh)
		if kerr != nil {
			fmt.Fprintf(stderr, "dohping: warning: cannot configure interactive quit: %v\n", kerr)
		} else {
			defer restore()
			// Wire the real cursor query now that a reader is listening.
			reanchor = func() (int, bool) {
				// Drop any stale response from a previous timed-out query.
				select {
				case <-cprCh:
				default:
				}
				fmt.Fprint(stdout, "\x1b[6n")
				deadline := time.After(150 * time.Millisecond)
				select {
				case c := <-cprCh:
					if c.row > 0 && c.col > 0 {
						return c.row, true
					}
					return 0, false
				case <-deadline:
					return 0, false
				}
			}
		}
	} else {
		close(keyCh) // no key handling with piped stdin (spec §15.4)
	}

	// Display selection (spec §17): quiet suppresses all; window mode needs
	// a terminal (else fall back to plain mode with a warning); otherwise
	// plain line mode.
	var disp displayer
	var winchCh <-chan os.Signal
	windowActive := opts.Window && tty.Stdout
	if windowActive {
		wd := output.NewWindow(stdout, layout, opts.WindowLines, opts.Quiet, opts.NoHeader,
			defaultSizeFn(stdout), reanchor)
		wd.Enter()
		defer wd.Exit()
		disp = wd
		c, stop := signalx.Winch()
		defer stop()
		winchCh = c
	} else {
		if opts.Window && !opts.Quiet {
			fmt.Fprintln(stderr, "dohping: warning: --window requires a terminal; falling back to plain line mode")
		}
		disp = output.NewDisplay(stdout, layout, opts.Quiet, opts.NoHeader, live,
			defaultSizeFn(stdout), reanchor)
		if live {
			// Plain live mode gets the same SIGWINCH fast path as the
			// window: an immediate live-line repaint re-anchors the
			// wrapped line the moment the terminal changes (the 1-second
			// tick would otherwise cover it within a second).
			c, stop := signalx.Winch()
			defer stop()
			winchCh = c
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh, stopSig := signalx.Listen()
	defer stopSig()

	// The liveness animation advances on a fixed 1-second timer,
	// independent of probe cadence: with a long --interval the probe
	// events are rare, but the display must still visibly move every
	// second (user report 2026-08-17). Piped/quiet runs have nothing to
	// animate — the channel stays nil and the select case never fires.
	var tickCh <-chan time.Time
	if !opts.Quiet && (live || windowActive) {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		tickCh = tick.C
	}

	events := make(chan state.Event, 8)
	go func() {
		Run(ctx, pr, eng, opts.Interval, opts.Count, events)
		close(events)
	}()

	runStart := time.Now()
	reason := stopCount
	var permErr error
loop:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				break loop // Run finished: count exhausted or cancelled
			}
			disp.Handle(ev)
			if logger != nil {
				logEvent(logger, ev)
			}
			// A permission-class operational error (raw socket, ping
			// socket, or ping command denied) is permanent — abort with
			// guidance instead of probing in error state forever.
			if (ev.Kind == state.EventError || ev.Kind == state.EventProbeError) && ev.Err != nil && ping.IsPermissionError(ev.Err) {
				permErr = ev.Err
				reason = stopPerm
				cancel()
			}
		case sig := <-sigCh:
			if sig == syscall.SIGTERM {
				reason = stopTerm
			} else {
				reason = stopInterrupt
			}
			cancel()
		case k, ok := <-keyCh:
			if !ok {
				keyCh = nil // reader gone; never select on it again
				continue
			}
			switch k {
			case keyQuit:
				reason = stopQuit
				cancel()
			case keyCtrlC:
				reason = stopInterrupt
				cancel()
			case keyEOF:
				keyCh = nil
			}
		case <-winchCh:
			// Immediate repaint on terminal resize (Unix SIGWINCH fast
			// path). Tick is the right repaint for both displays: Window
			// redraws the block, Display re-anchors the live line. On
			// Windows the channel never fires — the 1-second tick covers
			// resizes there (platform-split, DECISIONS #64/#65).
			disp.Tick()
		case <-tickCh:
			disp.Tick()
		}
	}

	// Graceful shutdown: finalize the current display line, log the final
	// state, print the summary (interactive only), exit predictably.
	disp.Finalize()
	if logger != nil {
		logFinal(logger, opts.Host, eng)
	}
	if reason == stopPerm {
		// Permission problem: report with guidance, exit 3 — never a
		// host-down condition, never an endless error state.
		fmt.Fprintf(stderr, "dohping: %v\n", permErr)
		fmt.Fprintln(stderr, "hint: run with elevated privileges or grant CAP_NET_RAW (e.g. setcap cap_net_raw+ep on the binary); on some systems the ping command itself needs privileges")
		return ExitProbeInit
	}
	if !opts.Quiet && tty.Stdout {
		printSummary(stdout, opts.Host, eng, time.Since(runStart))
	}
	return reason.exitCode()
}

// displayer is the interface shared by plain and window displays.
type displayer interface {
	Handle(state.Event)
	Finalize()
	Tick()
}

// defaultSizeFn reads the terminal size (width, height) from an *os.File
// writer (bytes.Buffer in tests → (0, 0) = unknown → startup column
// policy and the configured window size). Re-measured on EVERY redraw, so
// a resize is picked up by probe events, the 1-second tick (the only
// mechanism Windows has — no SIGWINCH there), and the Unix SIGWINCH fast
// path alike.
func defaultSizeFn(w io.Writer) func() (int, int) {
	f, ok := w.(*os.File)
	if !ok {
		return func() (int, int) { return 0, 0 }
	}
	return func() (int, int) {
		width, height, err := term.GetSize(int(f.Fd()))
		if err != nil {
			return 0, 0
		}
		return width, height
	}
}

// startKeyReader puts stdin into raw mode and reads keys in a goroutine.
// q/Q quits (exit 0); 0x03 (Ctrl-C in raw mode, ISIG off) interrupts
// (exit 130). CSI sequences are parsed: a cursor-position report
// (\x1b[<row>;<col>R — the terminal's answer to a DSR \x1b[6n query) is
// delivered on cpr for the displays' reflow re-anchor (DECISIONS #66);
// other CSI sequences (arrow keys etc.) are ignored. The terminal is
// restored when the reader exits and by the returned restore function.
// With raw stdin, Ctrl-C no longer raises SIGINT — the byte is mapped
// here so the exit code contract holds.
func startKeyReader(f *os.File, out chan<- keyEvent, cpr chan<- cprEvent) (restore func(), err error) {
	oldState, err := term.MakeRaw(int(f.Fd()))
	if err != nil {
		return nil, err
	}
	restore = func() { _ = term.Restore(int(f.Fd()), oldState) }
	go func() {
		defer restore()
		r := bufio.NewReader(f)
		for {
			b, err := r.ReadByte()
			if err != nil {
				out <- keyEOF
				return
			}
			switch b {
			case 'q', 'Q':
				out <- keyQuit
				return
			case 0x03:
				out <- keyCtrlC
				return
			case 0x04: // Ctrl-D: EOF for the reader, terminal restored
				out <- keyEOF
				return
			case 0x1b:
				// CSI sequence — possibly a CPR response. Read the
				// introducer, then parameters up to the final byte.
				next, err := r.ReadByte()
				if err != nil {
					out <- keyEOF
					return
				}
				if next != '[' {
					continue // ESC + non-CSI (e.g. lone ESC): ignore
				}
				params := make([]byte, 0, 8)
				for {
					c, err := r.ReadByte()
					if err != nil {
						out <- keyEOF
						return
					}
					if c >= 0x40 && c <= 0x7e { // final byte
						if c == 'R' {
							if row, col, ok := parseCPR(params); ok {
								cpr <- cprEvent{row: row, col: col}
							}
						}
						break
					}
					params = append(params, c)
				}
			}
		}
	}()
	return restore, nil
}

// parseCPR parses \x1b[<row>;<col>R parameters (1-based VT coordinates).
func parseCPR(params []byte) (row, col int, ok bool) {
	parts := strings.Split(string(params), ";")
	if len(parts) != 2 {
		return 0, 0, false
	}
	r, err1 := strconv.Atoi(parts[0])
	c, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || r < 1 || c < 1 {
		return 0, 0, false
	}
	return r, c, true
}

// logEvent logs the state that just ended, if it is a real status period
// (up/down/error). The initial unknown→X transition has nothing to log.
func logEvent(l *logx.Logger, ev state.Event) {
	if ev.Kind != state.EventStatusChange && ev.Kind != state.EventError {
		return
	}
	if ev.PrevStatus != state.StatusUp && ev.PrevStatus != state.StatusDown && ev.PrevStatus != state.StatusError {
		return
	}
	_ = l.Log(logx.Entry{
		Time:     ev.Time,
		Status:   ev.PrevStatus,
		Duration: ev.Duration,
		Fails:    ev.Fails,
		Stats:    ev.PrevStats,
	})
}

// logFinal logs the current state at shutdown.
func logFinal(l *logx.Logger, host string, eng *state.Engine) {
	if eng.Status() == state.StatusUnknown {
		return // never started a real status
	}
	_ = l.Log(logx.Entry{
		Time:     time.Now(),
		Status:   eng.Status(),
		Duration: time.Since(eng.Start()),
		Fails:    eng.Fails(),
		Stats:    eng.Stats(),
	})
}

// printSummary renders the optional exit summary (spec §15.3), shown only
// on interactive terminals so scripted/piped output stays parseable.
//
// Every line is \r-prefixed AND \r\n-terminated: after Finalize the
// cursor may sit anywhere (live mode ends mid-line on terminals without
// ONLCR), so each line explicitly resets to column 0 before writing and
// lands at column 0 of the next line after (DECISIONS #54 lesson, user
// report 2026-08-17 — the summary previously drifted progressively
// right until the terminal wrapped).
func printSummary(w io.Writer, host string, eng *state.Engine, runDuration time.Duration) {
	probes, ok, fail := eng.Totals()
	loss := 0.0
	if probes > 0 {
		loss = float64(fail) / float64(probes) * 100
	}
	fmt.Fprintf(w, "\r--- dohping summary ---\r\n")
	fmt.Fprintf(w, "\r%-16s %s\r\n", "host:", host)
	fmt.Fprintf(w, "\r%-16s %s\r\n", "current status:", eng.Status())
	fmt.Fprintf(w, "\r%-16s %s\r\n", "run duration:", formatRunDuration(runDuration))
	fmt.Fprintf(w, "\r%-16s %d\r\n", "total probes:", probes)
	fmt.Fprintf(w, "\r%-16s %d\r\n", "successful:", ok)
	fmt.Fprintf(w, "\r%-16s %d\r\n", "failed:", fail)
	fmt.Fprintf(w, "\r%-16s %.2f%%\r\n", "loss:", loss)
}

func formatRunDuration(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// buildProbe constructs the configured probe type.
func buildProbe(opts *cli.Options) (ping.Probe, error) {
	switch opts.ProbeType {
	case cli.ProbeTCP:
		return ping.NewTCPProbe(opts.Host, opts.TCPPort, opts.Timeout)
	default:
		return ping.NewICMPProbe(opts.Host, opts.Timeout)
	}
}
