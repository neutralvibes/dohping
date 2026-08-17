// Package cli parses and validates the dohping command line.
//
// It owns the full CLI contract: every flag, its short form, defaults,
// validation rules, conflict detection, and the help text. It performs no
// probing, display, or logging — it only decides what the user asked for.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// Defaults — the CLI contract defaults (spec §16).
const (
	DefaultInterval       = 1 * time.Second
	DefaultTimeout        = 2 * time.Second
	DefaultWindowLines    = 10
	DefaultDownAfter      = 1
	DefaultUpAfter        = 1
	DefaultTCPPort        = 443
	DefaultProbe          = "icmp"
	DefaultColorMode      = "auto"
	DefaultLiveMode       = "auto"
	DefaultLogFormat      = "text"
	DefaultTimestampFmt   = "HH:MM:SS"
	MaxHostWidth          = 40
	MinHostWidth          = 15
	DurationOverflowGuard = 99 * 24 * time.Hour // displayed as "99d+"
)

// Probe type constants.
const (
	ProbeICMP = "icmp"
	ProbeTCP  = "tcp"
)

// Action tells the caller what to do after a successful parse.
type Action int

const (
	// ActionRun proceeds to monitoring.
	ActionRun Action = iota
	// ActionHelp prints help and exits 0.
	ActionHelp
	// ActionVersion prints version and exits 0.
	ActionVersion
)

// Options is the fully validated configuration for one run.
type Options struct {
	Host string

	Interval time.Duration
	Timeout  time.Duration
	Count    int // 0 = unlimited

	Quiet     bool
	NoHeader  bool
	NoColor   bool
	ColorMode string // auto | always | never
	LiveMode  string // auto | on | off
	NoLive    bool

	Window      bool
	NoWindow    bool
	WindowLines int

	DownAfter int
	UpAfter   int

	LogFile   string
	LogFormat string // text | json

	TimestampFormat string // HH:MM:SS | rfc3339

	Probe string // icmp | tcp[:PORT]

	Help    bool
	Version bool

	// Parsed probe details (derived from Probe).
	ProbeType string // icmp | tcp
	TCPPort   int    // meaningful when ProbeType == tcp

	// Explicitly-set tracking, used for conflict detection and to
	// distinguish defaults from explicit values.
	countSet       bool
	windowLinesSet bool
	windowSet      bool
	noColorSet     bool
	colorSet       bool
	liveSet        bool
	noLiveSet      bool
	noWindowSet    bool
}

// secondsOrDuration implements flag.Value for interval/timeout flags:
// it accepts either a bare number of seconds ("5" → 5s, matching ping's
// `-i 5` convention) or a full Go duration string ("500ms", "1m30s").
// "0" or a negative number parses and is then rejected by validation.
type secondsOrDuration struct {
	d *time.Duration
}

func (v secondsOrDuration) String() string { return v.d.String() }

func (v secondsOrDuration) Set(s string) error {
	if d, err := time.ParseDuration(s); err == nil {
		*v.d = d
		return nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
		*v.d = time.Duration(n * float64(time.Second))
		return nil
	}
	return fmt.Errorf("expected a number of seconds (e.g. 5) or a duration like 5s or 1m30s, got %q", s)
}

// UsageError marks invalid usage; the caller maps it to exit code 2.
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

func usageErrorf(format string, a ...any) error {
	return &UsageError{msg: fmt.Sprintf(format, a...)}
}

