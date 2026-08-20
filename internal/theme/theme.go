// Package theme defines semantic color roles and the color enable/disable
// rules (spec §11). Colors are keyed by role, never hardcoded in output
// logic, and the default theme is trivially modifiable in one place.
package theme

import "dohping/internal/state"

// Role is a semantic color role (spec §11.1).
type Role int

const (
	RoleStatusUp Role = iota
	RoleStatusDown
	RoleStatusUnknown
	RoleStatusError
	RoleHeader
	RoleTimestamp
	RoleDuration
	RoleFails
)

// ANSI SGR codes.
const (
	Reset   = "\x1b[0m"
	Bold    = "\x1b[1m"
	Dim     = "\x1b[2m"
	Red     = "\x1b[31m"
	Green   = "\x1b[32m"
	Yellow  = "\x1b[33m"
	Blue    = "\x1b[34m"
	Magenta = "\x1b[35m"
	Cyan    = "\x1b[36m"
)

// Theme maps roles to SGR codes. The zero Theme disables coloring for
// every role (empty code → Paint returns s unchanged).
type Theme struct {
	Status                      map[state.Status]string
	Header, Timestamp, Duration string
	Fails                       string
}

// Default follows spec §11.2: up green, down red, unknown yellow, error
// magenta, header bold, timestamp default intensity (user correction
// 2026-08-17: dim was too hard to read — the leading column needs no
// extra muting), duration cyan, failure count red.
var Default = Theme{
	Status: map[state.Status]string{
		state.StatusUp:      Green,
		state.StatusDown:    Red,
		state.StatusUnknown: Yellow,
		state.StatusError:   Magenta,
	},
	Header:    Bold,
	Timestamp: "",
	Duration:  Cyan,
	Fails:     Red,
}

// Config is the CLI color configuration.
type Config struct {
	NoColor   bool
	ColorMode string // auto | always | never
}

// Env is the process environment relevant to color decisions.
type Env struct {
	NO_COLOR string
	TERM     string
}

// Enabled decides whether color output is active (spec §11.3). Disabled
// when any of: --no-color / --color=never, NO_COLOR set non-empty,
// stdout not a terminal, TERM=dumb. NO_COLOR overrides --color=always
// (spec §11.3, §16.4).
func Enabled(cfg Config, isTTY bool, env Env) bool {
	if cfg.NoColor || cfg.ColorMode == "never" {
		return false
	}
	if env.NO_COLOR != "" {
		return false
	}
	if !isTTY {
		return false
	}
	if env.TERM == "dumb" {
		return false
	}
	return true
}

// Renderer applies a theme conditionally.
type Renderer struct {
	enabled bool
	t       Theme
}

// NewRenderer returns a renderer. When enabled is false every Paint call
// returns its input unchanged — zero ANSI in output.
func NewRenderer(enabled bool, t Theme) *Renderer { return &Renderer{enabled: enabled, t: t} }

// Enabled reports whether coloring is active.
func (r *Renderer) Enabled() bool { return r.enabled }

// Paint wraps s in the role's color code. Returns s unchanged when color
// is disabled or the role has no code.
func (r *Renderer) Paint(s string, role Role) string {
	if !r.enabled {
		return s
	}
	var code string
	switch role {
	case RoleStatusUp:
		code = r.t.Status[state.StatusUp]
	case RoleStatusDown:
		code = r.t.Status[state.StatusDown]
	case RoleStatusUnknown:
		code = r.t.Status[state.StatusUnknown]
	case RoleStatusError:
		code = r.t.Status[state.StatusError]
	case RoleHeader:
		code = r.t.Header
	case RoleTimestamp:
		code = r.t.Timestamp
	case RoleDuration:
		code = r.t.Duration
	case RoleFails:
		code = r.t.Fails
	}
	if code == "" {
		return s
	}
	return code + s + Reset
}

// PaintStatus paints s with the role for the given status.
func (r *Renderer) PaintStatus(s string, st state.Status) string {
	switch st {
	case state.StatusUp:
		return r.Paint(s, RoleStatusUp)
	case state.StatusDown:
		return r.Paint(s, RoleStatusDown)
	case state.StatusUnknown:
		return r.Paint(s, RoleStatusUnknown)
	default:
		return r.Paint(s, RoleStatusError)
	}
}
