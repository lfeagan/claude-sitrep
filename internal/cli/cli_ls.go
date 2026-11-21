package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/prune"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

func (e Env) conf() conf.Conf {
	return conf.Conf{File: conf.Open(e.store().Dir).File, Getenv: e.Getenv}
}

func (e Env) registry() live.Registry {
	r := live.NewRegistry(e.configDir(), e.Runner)
	if e.Alive != nil {
		r.Alive = e.Alive
	}
	return r
}

func (e Env) builder() snapshot.Builder {
	return snapshot.Builder{Store: e.store(), ConfigDir: e.configDir(), Conf: e.conf(), Reg: e.registry(), Clock: e.Clock}
}

func cmdLs(e Env, args []string) error {
	const usage = "usage: sitrep ls [-a] [-v] [-n items] [--json [--full]] [id]"
	var items string
	var all, asJSON, full bool
loop:
	for len(args) > 0 {
		switch args[0] {
		case "-v":
			args = args[1:]
		case "--json":
			asJSON, args = true, args[1:]
		case "--full":
			full, args = true, args[1:]
		case "-a", "--all":
			all, args = true, args[1:]
		case "-n":
			if len(args) < 2 {
				return usageError(usage)
			}
			items, args = args[1], args[2:]
		default:
			break loop
		}
	}
	if items == "" {
		items = strconv.Itoa(e.conf().QueueItems())
	}
	if !isDigits(items) {
		return errors.New("items must be a number")
	}
	if full && !asJSON {
		return errors.New("--full needs --json")
	}
	for _, a := range args {
		if a == "--json" {
			return errors.New("--json goes before an id, and takes none")
		}
	}
	if !asJSON {
		return errors.New("ls: the plain listing is not ported yet")
	}
	if len(args) > 0 {
		return usageError("usage: sitrep ls --json [-a] [-n items | --full]")
	}
	cap, _ := strconv.Atoi(items)
	if full {
		cap = -1
	}
	return snapshot.Encode(e.Stdout, e.builder().Build(context.Background(), snapshot.Options{All: all, Cap: cap}))
}

func cmdSnooze(e Env, args []string) error {
	const usage = "usage: sitrep snooze [--off] <id>"
	off := false
	if len(args) > 0 && args[0] == "--off" {
		off, args = true, args[1:]
	}
	if len(args) != 1 {
		return usageError(usage)
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	if id == state.Inbox {
		return errors.New("the inbox is not a session")
	}
	if off {
		if err := e.notes().Unsnooze(id); err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, "%s is no longer snoozed\n", queue.Label(id))
		return nil
	}
	changed, ok := live.Changed(e.store(), string(id))
	if !ok {
		return fmt.Errorf("cannot read when %s last changed", queue.Label(id))
	}
	if err := e.notes().Snooze(id, changed); err != nil {
		return fmt.Errorf("cannot snooze %s", queue.Label(id))
	}
	fmt.Fprintf(e.Stdout, "snoozed %s until it changes\n", queue.Label(id))
	return nil
}

func cmdPrune(e Env, args []string) error {
	const usage = "usage: sitrep prune [-y | -k | --dry-run]"
	var yes, dry, keep bool
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		case "--dry-run":
			dry = true
		case "-k", "--keep":
			keep = true
		default:
			return usageError(usage)
		}
	}
	n := 0
	for _, b := range []bool{yes, dry, keep} {
		if b {
			n++
		}
	}
	if n > 1 {
		return usageError(usage)
	}
	p := prune.Pruner{Store: e.store(), ConfigDir: e.configDir(), Reg: e.registry()}
	actions, err := p.Plan(context.Background(), e.Clock.Now().Unix())
	if err != nil {
		return err
	}
	var tty *bufio.Reader
	if e.TTY != nil {
		if r, err := e.TTY(); err == nil {
			defer r.Close()
			tty = bufio.NewReader(r)
		}
	}
	marked, moved, asked := 0, 0, 0
	for _, a := range actions {
		who := fmt.Sprintf("%s (%s)", queue.Label(a.ID), a.Dir)
		if a.Mark {
			if dry {
				fmt.Fprintf(e.Stdout, "would mark %s ended\n", who)
			} else if err := p.MarkEnded(a.ID); err == nil {
				fmt.Fprintf(e.Stdout, "marked %s ended\n", who)
			}
			marked++
		}
		if a.Items == 0 {
			continue
		}
		if dry {
			fmt.Fprintf(e.Stdout, "would ask: move %d item(s) from %s to the inbox\n", a.Items, who)
			asked += a.Items
			continue
		}
		answer := "n"
		switch {
		case keep:
			fmt.Fprintf(e.Stdout, "kept %d item(s) on %s\n", a.Items, who)
		case yes:
			answer = "y"
		case tty != nil:
			fmt.Fprintf(e.Stderr, "Move %d item(s) from %s to the inbox? [y/N] ", a.Items, who)
			line, err := tty.ReadString('\n')
			if err != nil && line == "" {
				line = "n"
			}
			answer = strings.TrimSpace(line)
		default:
			fmt.Fprintf(e.Stderr, "%d item(s) left on %s: run prune in a terminal, or use -y\n", a.Items, who)
		}
		switch answer {
		case "y", "Y", "yes":
			for i := 0; i < a.Items; i++ {
				if _, err := e.queue().Move(a.ID, state.Inbox, 1, nil); err != nil {
					break
				}
				moved++
			}
		}
	}
	if dry {
		fmt.Fprintf(e.Stdout, "would mark %d ended, would ask about %d item(s)\n", marked, asked)
	} else {
		fmt.Fprintf(e.Stdout, "%d marked ended, %d item(s) moved to the inbox\n", marked, moved)
	}
	return nil
}
