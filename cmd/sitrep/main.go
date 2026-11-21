// Command sitrep is sitrep: per-session queues, the status line, the hook and the dashboard for Claude Code sessions, in one program.
package main

import (
	"os"
	"syscall"

	"github.com/lfeagan/claude-sitrep/internal/cli"
)

func main() {
	syscall.Umask(0o077) // state holds prompt text: owner-only
	os.Exit(cli.Run(os.Args[1:], cli.OSEnv()))
}
