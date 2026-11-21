package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/tui"
)

// envOn says whether a switch in the environment is on: set, and not 0, false or no.
func envOn(get func(string) string, name string) bool {
	switch strings.ToLower(get(name)) {
	case "", "0", "false", "no":
		return false
	}
	return true
}

// envOff says whether a switch in the environment is explicitly off: 0, false or no.
func envOff(get func(string) string, name string) bool {
	switch strings.ToLower(get(name)) {
	case "0", "false", "no":
		return true
	}
	return false
}

// tuiEngine gives the dashboard its data and its fixed commands, in this process: a snapshot is built directly, and a command is run by
// the same code `sitrep` runs, with its own buffers and no terminal to ask questions on.
func (e Env) tuiEngine() tui.Engine {
	return tui.Engine{
		Snapshot: func(ctx context.Context) (snapshot.Snapshot, error) {
			return e.builder().Build(ctx, snapshot.Options{All: true, Cap: -1}), nil
		},
		Command: func(ctx context.Context, args ...string) (string, error) {
			var out, errb bytes.Buffer
			c := e
			c.Stdout, c.Stderr, c.Stdin, c.TTY = &out, &errb, nil, nil
			if code := Run(args, c); code != 0 {
				line, _, _ := strings.Cut(errb.String(), "\n")
				if line = strings.TrimSpace(strings.TrimPrefix(line, "sitrep: ")); line == "" {
					line = fmt.Sprintf("failed with status %d", code)
				}
				return "", errors.New(line)
			}
			return out.String(), nil
		},
	}
}

// flags makes a flag set that reports to standard error. An unknown flag or a bad value is exit status 2.
func (e Env) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("sitrep "+name, flag.ContinueOnError)
	fs.SetOutput(e.Stderr)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitError(0) // the usage was printed: nothing more to do
		}
		return exitError(2)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "%s: unexpected argument %q\n", fs.Name(), fs.Arg(0))
		return exitError(2)
	}
	return nil
}

// cmdWatch is the live dashboard. Without a terminal it prints one frame, as `status` does, never a loop of escape codes.
func cmdWatch(e Env, args []string) error {
	fs := e.flags("watch")
	tab := fs.String("tab", "watch", "the tab to start on: watch or top")
	interval := fs.Duration("interval", 3*time.Second, "how often to refresh")
	once := fs.Bool("once", false, "print one frame as plain text and exit")
	readOnly := fs.Bool("read-only", envOn(e.Getenv, "SITREP_TUI_READONLY"), "turn off every key that changes state (or set SITREP_TUI_READONLY)")
	noBell := fs.Bool("no-bell", envOff(e.Getenv, "SITREP_TUI_BELL"), "do not ring the bell when a session starts needing you (or set SITREP_TUI_BELL=0)")
	quitOnJump := fs.Bool("quit-on-jump", false, "leave the dashboard after a good jump, for a tmux popup")
	width := fs.Int("width", 100, "the frame width for --once")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *once || !e.isTerminal() {
		return e.printFrame(*tab, *width, false)
	}
	err := tui.Run(tui.Options{Tab: *tab, Interval: *interval, ReadOnly: *readOnly, NoBell: *noBell, QuitOnJump: *quitOnJump}, e.tuiEngine())
	if err != nil {
		return err
	}
	return nil
}

// cmdStatus prints one frame of the dashboard as plain text.
func cmdStatus(e Env, args []string) error {
	fs := e.flags("status")
	tab := fs.String("tab", "watch", "the tab: watch or top")
	width := fs.Int("width", 100, "the frame width")
	all := fs.Bool("a", false, "include ended sessions")
	fs.BoolVar(all, "all", false, "include ended sessions")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	return e.printFrame(*tab, *width, *all)
}

func (e Env) printFrame(tab string, width int, all bool) error {
	frame, err := tui.Frame(context.Background(), e.tuiEngine(), tab, width, all)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, frame)
	return nil
}

// isTerminal says whether standard output is a terminal that can take cursor movement.
func (e Env) isTerminal() bool {
	if t := e.Getenv("TERM"); t == "" || t == "dumb" {
		return false
	}
	if e.TermSize == nil {
		return false
	}
	_, _, ok := e.TermSize()
	return ok
}