// Parse parses args (without the program name), validates them, and returns
// the resulting options plus the action the caller must take.
//
// A returned error is always a *UsageError (exit code 2).
func Parse(args []string) (*Options, Action, error) {
	opts := &Options{
		Interval:        DefaultInterval,
		Timeout:         DefaultTimeout,
		ColorMode:       DefaultColorMode,
		LiveMode:        DefaultLiveMode,
		WindowLines:     DefaultWindowLines,
		DownAfter:       DefaultDownAfter,
		UpAfter:         DefaultUpAfter,
		LogFormat:       DefaultLogFormat,
		TimestampFormat: DefaultTimestampFmt,
		Probe:           DefaultProbe,
		ProbeType:       ProbeICMP,
		TCPPort:         0,
	}

	fs := flag.NewFlagSet("dohping", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we own all output
	fs.Usage = func() {}

	// Basic options (spec §16.1).
	fs.BoolVar(&opts.Help, "h", false, "")
	fs.BoolVar(&opts.Help, "help", false, "")
	fs.BoolVar(&opts.Version, "V", false, "")
	fs.BoolVar(&opts.Version, "version", false, "")
	// Interval/timeout accept either a bare number of seconds ("5" → 5s,
	// matching ping's -i convention) or a Go duration string ("500ms",
	// "1m30s") — see secondsOrDuration.
	fs.Var(secondsOrDuration{&opts.Interval}, "i", "")
	fs.Var(secondsOrDuration{&opts.Interval}, "interval", "")
	fs.Var(secondsOrDuration{&opts.Timeout}, "t", "")
	fs.Var(secondsOrDuration{&opts.Timeout}, "timeout", "")
	fs.IntVar(&opts.Count, "c", 0, "")
	fs.IntVar(&opts.Count, "count", 0, "")
	fs.StringVar(&opts.Probe, "p", DefaultProbe, "")
	fs.StringVar(&opts.Probe, "probe", DefaultProbe, "")
	fs.IntVar(&opts.DownAfter, "d", opts.DownAfter, "")
	fs.IntVar(&opts.DownAfter, "down-after", opts.DownAfter, "")
	fs.IntVar(&opts.UpAfter, "u", opts.UpAfter, "")
	fs.IntVar(&opts.UpAfter, "up-after", opts.UpAfter, "")

	// Display options (spec §16.2).
	fs.BoolVar(&opts.Quiet, "q", false, "")
	fs.BoolVar(&opts.Quiet, "quiet", false, "")
	fs.BoolVar(&opts.NoHeader, "no-header", false, "")
	fs.BoolVar(&opts.NoColor, "no-color", false, "")
	fs.StringVar(&opts.ColorMode, "color", opts.ColorMode, "")
	fs.StringVar(&opts.LiveMode, "live", opts.LiveMode, "")
	fs.BoolVar(&opts.NoLive, "no-live", false, "")
	fs.BoolVar(&opts.Window, "w", false, "")
	fs.BoolVar(&opts.Window, "window", false, "")
	fs.BoolVar(&opts.NoWindow, "no-window", false, "")
	fs.IntVar(&opts.WindowLines, "window-lines", opts.WindowLines, "")
	fs.StringVar(&opts.TimestampFormat, "timestamp-format", opts.TimestampFormat, "")

	// Logging options (spec §16.3).
	fs.StringVar(&opts.LogFile, "l", "", "")
	fs.StringVar(&opts.LogFile, "log-file", "", "")
	fs.StringVar(&opts.LogFormat, "log-format", opts.LogFormat, "")

	// Parse with GNU-style flag permutation: flags may appear before or
	// after the positional HOST. Go's flag package stops at the first
	// non-flag argument, so parse iteratively — each round consumes
	// flags up to the next positional, which is collected, then parsing
	// continues with the remainder (user acceptance fix).
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return opts, ActionHelp, nil
			}
			return nil, ActionRun, usageErrorf("%v", err)
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positionals = append(positionals, rest[0])
		rest = rest[1:]
	}

	// Track which conflict-relevant flags were explicitly set.
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "count", "c":
			opts.countSet = true
		case "window-lines":
			opts.windowLinesSet = true
		case "window", "w":
			opts.windowSet = true
		case "no-color":
			opts.noColorSet = true
		case "color":
			opts.colorSet = true
		case "live":
			opts.liveSet = true
		case "no-live":
			opts.noLiveSet = true
		case "no-window":
			opts.noWindowSet = true
		}
	})

	if opts.Help {
		return opts, ActionHelp, nil
	}
	if opts.Version {
		return opts, ActionVersion, nil
	}

	// Positional HOST argument.
	if len(positionals) == 0 {
		return nil, ActionRun, usageErrorf("missing required HOST argument")
	}
	if len(positionals) > 1 {
		return nil, ActionRun, usageErrorf("too many arguments: unexpected %q (expected exactly one HOST)", positionals[1])
	}
	opts.Host = positionals[0]

	if err := validate(opts); err != nil {
		return nil, ActionRun, err
	}
	return opts, ActionRun, nil
}

