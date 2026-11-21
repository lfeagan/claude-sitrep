// Package runner is the one place sitrep starts other programs (ps, tmux, cmux, claude), so tests can replace them.
package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Cmd is one program to run.
type Cmd struct {
	Name  string
	Args  []string
	Dir   string    // working directory; empty means the caller's
	Env   []string  // the whole environment; nil means the caller's
	Stdin io.Reader // nil means none
}

// Runner runs a program and returns its standard output.
type Runner interface {
	Run(ctx context.Context, c Cmd) ([]byte, error)
}

// Exec runs the real program.
type Exec struct{}

func (Exec) Run(ctx context.Context, c Cmd) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir, cmd.Env, cmd.Stdin = c.Dir, c.Env, c.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // its own group, so the deadline can end the whole tree
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second // a grandchild holding the pipe must not outlive the deadline
	return cmd.Output()
}

// Result is one canned answer.
type Result struct {
	Out []byte
	Err error
}

// Fake answers from a table keyed by the command line, and records what it was asked, for tests.
type Fake struct {
	Answers map[string]Result

	mu    sync.Mutex
	calls []Cmd
}

// Key is the command line a Fake is keyed by: the name and the arguments, joined by single spaces.
func Key(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *Fake) Run(_ context.Context, c Cmd) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	r, ok := f.Answers[Key(c.Name, c.Args...)]
	if !ok {
		return nil, fmt.Errorf("runner: no answer for %q", Key(c.Name, c.Args...))
	}
	return r.Out, r.Err
}

// Calls lists what was run, in order.
func (f *Fake) Calls() []Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Cmd(nil), f.calls...)
}
