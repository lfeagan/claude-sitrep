// Package cli is the sitrep command line: it parses arguments, runs the commands and prints their answers.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/notes"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/version"
)

// Env is everything a command touches outside its arguments, so tests can replace it.
type Env struct {
	Home       string
	Getenv     func(string) string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Clock      clock.Clock
	Runner     runner.Runner
	TTY        func() (io.ReadCloser, error)    // the controlling terminal, for questions; nil or an error means there is none
	Executable func() (string, error)           // the path of this program
	Spawn      func(args ...string) error       // starts this program again, detached; the hook uses it for the summarizer
	Alive      func(pid int) bool               // whether a process exists; nil means ask the system
	TermSize   func() (rows, cols int, ok bool) // the size of the terminal on standard output; nil or !ok means there is none
}

// OSEnv is the real process environment.
func OSEnv() Env {
	home, _ := os.UserHomeDir()
	return Env{Home: home, Getenv: os.Getenv, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Clock: clock.Real{}, Runner: runner.Exec{}, TTY: openTTY, TermSize: termSize, Spawn: spawnSelf, Executable: os.Executable}
}

func termSize() (int, int, bool) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	return h, w, err == nil
}

func openTTY() (io.ReadCloser, error) { return os.Open("/dev/tty") }

func (e Env) configDir() string { return state.ConfigDir(e.Getenv, e.Home) }

func (e Env) store() state.Store { return state.Open(e.Home) }
func (e Env) queue() queue.Queue { return queue.Queue{Store: e.store()} }
func (e Env) notes() notes.Notes { return notes.Notes{Store: e.store()} }

// usageError is a wrong command line; its text is the usage line.
type usageError string

func (u usageError) Error() string { return string(u) }

// exitError is a command that has already printed its own messages and only needs an exit status.
type exitError int

func (e exitError) Error() string { return "exit status " + strconv.Itoa(int(e)) }

// command runs with the arguments after its name.
type command func(e Env, args []string) error

var commands map[string]command

func init() {
	commands = map[string]command{
		"add": cmdAdd, "edit": cmdEdit, "rm": cmdRm, "top": cmdTop, "reorder": cmdReorder, "move": cmdMove, "pin": cmdPin, "unpin": cmdUnpin, "ls": cmdLs, "snooze": cmdSnooze, "prune": cmdPrune,
		"setup": cmdSetup, "migrate": cmdMigrate, "version": cmdVersion, "completion": cmdCompletion, "__complete": cmdComplete, "watch": cmdWatch, "status": cmdStatus, "hook": cmdHook, "statusline": cmdStatusline, "hook-summarize": cmdHookSummarize, "update-check": cmdUpdateCheck, "help": cmdHelp, "-h": cmdHelp, "--help": cmdHelp,
	}
}

// Run runs one command line (without the program name) and returns the exit status.
func Run(args []string, e Env) int {
	if !filepath.IsAbs(e.Home) {
		return e.die(errors.New("cannot find your home folder"))
	}
	cmd := "ls"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
		if cmd == "" {
			cmd = "ls" // an empty word is no command, as in the shell's ${1:-ls}
		}
	}
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") && cmd != "help" && cmd != "-h" && cmd != "--help" {
		text, ok := help[cmd]
		if !ok {
			return e.die(fmt.Errorf("unknown command '%s'; run 'sitrep help'", textsafe.Clean(cmd, 200)))
		}
		fmt.Fprint(e.Stdout, text)
		return 0
	}
	run, ok := commands[cmd]
	if !ok {
		return e.die(fmt.Errorf("unknown command '%s'; run 'sitrep help'", textsafe.Clean(cmd, 200)))
	}
	if err := run(e, args); err != nil {
		var code exitError
		if errors.As(err, &code) {
			return int(code) // the command has said what there was to say
		}
		return e.die(err)
	}
	return 0
}

// die prints `sitrep: <message>` to standard error and returns the exit status 1.
func (e Env) die(err error) int {
	fmt.Fprintf(e.Stderr, "sitrep: %s\n", textsafe.Strip(err.Error()))
	return 1
}

