// Command sitrep is the Go port of the sitrep shell scripts. Commands arrive phase by phase; see docs/GO-PORT.md.
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