// validate checks value ranges, allowed choices, probe syntax, and flag
// conflicts. It mutates opts where parsing derives values (probe details,
// window inference).
func validate(opts *Options) error {
	// Conflicts first — never silently resolve ambiguity (spec §16.4).
	if opts.noWindowSet && opts.windowLinesSet {
		return usageErrorf("--no-window conflicts with --window-lines: remove one of them")
	}
	if opts.noWindowSet && opts.windowSet {
		return usageErrorf("--no-window conflicts with --window: remove one of them")
	}
	if opts.noColorSet && opts.colorSet && opts.ColorMode == "always" {
		return usageErrorf("--no-color conflicts with --color=always: remove one of them")
	}
	if opts.noLiveSet && opts.liveSet && opts.LiveMode == "on" {
		return usageErrorf("--no-live conflicts with --live=on: remove one of them")
	}

	// --window-lines implies --window (spec §8.1, §16.2).
	if opts.windowLinesSet {
		opts.Window = true
	}

	if opts.Host == "" {
		return usageErrorf("missing required HOST argument")
	}

	if opts.Interval <= 0 {
		return usageErrorf("interval must be greater than zero (got %s)", opts.Interval)
	}
	if opts.Timeout <= 0 {
		return usageErrorf("timeout must be greater than zero (got %s)", opts.Timeout)
	}
	if opts.CountSet() && opts.Count < 1 {
		return usageErrorf("count must be at least 1 (got %d)", opts.Count)
	}
	if opts.WindowLines < 1 {
		return usageErrorf("window-lines must be at least 1 (got %d)", opts.WindowLines)
	}
	if opts.DownAfter < 1 {
		return usageErrorf("down-after must be at least 1 (got %d)", opts.DownAfter)
	}
	if opts.UpAfter < 1 {
		return usageErrorf("up-after must be at least 1 (got %d)", opts.UpAfter)
	}

	switch opts.ColorMode {
	case "auto", "always", "never":
	default:
		return usageErrorf("invalid color mode %q: must be auto, always, or never", opts.ColorMode)
	}
	switch opts.LiveMode {
	case "auto", "on", "off":
	default:
		return usageErrorf("invalid live mode %q: must be auto, on, or off", opts.LiveMode)
	}
	switch opts.LogFormat {
	case "text", "json":
	default:
		return usageErrorf("invalid log format %q: must be text or json", opts.LogFormat)
	}
	switch opts.TimestampFormat {
	case "HH:MM:SS", "rfc3339":
	default:
		return usageErrorf("invalid timestamp format %q: must be HH:MM:SS or rfc3339", opts.TimestampFormat)
	}

	probeType, port, err := parseProbe(opts.Probe)
	if err != nil {
		return usageErrorf("%v", err)
	}
	opts.ProbeType = probeType
	opts.TCPPort = port

	return nil
}

// CountSet reports whether --count was explicitly given (distinct from the
// unlimited default of 0).
func (o *Options) CountSet() bool { return o.countSet }

// parseProbe parses "icmp" | "tcp" | "tcp:PORT" into a type and port.
func parseProbe(s string) (probeType string, port int, err error) {
	switch {
	case s == ProbeICMP:
		return ProbeICMP, 0, nil
	case s == ProbeTCP:
		return ProbeTCP, DefaultTCPPort, nil
	case strings.HasPrefix(s, "tcp:"):
		ps := strings.TrimPrefix(s, "tcp:")
		if ps == "" {
			return "", 0, fmt.Errorf("invalid probe %q: missing port", s)
		}
		n, perr := strconv.Atoi(ps)
		if perr != nil || n < 1 || n > 65535 {
			return "", 0, fmt.Errorf("invalid probe %q: port must be a number in 1-65535", s)
		}
		return ProbeTCP, n, nil
	default:
		return "", 0, fmt.Errorf("invalid probe type %q: must be icmp or tcp[:PORT]", s)
	}
}