// itemNo reads a 1-based item number. A number too big for an int is a number that matches no item.
func itemNo(s string) (int, error) {
	if s == "" || s[0] == '0' {
		return 0, errors.New("item must be a number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("item must be a number")
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return int(^uint(0) >> 1), nil
	}
	return n, nil
}

// guarded splits a trailing `--item <text>` off the arguments. A second --item, or one anywhere else, is an error.
func guarded(args []string) (rest []string, guard *string, err error) {
	rest = args
	if n := len(args); n >= 2 && args[n-2] == "--item" {
		g := args[n-1]
		guard, rest = &g, args[:n-2]
	}
	for _, a := range rest {
		if a == "--item" {
			return nil, nil, errors.New("--item needs the text of the item, last")
		}
	}
	return rest, guard, nil
}

func (e Env) resolve(prefix string) (state.ID, error) { return e.store().Resolve(prefix) }

func cmdAdd(e Env, args []string) error {
	if len(args) < 2 {
		return usageError("usage: sitrep add <id> <text>")
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	n, err := e.queue().Add(id, textsafe.Clean(strings.Join(args[1:], " "), queue.MaxItem))
	if err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "queued on %s (now %d)\n", queue.Label(id), n)
	return nil
}

func cmdRm(e Env, args []string) error {
	const usage = "usage: sitrep rm <id> [item=1] [--item <text>]"
	args, guard, err := guarded(args)
	if err != nil {
		return err
	}
	if len(args) < 1 {
		return usageError(usage)
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	n := "1"
	if len(args) > 1 {
		n = args[1]
	}
	no, err := itemNo(n)
	if err != nil {
		return err
	}
	item, err := e.queue().Remove(id, no, guard)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.Stdout, item)
	return nil
}

func cmdEdit(e Env, args []string) error {
	const usage = "usage: sitrep edit <id> <n> <text> [--item <old text>]"
	args, guard, err := guarded(args)
	if err != nil {
		return err
	}
	if len(args) < 3 {
		return usageError(usage)
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	no, err := itemNo(args[1])
	if err != nil {
		return err
	}
	text := textsafe.Clean(strings.Join(args[2:], " "), queue.MaxItem)
	if _, err := e.queue().Edit(id, no, text, guard); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "item %d on %s: %s\n", no, queue.Label(id), text)
	return nil
}

func cmdTop(e Env, args []string) error {
	args, guard, err := guarded(args)
	if err != nil {
		return err
	}
	if len(args) < 2 {
		return usageError("usage: sitrep top <id> <n> [--item <text>]")
	}
	return e.reorder(args[0], args[1], "1", guard)
}

func cmdReorder(e Env, args []string) error {
	args, guard, err := guarded(args)
	if err != nil {
		return err
	}
	if len(args) < 3 {
		return usageError("usage: sitrep reorder <id> <from> <to> [--item <text>]")
	}
	return e.reorder(args[0], args[1], args[2], guard)
}

func (e Env) reorder(prefix, from, to string, guard *string) error {
	id, err := e.resolve(prefix)
	if err != nil {
		return err
	}
	f, err := itemNo(from)
	if err != nil {
		return err
	}
	t, err := itemNo(to)
	if err != nil {
		return err
	}
	item, err := e.queue().Reorder(id, f, t, guard)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "item %d on %s: %s\n", t, queue.Label(id), item)
	return nil
}

func cmdMove(e Env, args []string) error {
	const usage = "usage: sitrep move <from> <to> [item=1] [--item <text>]"
	if len(args) < 2 {
		return usageError(usage)
	}
	from, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	to, err := e.resolve(args[1])
	if err != nil {
		return err
	}
	rest := args[2:]
	n := "1"
	if len(rest) > 0 && isDigits(rest[0]) {
		n, rest = rest[0], rest[1:]
	}
	var guard *string
	if len(rest) > 0 && rest[0] == "--item" {
		if len(rest) != 2 {
			return usageError(usage)
		}
		g := rest[1]
		guard, rest = &g, nil
	}
	if len(rest) != 0 {
		return usageError(usage)
	}
	no, err := itemNo(n)
	if err != nil {
		return err
	}
	if from == to {
		return errors.New("from and to are the same queue")
	}
	item, err := e.queue().Move(from, to, no, guard)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "moved to %s: %s\n", queue.Label(to), item)
	return nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func cmdPin(e Env, args []string) error {
	if len(args) < 2 {
		return usageError("usage: sitrep pin <id> <text>")
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	if id == state.Inbox {
		return errors.New("inbox has no objective")
	}
	if err := e.notes().Pin(id, textsafe.Clean(strings.Join(args[1:], " "), queue.MaxItem)); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "objective set on %s\n", queue.Label(id))
	return nil
}

func cmdUnpin(e Env, args []string) error {
	if len(args) < 1 {
		return usageError("usage: sitrep unpin <id>")
	}
	id, err := e.resolve(args[0])
	if err != nil {
		return err
	}
	if id == state.Inbox {
		return errors.New("inbox has no objective")
	}
	if err := e.notes().Unpin(id); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, "objective on %s will be summarized again\n", queue.Label(id))
	return nil
}

func cmdHelp(e Env, args []string) error {
	switch {
	case len(args) == 0:
		fmt.Fprint(e.Stdout, helpAll)
	case args[0] == "-h" || args[0] == "--help":
		fmt.Fprint(e.Stdout, help["help"])
	default:
		text, ok := help[args[0]]
		if !ok {
			return fmt.Errorf("no help for '%s'; run 'sitrep help'", textsafe.Clean(args[0], 200))
		}
		fmt.Fprint(e.Stdout, text)
	}
	return nil
}

func cmdVersion(e Env, args []string) error {
	fmt.Fprintln(e.Stdout, version.Version)
	return nil
}
