package cli

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, args ...string) *Options {
	t.Helper()
	opts, action, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", args, err)
	}
	if action != ActionRun {
		t.Fatalf("Parse(%q) action = %v, want ActionRun", args, action)
	}
	return opts
}

func mustFail(t *testing.T, args ...string) *UsageError {
	t.Helper()
	_, _, err := Parse(args)
	if err == nil {
		t.Fatalf("Parse(%q) succeeded, want usage error", args)
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("Parse(%q) error = %T, want *UsageError", args, err)
	}
	return ue
}

func TestParseDefaults(t *testing.T) {
	opts := mustParse(t, "192.168.1.23")
	if opts.Host != "192.168.1.23" {
		t.Errorf("Host = %q, want 192.168.1.23", opts.Host)
	}
	if opts.Interval != DefaultInterval {
		t.Errorf("Interval = %v, want %v", opts.Interval, DefaultInterval)
	}
	if opts.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", opts.Timeout, DefaultTimeout)
	}
	if opts.Count != 0 {
		t.Errorf("Count = %d, want 0 (unlimited)", opts.Count)
	}
	if opts.ColorMode != DefaultColorMode || opts.LiveMode != DefaultLiveMode {
		t.Errorf("ColorMode/LiveMode = %q/%q, want %q/%q", opts.ColorMode, opts.LiveMode, DefaultColorMode, DefaultLiveMode)
	}
	if opts.Window || opts.NoWindow {
		t.Errorf("Window/NoWindow = %v/%v, want false/false", opts.Window, opts.NoWindow)
	}
	if opts.WindowLines != DefaultWindowLines {
		t.Errorf("WindowLines = %d, want %d", opts.WindowLines, DefaultWindowLines)
	}
	if opts.DownAfter != DefaultDownAfter || opts.UpAfter != DefaultUpAfter {
		t.Errorf("DownAfter/UpAfter = %d/%d, want %d/%d", opts.DownAfter, opts.UpAfter, DefaultDownAfter, DefaultUpAfter)
	}
	if opts.ProbeType != ProbeICMP || opts.TCPPort != 0 {
		t.Errorf("ProbeType/TCPPort = %q/%d, want icmp/0", opts.ProbeType, opts.TCPPort)
	}
	if opts.LogFormat != DefaultLogFormat || opts.TimestampFormat != DefaultTimestampFmt {
		t.Errorf("LogFormat/TimestampFormat = %q/%q, want %q/%q", opts.LogFormat, opts.TimestampFormat, DefaultLogFormat, DefaultTimestampFmt)
	}
	if opts.Quiet || opts.NoHeader || opts.NoColor || opts.NoLive {
		t.Errorf("unexpected display flags set: %+v", opts)
	}
}

func TestShortFlags(t *testing.T) {
	opts := mustParse(t, "-i", "500ms", "-t", "1s", "-c", "5", "-q", "-l", "out.log",
		"-p", "tcp:8443", "-w", "-d", "2", "-u", "3", "example.com")
	if opts.Interval != 500*time.Millisecond {
		t.Errorf("Interval = %v, want 500ms", opts.Interval)
	}
	if opts.Timeout != time.Second {
		t.Errorf("Timeout = %v, want 1s", opts.Timeout)
	}
	if opts.Count != 5 {
		t.Errorf("Count = %d, want 5", opts.Count)
	}
	if !opts.Quiet {
		t.Error("Quiet = false, want true")
	}
	if opts.LogFile != "out.log" {
		t.Errorf("LogFile = %q, want out.log", opts.LogFile)
	}
	if opts.ProbeType != ProbeTCP || opts.TCPPort != 8443 {
		t.Errorf("ProbeType/TCPPort = %q/%d, want tcp/8443", opts.ProbeType, opts.TCPPort)
	}
	if !opts.Window {
		t.Error("Window = false, want true")
	}
	if opts.DownAfter != 2 || opts.UpAfter != 3 {
		t.Errorf("DownAfter/UpAfter = %d/%d, want 2/3", opts.DownAfter, opts.UpAfter)
	}
	if opts.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", opts.Host)
	}
}

