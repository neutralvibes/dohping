package app

import (
	"bytes"
	"strings"
	"testing"
)

var noTTY = TTY{Stdout: false, Stdin: false}

func TestMainHelp(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--help"}, &out, &errb, noTTY)
	if code != ExitOK {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "dohping [options] HOST") {
		t.Errorf("help output missing usage: %q", out.String())
	}
	if errb.Len() != 0 {
		t.Errorf("stderr not empty: %q", errb.String())
	}
}

func TestMainVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-V"}} {
		var out, errb bytes.Buffer
		code := Main(args, &out, &errb, noTTY)
		if code != ExitOK {
			t.Errorf("Main(%q) exit = %d, want 0", args, code)
		}
		if !strings.HasPrefix(out.String(), "dohping ") {
			t.Errorf("version output = %q, want prefix \"dohping \"", out.String())
		}
	}
}

func TestMainMissingHost(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main(nil, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "missing required HOST") {
		t.Errorf("stderr = %q, want missing-host message", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout not empty on usage error: %q", out.String())
	}
}

func TestMainInvalidValue(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--window-lines", "0", "h"}, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "window-lines") {
		t.Errorf("stderr = %q, want window-lines message", errb.String())
	}
}

func TestMainConflict(t *testing.T) {
	var out, errb bytes.Buffer
	code := Main([]string{"--no-window", "--window-lines", "5", "h"}, &out, &errb, noTTY)
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errb.String(), "conflict") {
		t.Errorf("stderr = %q, want conflict message", errb.String())
	}
}
