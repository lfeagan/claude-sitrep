// Package runner is the one place sitrep starts other programs (ps, tmux, cmux, claude), so tests can replace them.
package runner

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Runner runs a program and returns its standard output.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Exec runs the real program.
type Exec struct{}

func (Exec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second // a grandchild holding the pipe must not outlive the deadline
	return cmd.Output()
}

// Fake answers from a table keyed by the command line, for tests.
type Fake map[string]Result

// Result is one canned answer.
type Result struct {
	Out []byte
	Err error
}

// Key is the command line a Fake is keyed by: the name and the arguments, joined by single spaces.
func Key(name string, args ...string) string {
	k := name
	for _, a := range args {
		k += " " + a
	}
	return k
}

func (f Fake) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r, ok := f[Key(name, args...)]
	if !ok {
		return nil, fmt.Errorf("runner: no answer for %q", Key(name, args...))
	}
	return r.Out, r.Err
}