func TestLongFlags(t *testing.T) {
	opts := mustParse(t,
		"--interval", "250ms", "--timeout", "3s", "--count", "10",
		"--no-header", "--no-color", "--color", "never", "--live", "off", "--no-live",
		"--window", "--window-lines", "8", "--down-after", "4", "--up-after", "5",
		"--probe", "tcp", "--log-file", "x.log", "--log-format", "json",
		"--timestamp-format", "rfc3339", "--quiet", "10.0.0.1")
	if opts.Interval != 250*time.Millisecond || opts.Timeout != 3*time.Second {
		t.Errorf("Interval/Timeout = %v/%v", opts.Interval, opts.Timeout)
	}
	if opts.Count != 10 || !opts.NoHeader || !opts.NoColor || !opts.NoLive || !opts.Quiet {
		t.Errorf("basic flags wrong: %+v", opts)
	}
	if opts.ColorMode != "never" || opts.LiveMode != "off" {
		t.Errorf("ColorMode/LiveMode = %q/%q", opts.ColorMode, opts.LiveMode)
	}
	if !opts.Window || opts.WindowLines != 8 {
		t.Errorf("Window/WindowLines = %v/%d, want true/8", opts.Window, opts.WindowLines)
	}
	if opts.DownAfter != 4 || opts.UpAfter != 5 {
		t.Errorf("DownAfter/UpAfter = %d/%d", opts.DownAfter, opts.UpAfter)
	}
	if opts.ProbeType != ProbeTCP || opts.TCPPort != DefaultTCPPort {
		t.Errorf("ProbeType/TCPPort = %q/%d, want tcp/%d", opts.ProbeType, opts.TCPPort, DefaultTCPPort)
	}
	if opts.LogFormat != "json" || opts.TimestampFormat != "rfc3339" {
		t.Errorf("LogFormat/TimestampFormat = %q/%q", opts.LogFormat, opts.TimestampFormat)
	}
}

func TestHelpAction(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}} {
		_, action, err := Parse(args)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", args, err)
		}
		if action != ActionHelp {
			t.Errorf("Parse(%q) action = %v, want ActionHelp", args, action)
		}
	}
}

func TestVersionAction(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-V"}} {
		_, action, err := Parse(args)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", args, err)
		}
		if action != ActionVersion {
			t.Errorf("Parse(%q) action = %v, want ActionVersion", args, action)
		}
	}
}

func TestWindowLinesImpliesWindow(t *testing.T) {
	opts := mustParse(t, "--window-lines", "5", "host.example")
	if !opts.Window {
		t.Error("Window = false, want true (implied by --window-lines)")
	}
	if opts.WindowLines != 5 {
		t.Errorf("WindowLines = %d, want 5", opts.WindowLines)
	}
	// Short form too.
	opts = mustParse(t, "-w", "host.example")
	if !opts.Window || opts.WindowLines != DefaultWindowLines {
		t.Errorf("Window/WindowLines = %v/%d", opts.Window, opts.WindowLines)
	}
}

