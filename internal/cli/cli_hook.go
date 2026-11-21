package cli

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/hook"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statusline"
)

func (e Env) hook() hook.Hook {
	return hook.Hook{Store: e.store(), ConfigDir: e.configDir(), Getenv: e.Getenv, Clock: e.Clock, Out: e.Stdout, Spawn: e.Spawn}
}

// cmdHook handles one hook event from standard input. It never fails: whatever happens, the exit status is 0 and the prompt is not held.
func cmdHook(e Env, args []string) error {
	if e.Getenv("SITREP_SUMMARIZING") != "" {
		return nil // the summarizer's own claude -p fires hooks; this ends the loop
	}
	if e.Stdin != nil && e.Spawn != nil {
		e.hook().Run(e.Stdin)
	}
	return nil
}

// cmdHookSummarize is the detached child the hook starts: sitrep hook-summarize [--wait seconds --nonce n] [--since count] <session id> <transcript>.
func cmdHookSummarize(e Env, args []string) error {
	var o hook.Opts
	for len(args) >= 2 && (args[0] == "--wait" || args[0] == "--since" || args[0] == "--nonce") {
		switch args[0] {
		case "--wait":
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 0 {
				return nil
			}
			o.Wait = n
		case "--nonce":
			o.Nonce = args[1]
		case "--since":
			o.Since = args[1]
		}
		args = args[2:]
	}
	if len(args) != 2 {
		return nil
	}
	z := hook.Summarizer{Hook: e.hook(), Run: e.Runner, Sleep: time.Sleep, Env: os.Environ()}
	if n, err := strconv.Atoi(e.Getenv("SITREP_SUMMARIZE_TIMEOUT")); err == nil && n > 0 { // for tests, so that no stub outlives them
		z.Limit = time.Duration(n) * time.Second
	}
	// a kill of this child must reach the claude it started, which has its own process group
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer stop()
	z.Do(ctx, state.ID(args[0]), args[1], o)
	return nil
}

// cmdStatusline draws the status line for the session Claude Code describes on standard input.
func cmdStatusline(e Env, args []string) error {
	if e.Getenv("SITREP_RENDERING") != "" {
		return nil // never render ourselves, even if the upstream command points back here
	}
	if e.Stdin == nil {
		return nil
	}
	statusline.Line{Store: e.store(), Conf: e.conf(), Run: e.Runner, Env: os.Environ()}.Render(context.Background(), e.Stdin, e.Stdout)
	return nil
}

// spawnSelf starts this program again with args, detached: a new session, no standard streams, and not waited for.
func spawnSelf(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
