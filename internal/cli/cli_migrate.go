package cli

import (
	"errors"
	"fmt"

	"github.com/lfeagan/claude-sitrep/internal/migrate"
)

// cmdMigrate moves state of the old flat layout into one folder per session.
func cmdMigrate(e Env, args []string) error {
	dry := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--dry-run":
		dry = true
	default:
		return usageError("usage: sitrep migrate [--dry-run]")
	}
	r := migrate.Run(e.store(), dry)
	for _, l := range r.Lines {
		fmt.Fprintln(e.Stdout, l)
	}
	verb := ""
	if dry {
		verb = "would be "
	}
	fmt.Fprintf(e.Stdout, "%d %smoved, %d %smerged, %d left, %d %sdeleted, %d errors\n", r.Moved, verb, r.Merged, verb, r.Left, r.Deleted, verb, r.Errors)
	if r.Errors > 0 {
		return errors.New("some files could not be moved: run it again, or look at the errors above")
	}
	return nil
}