func TestProbeParsing(t *testing.T) {
	tests := []struct {
		probe    string
		wantType string
		wantPort int
		wantErr  bool
	}{
		{"icmp", ProbeICMP, 0, false},
		{"tcp", ProbeTCP, 443, false},
		{"tcp:443", ProbeTCP, 443, false},
		{"tcp:8443", ProbeTCP, 8443, false},
		{"tcp:1", ProbeTCP, 1, false},
		{"tcp:65535", ProbeTCP, 65535, false},
		{"tcp:0", "", 0, true},
		{"tcp:65536", "", 0, true},
		{"tcp:abc", "", 0, true},
		{"tcp:", "", 0, true},
		{"udp", "", 0, true},
		{"icmp:443", "", 0, true},
		{"", "", 0, true},
	}
	for _, tt := range tests {
		opts, _, err := Parse([]string{"--probe", tt.probe, "h"})
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(probe=%q) succeeded, want error", tt.probe)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(probe=%q) error: %v", tt.probe, err)
			continue
		}
		if opts.ProbeType != tt.wantType || opts.TCPPort != tt.wantPort {
			t.Errorf("Parse(probe=%q) = %q/%d, want %q/%d", tt.probe, opts.ProbeType, opts.TCPPort, tt.wantType, tt.wantPort)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"missing host", []string{}, "missing required HOST"},
		{"too many args", []string{"a", "b"}, "too many arguments"},
		{"bad interval string", []string{"--interval", "nonsense", "h"}, "invalid value"},
		{"negative interval", []string{"--interval", "-1s", "h"}, "interval must be greater than zero"},
		{"zero interval", []string{"--interval", "0s", "h"}, "interval must be greater than zero"},
		{"negative timeout", []string{"--timeout", "-1s", "h"}, "timeout must be greater than zero"},
		{"zero timeout", []string{"--timeout", "0s", "h"}, "timeout must be greater than zero"},
		{"window-lines zero", []string{"--window-lines", "0", "h"}, "window-lines must be at least 1"},
		{"window-lines negative", []string{"--window-lines", "-2", "h"}, "window-lines must be at least 1"},
		{"bogus color", []string{"--color", "bogus", "h"}, "invalid color mode"},
		{"bogus live", []string{"--live", "sometimes", "h"}, "invalid live mode"},
		{"bogus log-format", []string{"--log-format", "xml", "h"}, "invalid log format"},
		{"bogus timestamp format", []string{"--timestamp-format", "epoch", "h"}, "invalid timestamp format"},
		{"count zero", []string{"--count", "0", "h"}, "count must be at least 1"},
		{"count negative", []string{"--count", "-3", "h"}, "count must be at least 1"},
		{"down-after zero", []string{"--down-after", "0", "h"}, "down-after must be at least 1"},
		{"up-after zero", []string{"--up-after", "0", "h"}, "up-after must be at least 1"},
		{"bad probe type", []string{"--probe", "udp", "h"}, "invalid probe type"},
		{"bad tcp port", []string{"--probe", "tcp:0", "h"}, "port must be"},
		{"empty host", []string{""}, "missing required HOST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ue := mustFail(t, tt.args...)
			if !strings.Contains(ue.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", ue.Error(), tt.wantSub)
			}
		})
	}
}

func TestConflicts(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"no-window x window-lines", []string{"--no-window", "--window-lines", "5", "h"}, "--no-window"},
		{"window-lines x no-window reversed", []string{"--window-lines", "5", "--no-window", "h"}, "--window-lines"},
		{"no-color x color=always", []string{"--no-color", "--color=always", "h"}, "--no-color"},
		{"color=always x no-color reversed", []string{"--color=always", "--no-color", "h"}, "--color=always"},
		{"no-live x live=on", []string{"--no-live", "--live=on", "h"}, "--no-live"},
		{"live=on x no-live reversed", []string{"--live=on", "--no-live", "h"}, "--live=on"},
		{"no-window x window", []string{"--no-window", "--window", "h"}, "--no-window"},
		{"window x no-window reversed", []string{"--window", "--no-window", "h"}, "--no-window"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ue := mustFail(t, tt.args...)
			if !strings.Contains(ue.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", ue.Error(), tt.wantSub)
			}
		})
	}
}

func TestNonConflicts(t *testing.T) {
	// Redundant-but-not-opposed combinations must parse fine.
	mustParse(t, "--no-color", "--color=never", "h")
	mustParse(t, "--color=never", "--no-color", "h")
	mustParse(t, "--no-live", "--live=off", "h")
	mustParse(t, "--no-window", "h")
	mustParse(t, "--window", "--window-lines", "5", "h")
}

