package theme

import (
	"strings"
	"testing"

	"dohping/internal/state"
)

func TestEnabledRules(t *testing.T) {
	auto := Config{NoColor: false, ColorMode: "auto"}
	always := Config{NoColor: false, ColorMode: "always"}
	never := Config{ColorMode: "never"}
	noColor := Config{NoColor: true, ColorMode: "auto"}
	clean := Env{NO_COLOR: "", TERM: "xterm-256color"}
	dumb := Env{NO_COLOR: "", TERM: "dumb"}
	noColorEnv := Env{NO_COLOR: "1", TERM: "xterm-256color"}

	tests := []struct {
		name  string
		cfg   Config
		isTTY bool
		env   Env
		want  bool
	}{
		{"auto TTY", auto, true, clean, true},
		{"auto piped", auto, false, clean, false},
		{"always piped", always, false, clean, false}, // stdout not a TTY still disables
		{"always TTY", always, true, clean, true},
		{"never TTY", never, true, clean, false},
		{"no-color TTY", noColor, true, clean, false},
		{"TERM dumb TTY", auto, true, dumb, false},
		{"TERM dumb always", always, true, dumb, false},
		{"NO_COLOR TTY", auto, true, noColorEnv, false},
		{"NO_COLOR overrides always", always, true, noColorEnv, false},
		{"NO_COLOR empty ok", auto, true, Env{NO_COLOR: "", TERM: "xterm"}, true},
		{"NO_COLOR whitespace counts", always, true, Env{NO_COLOR: " ", TERM: "xterm"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Enabled(tt.cfg, tt.isTTY, tt.env); got != tt.want {
				t.Errorf("Enabled(%+v, %v, %+v) = %v, want %v", tt.cfg, tt.isTTY, tt.env, got, tt.want)
			}
		})
	}
}

func TestRendererDisabledReturnsPlain(t *testing.T) {
	r := NewRenderer(false, Default)
	for _, role := range []Role{RoleStatusUp, RoleStatusDown, RoleHeader, RoleTimestamp, RoleDuration, RoleFails} {
		if got := r.Paint("hello", role); got != "hello" {
			t.Errorf("disabled Paint(%q, %v) = %q, want plain", "hello", role, got)
		}
	}
	if got := r.PaintStatus("up", state.StatusUp); got != "up" {
		t.Errorf("disabled PaintStatus = %q", got)
	}
}

func TestRendererEnabledColors(t *testing.T) {
	r := NewRenderer(true, Default)
	got := r.PaintStatus("up", state.StatusUp)
	if !strings.Contains(got, Green) || !strings.Contains(got, Reset) {
		t.Errorf("PaintStatus(up) = %q, want green + reset", got)
	}
	got = r.PaintStatus("down", state.StatusDown)
	if !strings.Contains(got, Red) {
		t.Errorf("PaintStatus(down) = %q, want red", got)
	}
	got = r.Paint("ts", RoleTimestamp)
	if got != "ts" {
		t.Errorf("Paint(timestamp) = %q, want default intensity (no SGR) per user correction 2026-08-17", got)
	}
	got = r.Paint("h", RoleHeader)
	if !strings.Contains(got, Bold) {
		t.Errorf("Paint(header) = %q, want bold", got)
	}
	// Custom theme is respected.
	custom := Default
	custom.Status = map[state.Status]string{state.StatusUp: Blue}
	r2 := NewRenderer(true, custom)
	if got := r2.PaintStatus("up", state.StatusUp); !strings.Contains(got, Blue) {
		t.Errorf("custom theme not applied: %q", got)
	}
}

func TestZeroThemeNoColor(t *testing.T) {
	r := NewRenderer(true, Theme{})
	if got := r.Paint("x", RoleStatusUp); got != "x" {
		t.Errorf("zero theme painted: %q", got)
	}
}
