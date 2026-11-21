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
	"github.com/lfeagan/claude-sitrep/internal/migrate"
	"github.com/lfeagan/claude-sitrep/internal/prune"
	"github.com/lfeagan/claude-sitrep/internal/render"
	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
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
	var all, asJSON, full, verbose bool
loop:
	for len(args) > 0 {
		switch args[0] {
		case "-v":
			verbose, args = true, args[1:]
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
			items, args, verbose = args[1], args[2:], true
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
	n, _ := strconv.Atoi(items) // all digits, so only a number too big for an int fails; it then lists everything
	if n == 0 && items != "0" {
		n = int(^uint(0) >> 1)
	}
	if !asJSON {
		if len(args) > 0 {
			return e.lsQueue(args[0])
		}
		o := render.Opts{Verbose: verbose, Items: n, Cols: e.cols()}
		snap := e.builder().Build(context.Background(), snapshot.Options{All: all, Cap: o.Cap()})
		for _, l := range render.Frame(snap, o) {
			fmt.Fprintln(e.Stdout, l)
		}
		if n := migrate.Pending(e.store()); n > 0 {
			fmt.Fprintf(e.Stderr, "%d files of the old state layout are still in ~/.claude/sitrep: run 'sitrep migrate'\n", n)
		}
		return nil
	}
	if len(args) > 0 {
		return usageError("usage: sitrep ls --json [-a] [-n items | --full]")
	}
	cap := n
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
		fmt.Fprintf(e.Stdout, "%s is no longer snoozed\n", e.label(id))
		return nil
	}
	changed, ok := live.Changed(e.store(), string(id))
	if !ok {
		return fmt.Errorf("cannot read when %s last changed", e.label(id))
	}
	if err := e.notes().Snooze(id, changed); err != nil {
		return fmt.Errorf("cannot snooze %s", e.label(id))
	}
	fmt.Fprintf(e.Stdout, "snoozed %s until it changes\n", e.label(id))
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
		who := fmt.Sprintf("%s (%s)", e.label(a.ID), a.Dir)
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
			if err != nil {
				line = "n" // end of input before the end of the line is no answer
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

// cols is the width of the terminal: COLUMNS, then the terminal itself, then 100. Anything below 10 is not a width.
func (e Env) cols() int {
	cols, _ := strconv.Atoi(e.Getenv("COLUMNS"))
	if cols < 1 && e.TermSize != nil {
		if _, c, ok := e.TermSize(); ok {
			cols = c
		}
	}
	if cols < 10 {
		return 100
	}
	return cols
}

// lsQueue lists one queue: the inbox, or a session with its Objective and state, then its items numbered.
func (e Env) lsQueue(prefix string) error {
	id, err := e.resolve(prefix)
	if err != nil {
		return err
	}
	st := e.store()
	if id == state.Inbox {
		fmt.Fprintln(e.Stdout, "Inbox")
	} else {
		now := e.Clock.Now().Unix()
		changed, _ := live.Changed(st, string(id))
		quiet, hasQuiet := int64(0), false
		word := live.Word(st, string(id))
		if word == "working" {
			quiet, hasQuiet = live.Quiet(e.configDir(), string(id), now)
		}
		label := live.Label(word, changed, now, quiet, hasQuiet)
		suffix := ""
		if label != "-" {
			suffix = "  (" + label + ")"
		}
		cwd, _ := state.ReadFile(st.Path(id, "cwd"), 64<<10)
		who := e.label(id)
		if a := st.Alias(id); a != "" {
			who += " (" + string(id[:min(len(id), 4)]) + ")" // the alias, and the id that is its stable name
		}
		fmt.Fprintf(e.Stdout, "Session %s  %s%s\n", who, textsafe.Clean(string(cwd), 200), suffix)
		notes, _ := state.ReadFile(st.Path(id, "notes"), 1<<20)
		for _, l := range strings.Split(string(notes), "\n") {
			if rest, ok := strings.CutPrefix(l, "Objective: "); ok {
				fmt.Fprintln(e.Stdout, "  Objective: "+textsafe.Strip(rest).String())
			}
		}
	}
	b, _ := state.ReadFile(st.Path(id, "queue"), 8<<20)
	if len(b) == 0 {
		fmt.Fprintln(e.Stdout, "  (queue empty)")
		return nil
	}
	n := 0
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" {
			fmt.Fprintln(e.Stdout)
			continue
		}
		n++
		fmt.Fprintf(e.Stdout, "%2d. %s\n", n, textsafe.Strip(l))
	}
	return nil
}
