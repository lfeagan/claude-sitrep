package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvSwitches(t *testing.T) {
	for v, c := range map[string]struct{ on, off bool }{"": {false, false}, "0": {false, true}, "false": {false, true}, "FALSE": {false, true}, "no": {false, true}, "1": {true, false}, "yes": {true, false}, "on": {true, false}} {
		get := func(string) string { return v }
		if got := envOn(get, "X"); got != c.on {
			t.Errorf("envOn(%q) = %v", v, got)
		}
		if got := envOff(get, "X"); got != c.off {
			t.Errorf("envOff(%q) = %v", v, got)
		}
	}
}

func TestStatusPrintsAFrame(t *testing.T) {
	e, out, errb, _ := pruneEnv(t, "none")
	if code := Run([]string{"status"}, e); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if !strings.Contains(out.String(), "live0001") && !strings.Contains(out.String(), "live") {
		t.Errorf("%q", out)
	}
	out.Reset()
	if code := Run([]string{"status", "-a", "--tab", "table", "--width", "60"}, e); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if !strings.Contains(out.String(), "done") {
		t.Errorf("ended sessions are shown with -a: %q", out)
	}
}

func TestWatchWithoutATerminalPrintsOneFrame(t *testing.T) {
	e, out, errb, _ := pruneEnv(t, "none")
	e.Getenv = func(k string) string { return map[string]string{"TERM": "xterm-256color"}[k] }
	e.TermSize = func() (int, int, bool) { return 0, 0, false } // standard output is a pipe
	if code := Run([]string{"watch"}, e); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if out.Len() == 0 || strings.Contains(out.String(), "\x1b[") {
		t.Errorf("%q", out)
	}
	for _, env := range []map[string]string{{"TERM": "dumb"}, {}} {
		out.Reset()
		e.Getenv = func(k string) string { return env[k] }
		e.TermSize = func() (int, int, bool) { return 24, 80, true }
		if code := Run([]string{"watch"}, e); code != 0 || out.Len() == 0 {
			t.Errorf("TERM %v: exit %d, %q", env, code, out)
		}
	}
}

func TestWatchAndStatusFlagErrors(t *testing.T) {
	e, _, errb, _ := pruneEnv(t, "none")
	for _, args := range [][]string{{"watch", "--bogus"}, {"watch", "--interval", "x"}, {"watch", "extra"}, {"status", "--width"}, {"status", "x"}} {
		errb.Reset()
		if code := Run(args, e); code != 2 || errb.Len() == 0 {
			t.Errorf("%v: exit %d, %q", args, code, errb)
		}
	}
	errb.Reset()
	if code := Run([]string{"status", "--tab", "nowhere"}, e); code != 1 || !strings.Contains(errb.String(), `unknown tab "nowhere"`) {
		t.Errorf("exit %d: %q", code, errb)
	}
	errb.Reset()
	if code := Run([]string{"watch", "--tab", "table", "-h"}, e); code != 0 {
		t.Errorf("-h exits %d", code)
	}
}

func TestHelpForTheNewCommands(t *testing.T) {
	e, out, _, _ := pruneEnv(t, "none")
	for _, c := range []string{"watch", "status", "setup", "hook", "statusline"} {
		out.Reset()
		if code := Run([]string{c, "--help"}, e); code != 0 || !strings.HasPrefix(out.String(), "usage: sitrep "+c) {
			t.Errorf("%s: %d %q", c, code, out)
		}
	}
	out.Reset()
	Run([]string{"help"}, e)
	for _, c := range []string{"watch ", "status ", "setup ", "ls ", "prune "} {
		if !strings.Contains(out.String(), "  "+c) {
			t.Errorf("help lists no %q", c)
		}
	}
}

// The dashboard acts through the same commands, in this process, with no terminal to ask on.
func TestTheEngineRunsFixedCommandsInProcess(t *testing.T) {
	e, _, _, dir := pruneEnv(t, "y\n")
	eng := e.tuiEngine()
	out, err := eng.Command(context.Background(), "prune", "-y")
	if err != nil || !strings.Contains(out, "2 item(s) moved to the inbox") {
		t.Fatalf("%q, %v", out, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "inbox.queue")); string(b) != "one\ntwo\n" {
		t.Errorf("inbox %q", b)
	}
	// a failure comes back as its message, without the program name
	_, err = eng.Command(context.Background(), "rm", "inbox", "9", "--item", "x")
	if err == nil || err.Error() != "no item 9 on inbox" {
		t.Errorf("error %v", err)
	}
	_, err = eng.Command(context.Background(), "rm", "inbox", "1", "--item", "wrong")
	if err == nil || err.Error() != "queue changed; run 'sitrep ls inbox'" {
		t.Errorf("error %v", err)
	}
	// no question is ever asked on a terminal
	var asked bool
	e.TTY = func() (io.ReadCloser, error) { asked = true; return io.NopCloser(strings.NewReader("y\n")), nil }
	_, _ = e.tuiEngine().Command(context.Background(), "prune")
	if asked {
		t.Error("the engine opened the terminal")
	}
}

func TestTheEngineSnapshotHasEverything(t *testing.T) {
	e, _, _, _ := pruneEnv(t, "none")
	snap, err := e.tuiEngine().Snapshot(context.Background())
	if err != nil || len(snap.Sessions) != 2 {
		t.Fatalf("%d sessions, %v", len(snap.Sessions), err)
	}
}