func TestFlagsBeforeOrAfterHost(t *testing.T) {
	// GNU-style permutation (user acceptance fix 2026-08-17): flags may
	// appear on either side of the single positional HOST.
	cases := [][]string{
		{"host", "-c", "5"},
		{"-c", "5", "host"},
		{"host", "--interval", "5s", "-t", "3s"},
		{"--interval", "5s", "-t", "3s", "host"},
		{"host", "--window", "-c", "2"},
		{"-c", "2", "host", "--window"},
		{"example.com", "--probe", "tcp:8443", "-c", "1"},
	}
	for _, args := range cases {
		opts := mustParse(t, args...)
		if opts.Host != "host" && opts.Host != "example.com" {
			t.Errorf("Parse(%q) Host = %q, want the positional", args, opts.Host)
		}
	}
}

func TestFlagsAfterHostStillValidates(t *testing.T) {
	// Flags after the host are real flags, not extra positionals: they
	// still get validated (conflicts, values).
	ue := mustFail(t, "host", "--window-lines", "0")
	if !strings.Contains(ue.Error(), "window-lines") {
		t.Errorf("error = %q, want window-lines validation", ue.Error())
	}
	// Two positionals are still an error, wherever they sit.
	ue = mustFail(t, "a", "b", "-c", "5")
	if !strings.Contains(ue.Error(), "too many arguments") {
		t.Errorf("error = %q, want too-many-arguments", ue.Error())
	}
	ue = mustFail(t, "-c", "5", "a", "b")
	if !strings.Contains(ue.Error(), "too many arguments") {
		t.Errorf("error = %q, want too-many-arguments", ue.Error())
	}
}

func TestIntervalTimeoutAcceptsSeconds(t *testing.T) {
	// Bare numbers are seconds (ping's -i convention), not a parse error.
	opts := mustParse(t, "-i", "5", "h")
	if opts.Interval != 5*time.Second {
		t.Errorf("Interval = %v, want 5s", opts.Interval)
	}
	opts = mustParse(t, "--interval", "5", "h")
	if opts.Interval != 5*time.Second {
		t.Errorf("Interval = %v, want 5s", opts.Interval)
	}
	opts = mustParse(t, "-t", "3", "h")
	if opts.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", opts.Timeout)
	}
	opts = mustParse(t, "--timeout", "3", "h")
	if opts.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", opts.Timeout)
	}
	// Fractional seconds and full duration strings still work.
	opts = mustParse(t, "-i", "0.5", "h")
	if opts.Interval != 500*time.Millisecond {
		t.Errorf("Interval = %v, want 500ms", opts.Interval)
	}
	opts = mustParse(t, "-i", "1m30s", "h")
	if opts.Interval != 90*time.Second {
		t.Errorf("Interval = %v, want 90s", opts.Interval)
	}
	opts = mustParse(t, "-i", "250ms", "h")
	if opts.Interval != 250*time.Millisecond {
		t.Errorf("Interval = %v, want 250ms", opts.Interval)
	}
}

func TestIntervalSecondsAfterHost(t *testing.T) {
	// The exact user invocation: `dohping HOST -i 5`.
	opts := mustParse(t, "google.com", "-i", "5")
	if opts.Host != "google.com" || opts.Interval != 5*time.Second {
		t.Errorf("Host/Interval = %q/%v, want google.com/5s", opts.Host, opts.Interval)
	}
}

func TestBadDurationMessage(t *testing.T) {
	ue := mustFail(t, "-i", "nonsense", "h")
	if !strings.Contains(ue.Error(), "number of seconds") {
		t.Errorf("error = %q, want helpful duration hint", ue.Error())
	}
}

func TestHelpContent(t *testing.T) {
	var sb strings.Builder
	WriteHelp(&sb)
	out := sb.String()
	for _, want := range []string{
		"Usage:", "dohping [options] HOST",
		"--help", "-V, --version", "-i, --interval", "-t, --timeout", "-c, --count",
		"-p, --probe", "-d, --down-after", "-u, --up-after",
		"-q, --quiet", "--no-header", "--no-color", "--color MODE", "--live MODE",
		"--no-live", "-w, --window", "--no-window", "--window-lines",
		"--timestamp-format", "-l, --log-file", "--log-format",
		"Implies --window", "(default 10)",
		"Exit codes", "130", "143", "NO_COLOR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q", want)
		}
	}
}
