package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/clock"
)

// runWith runs a command line with SITREP_STATE_DIR set to dir, and with stdin as the standard input.
func runWith(t *testing.T, home, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	e := Env{
		Home:   home,
		Getenv: func(k string) string { return map[string]string{"SITREP_STATE_DIR": dir}[k] },
		Stdin:  strings.NewReader(stdin),
		Stdout: &out, Stderr: &errb, Clock: clock.Real{},
		Spawn: func(...string) error { return nil },
	}
	code := Run(args, e)
	return code, out.String(), errb.String()
}

func TestStateDirMovesTheState(t *testing.T) {
	home, dir := t.TempDir(), filepath.Join(t.TempDir(), "demo")
	if code, out, errs := runWith(t, home, dir, "", "add", "inbox", "in the other folder"); code != 0 {
		t.Fatalf("add: %d %q %q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.queue")); err != nil {
		t.Errorf("the inbox is not in the state folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("the default folder was touched: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("the folder must be owner-only: %v %v", fi, err)
	}
	if code, out, _ := runWith(t, home, dir, "", "ls", "inbox"); code != 0 || !strings.Contains(out, "in the other folder") {
		t.Errorf("ls: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "ls", "inbox"); code != 0 || strings.Contains(out, "in the other folder") {
		t.Errorf("the default folder must not see it: %d %q", code, out)
	}
}

func TestStateDirTakesTheHooksState(t *testing.T) {
	home, dir := t.TempDir(), filepath.Join(t.TempDir(), "demo")
	ev := `{"session_id":"sess0001","hook_event_name":"UserPromptSubmit","cwd":"/tmp/proj","prompt":"fix the flaky test"}`
	if code, out, errs := runWith(t, home, dir, ev, "hook"); code != 0 || out != "" || errs != "" {
		t.Fatalf("hook: %d %q %q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "sessions", "sess0001")); err != nil {
		t.Errorf("the session is not in the state folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("the default folder was touched: %v", err)
	}
}

func TestABadStateDirNeverFailsAHook(t *testing.T) {
	home := t.TempDir()
	ev := `{"session_id":"sess0001","hook_event_name":"UserPromptSubmit","cwd":"/tmp/proj","prompt":"hello"}`
	for _, args := range [][]string{{"hook"}, {"hook-summarize", "sess0001", "/x"}, {"update-check"}, {"__complete", "add", ""}} {
		if code, out, errs := runWith(t, home, "relative/state", ev, args...); code != 0 || out != "" || errs != "" {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("a bad value must not fall back to the default folder: %v", err)
	}
}

func TestABadStateDirSaysSoOnTheStatusLineAndInCommands(t *testing.T) {
	home := t.TempDir()
	code, out, _ := runWith(t, home, "~/state", "{}", "statusline")
	if code != 0 || !strings.Contains(out, "SITREP_STATE_DIR must be an absolute path") {
		t.Errorf("statusline: %d %q", code, out)
	}
	for _, args := range [][]string{{"ls"}, {"add", "inbox", "x"}, {"dash"}, {"prune"}} {
		code, out, errs := runWith(t, home, "state\x1b[31m", "", args...)
		if code != 1 || out != "" || !strings.Contains(errs, "SITREP_STATE_DIR must be an absolute path") || strings.Contains(errs, "\x1b") {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	for _, args := range [][]string{{"version"}, {"help"}, {"completion", "bash"}} {
		if code, _, errs := runWith(t, home, "relative", "", args...); code != 0 || errs != "" {
			t.Errorf("%v needs no state: %d %q", args, code, errs)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("a bad value must not fall back to the default folder: %v", err)
	}
}

func TestSetupAndMigrateRefuseWhenTheStateMoved(t *testing.T) {
	home, dir := t.TempDir(), filepath.Join(t.TempDir(), "demo")
	for _, args := range [][]string{{"setup"}, {"setup", "--remove"}, {"migrate"}, {"migrate", "--dry-run"}} {
		code, out, errs := runWith(t, home, dir, "", args...)
		if code != 1 || out != "" || !strings.Contains(errs, "SITREP_STATE_DIR is set") || !strings.Contains(errs, "nothing was changed") {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
	for _, p := range []string{filepath.Join(home, ".claude"), dir} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s was made: %v", p, err)
		}
	}
}
