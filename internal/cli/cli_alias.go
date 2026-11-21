package cli

import (
	"fmt"
	"path/filepath"

	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// cmdAlias names a session, removes its name, or lists the names.
func cmdAlias(e Env, args []string) error {
	const usage = "usage: sitrep alias [<id> <name> | <id> --off]"
	st := e.store()
	if len(args) == 0 {
		for _, a := range st.AliasList() {
			dir := filepath.Base(textsafe.Clean(state.FirstLine(st.Path(a.ID, "cwd")), 200).String())
			if dir == "." || dir == "/" {
				dir = "-"
			}
			fmt.Fprintf(e.Stdout, "%s  %s  %s\n", a.Name, queue.Label(a.ID), dir)
		}
		return nil
	}
	if len(args) != 2 {
		return usageError(usage)
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	if args[1] == "--off" {
		old, err := st.ClearAlias(id)
		if err != nil {
			return err
		}
		if old == "" {
			fmt.Fprintf(e.Stdout, "%s has no alias\n", queue.Label(id))
		} else {
			fmt.Fprintf(e.Stdout, "%s is no longer called %s\n", queue.Label(id), old)
		}
		return nil
	}
	changed, err := st.SetAlias(id, args[1])
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintf(e.Stdout, "%s is already called %s\n", queue.Label(id), args[1])
		return nil
	}
	fmt.Fprintf(e.Stdout, "%s is now called %s\n", queue.Label(id), args[1])
	return nil
}
