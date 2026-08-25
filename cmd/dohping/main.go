// Command dohping is a better interactive ping: compact, stateful status
// display for a single host.
package main

import (
	"os"

	"golang.org/x/term"

	"dohping/internal/app"
	"dohping/internal/console"
)

func main() {
	console.EnableVT()
	code := app.Main(os.Args[1:], os.Stdout, os.Stderr, app.TTY{
		Stdout:    term.IsTerminal(int(os.Stdout.Fd())),
		Stdin:     term.IsTerminal(int(os.Stdin.Fd())),
		StdinFile: os.Stdin,
	})
	os.Exit(code)
}
