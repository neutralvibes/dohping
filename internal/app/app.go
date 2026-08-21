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
	"syscall"
	"time"

	"golang.org/x/term"

	"dohping/internal/cli"
	"dohping/internal/debugx"
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

// cprEvent was removed with the DSR/CPR re-anchor machinery: ConPTY
// reports unreliable cursor positions on resize, so the displays
// freeze-and-restart instead of querying the terminal.

// Main is the process entry point: parse args, dispatch, return exit code.
// Stdout/stderr/tty are injected so tests can capture output.
func Main(args []string, stdout, stderr io.Writer, tty TTY) int {
	// Optional diagnostic logger: DOHPING_DEBUG=<path> enables the
	// resize/redraw forensics file — the app's own record of the widths a
	// resize drag passes through (no terminal displays them). File-only:
	// the display owns the terminal, so debug output never goes there.
	debugx.Init()
	defer debugx.Close()

	opts, action, err := cli.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "dohping: %v\n", err)
		_, _ = fmt.Fprintf(stderr, "run 'dohping --help' for usage\n")
		return ExitUsage
	}

	switch action {
	case cli.ActionHelp:
		cli.WriteHelp(stdout)
		return ExitOK
	case cli.ActionVersion:
		_, _ = fmt.Fprintln(stdout, version.String())
		return ExitOK
	}

	// Probe construction: operational errors (permission, DNS) exit 3
	// with guidance — never a host-down condition.
	pr, err := buildProbe(opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "dohping: %v\n", err)
		if ping.IsPermissionError(err) {
			_, _ = fmt.Fprintln(stderr, "hint: run with elevated privileges or grant CAP_NET_RAW (e.g. setcap cap_net_raw+ep on the binary)")
		}
		return ExitProbeInit
	}
	defer func() { _ = pr.Close() }()

	eng := state.New(opts.DownAfter, opts.UpAfter)

	// Log file: a failure to open is a clean error, never silent loss.
	var logger *logx.Logger
	if opts.LogFile != "" {
		logger, err = logx.Open(opts.LogFile, opts.LogFormat, opts.Host)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "dohping: unable to open log file %q: %v\n", opts.LogFile, err)
			return ExitError
		}
		defer func() { _ = logger.Close() }()
	}

	colorEnabled := theme.Enabled(theme.Config{NoColor: opts.NoColor, ColorMode: opts.ColorMode},
		tty.Stdout, theme.Env{NO_COLOR: os.Getenv("NO_COLOR"), TERM: os.Getenv("TERM")})
	th := theme.NewRenderer(colorEnabled, theme.Default)
	layout := output.NewLayout(opts.Host, opts.TimestampFormat, th)

	live := !opts.NoLive && (opts.LiveMode == "on" || (opts.LiveMode == "auto" && tty.Stdout))

	// Interactive q-quit reader (raw stdin when a terminal).
	keyCh := make(chan keyEvent, 1)
	if tty.Stdin && tty.StdinFile != nil {
		restore, kerr := startKeyReader(tty.StdinFile, keyCh)
		if kerr != nil {
			_, _ = fmt.Fprintf(stderr, "dohping: warning: cannot configure interactive quit: %v\n", kerr)
		} else {
			defer restore()
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
			defaultSizeFn(stdout))
		wd.Enter()
		defer wd.Exit()
		disp = wd
		debugx.Debugf("display", "window mode active (lines=%d)", opts.WindowLines)
		c, stop := signalx.Winch()
		defer stop()
		winchCh = c
	} else {
		if opts.Window && !opts.Quiet {
			_, _ = fmt.Fprintln(stderr, "dohping: warning: --window requires a terminal; falling back to plain line mode")
		}
		disp = output.NewDisplay(stdout, layout, opts.Quiet, opts.NoHeader, live,
			defaultSizeFn(stdout))
		debugx.Debugf("display", "plain mode active (live=%v)", live)
		if live {
			// Plain live mode gets the same SIGWINCH fast path as the
			// window: an immediate live-line repaint notices a width
			// change the moment the terminal changes (the 1-second tick
			// would otherwise cover it within a second); the display then
			// freezes and restarts below once the width settles.
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
			// redraws the block, Display refreshes the live line — both
			// notice the width change and freeze/restart as needed. On
			// Windows the channel never fires — the 1-second tick covers
			// resizes there. Debug forensics: whether ConPTY → WSL2 even
			// delivers SIGWINCH is itself a fact the log must record.
			debugx.Debugf("winch", "SIGWINCH received → repaint")
			disp.Tick()
		case <-tickCh:
			debugx.Debugf("tick", "1s tick repaint")
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
		_, _ = fmt.Fprintf(stderr, "dohping: %v\n", permErr)
		_, _ = fmt.Fprintln(stderr, "hint: run with elevated privileges or grant CAP_NET_RAW (e.g. setcap cap_net_raw+ep on the binary); on some systems the ping command itself needs privileges")
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
// (exit 130). Other bytes (arrows, ESC, …) are consumed and ignored. The
// terminal is restored when the reader exits and by the returned restore
// function. With raw stdin, Ctrl-C no longer raises SIGINT — the byte is
// mapped here so the exit code contract holds.
func startKeyReader(f *os.File, out chan<- keyEvent) (restore func(), err error) {
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
			}
		}
	}()
	return restore, nil
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
// lands at column 0 of the next line after (the summary previously
// drifted progressively right until the terminal wrapped).
func printSummary(w io.Writer, host string, eng *state.Engine, runDuration time.Duration) {
	probes, ok, fail := eng.Totals()
	loss := 0.0
	if probes > 0 {
		loss = float64(fail) / float64(probes) * 100
	}
	_, _ = fmt.Fprintf(w, "\r--- dohping summary ---\r\n")
	_, _ = fmt.Fprintf(w, "\r%-16s %s\r\n", "host:", host)
	_, _ = fmt.Fprintf(w, "\r%-16s %s\r\n", "current status:", eng.Status())
	_, _ = fmt.Fprintf(w, "\r%-16s %s\r\n", "run duration:", formatRunDuration(runDuration))
	_, _ = fmt.Fprintf(w, "\r%-16s %d\r\n", "total probes:", probes)
	_, _ = fmt.Fprintf(w, "\r%-16s %d\r\n", "successful:", ok)
	_, _ = fmt.Fprintf(w, "\r%-16s %d\r\n", "failed:", fail)
	_, _ = fmt.Fprintf(w, "\r%-16s %.2f%%\r\n", "loss:", loss)
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
