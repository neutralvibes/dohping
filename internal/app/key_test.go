package app

import "testing"

// classifyKey maps the raw-mode stdin bytes to key events. This is the
// q-quit path's byte-mapping contract (startKeyReader needs a real PTY, so
// the pure mapping is tested here directly).

func TestClassifyKey(t *testing.T) {
	cases := []struct {
		name  string
		b     byte
		want  keyEvent
		valid bool
	}{
		{"lowercase q quits", 'q', keyQuit, true},
		{"uppercase Q quits", 'Q', keyQuit, true},
		{"Ctrl-C interrupts", 0x03, keyCtrlC, true},
		{"Ctrl-D is EOF", 0x04, keyEOF, true},
		{"space ignored", ' ', 0, false},
		{"arrow escape ignored", 0x1b, 0, false},
		{"enter ignored", '\n', 0, false},
		{"null ignored", 0x00, 0, false},
	}
	for _, c := range cases {
		got, ok := classifyKey(c.b)
		if ok != c.valid {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.valid)
		}
		if ok && got != c.want {
			t.Errorf("%s: event = %v, want %v", c.name, got, c.want)
		}
	}
}
