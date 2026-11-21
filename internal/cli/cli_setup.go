package cli

import (
	"errors"
	"os"

	"github.com/lfeagan/claude-sitrep/internal/setup"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

// cmdSetup puts the sitrep block on the status line, or with --remove takes it off.
func cmdSetup(e Env, args []string) error {
	if e.Getenv(state.EnvDir) != "" {
		return errors.New(state.EnvDir + " is set, and setup installs into ~/.claude/sitrep: unset it for this command, and nothing was changed")
	}
	remove := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--remove":
		remove = true
	default:
		return usageError("usage: sitrep setup [--remove]")
	}
	exe := e.Executable
	if exe == nil {
		exe = os.Executable
	}
	s := setup.Setup{Home: e.Home, BinDir: e.Getenv("BIN_DIR"), SkillsDir: e.Getenv("SKILLS_DIR"), Executable: exe, Now: e.Clock.Now, Out: e.Stdout, Err: e.Stderr}
	if code := s.Run(remove); code != 0 {
		return exitError(code)
	}
	return nil
}
