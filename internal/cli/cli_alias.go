package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/lfeagan/claude-sitrep/internal/histlog"
	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// cmdAlias names a session, removes its name, or lists the names.
func cmdAlias(e Env, args []string) error {
	const usage = "usage: sitrep alias [<id> <name> [--take] | <id> --off]"
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
	take := len(args) == 3 && args[2] == "--take"
	if len(args) != 2 && !take {
		return usageError(usage)
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	if args[1] == "--off" && !take {
		old, err := st.ClearAlias(id)
		if err != nil {
			return err
		}
		if old == "" {
			fmt.Fprintf(e.Stdout, "%s has no alias\n", queue.Label(id))
		} else {
			e.log().Record(histlog.Entry{Op: histlog.Alias, ID: id, Old: textsafe.Clean(old, 64)})
			fmt.Fprintf(e.Stdout, "%s is no longer called %s\n", queue.Label(id), old)
		}
		return nil
	}
	old := st.Alias(id)
	ended := e.endedFunc()
	var mayTake func(state.ID) bool
	if take {
		mayTake = ended
	}
	changed, from, err := st.SetAliasFrom(id, args[1], mayTake)
	var taken *state.AliasTakenError
	if errors.As(err, &taken) {
		switch {
		case take: // the holder is running, or cannot be told to have ended
			return fmt.Errorf("'%s' is the alias of %s, which has not ended, so it cannot be taken; free it with 'sitrep alias %s --off'", taken.Name, queue.Label(taken.Holder), queue.Label(taken.Holder))
		case ended(taken.Holder):
			return fmt.Errorf("'%s' is the alias of %s, which has ended; take the name with 'sitrep alias %s %s --take', or free it with 'sitrep alias %s --off'", taken.Name, queue.Label(taken.Holder), queue.Label(id), taken.Name, queue.Label(taken.Holder))
		}
	}
	if err != nil {
		return err
	}
	if from != "" {
		e.log().Record(histlog.Entry{Op: histlog.Alias, ID: from, Old: textsafe.Clean(args[1], 64)})
	}
	if !changed && from == "" {
		fmt.Fprintf(e.Stdout, "%s is already called %s\n", queue.Label(id), args[1])
		return nil
	}
	e.log().Record(histlog.Entry{Op: histlog.Alias, ID: id, Text: textsafe.Clean(args[1], 64), Old: textsafe.Clean(old, 64)})
	if from != "" {
		fmt.Fprintf(e.Stdout, "%s is now called %s, taken from %s, which has ended\n", queue.Label(id), args[1], queue.Label(from))
		return nil
	}
	fmt.Fprintf(e.Stdout, "%s is now called %s\n", queue.Label(id), args[1])
	return nil
}

// endedFunc says whether a session has ended, as the listing does: Claude Code's registry has no live process for it, or it said so itself.
// It reads the registry once, and only when asked. A session that cannot be judged (no state to read) has not ended.
func (e Env) endedFunc() func(state.ID) bool {
	var set *live.Set
	return func(id state.ID) bool {
		st := e.store()
		changed, ok := live.Changed(st, string(id))
		if !ok {
			return false
		}
		if set == nil {
			r := e.registry().Read(context.Background())
			set = &r
		}
		return live.Dead(*set, string(id), live.Word(st, string(id)), changed, e.Clock.Now().Unix())
	}
}
