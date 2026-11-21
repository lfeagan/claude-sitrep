package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func osascriptCalls(f *fake) []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "osascript ") {
			out = append(out, c)
		}
	}
	return out
}

func TestJumpITermAndTerminal(t *testing.T) {
	sess := Session{ID: "abcdef123456", Short: "abcd", PID: pidOf(70)}
	for _, tc := range []struct {
		name   string
		env    jumpEnv
		bundle string
	}{
		{"iterm", jumpEnv{ITerm: true}, "com.googlecode.iterm2"},
		{"terminal", jumpEnv{Terminal: true}, "com.apple.Terminal"},
	} {
		f := &fake{answer: map[string]string{"ps -A": "50 1 -zsh\n60 50 -zsh\n70 60 claude\n", "ps -o tty= -p 70": "ttys003\n", "osascript": "found\n"}}
		msg, err := jump(context.Background(), sess, tc.env, f.run)
		if err != nil || msg != "jumped to abcd" {
			t.Fatalf("%s: %q %v", tc.name, msg, err)
		}
		calls := osascriptCalls(f)
		if len(calls) != 1 || !strings.Contains(calls[0], tc.bundle) || !strings.HasSuffix(calls[0], "end run /dev/ttys003") {
			t.Errorf("%s: the script gets the tty as an argument: %q", tc.name, calls)
		}
		if strings.Contains(strings.Split(calls[0], "end run")[0], "ttys003") {
			t.Errorf("%s: the tty is in the script text", tc.name)
		}
	}
}

func TestJumpAppNotFound(t *testing.T) {
	sess := Session{ID: "abcdef123456", Short: "abcd", PID: pidOf(70)}
	f := &fake{answer: map[string]string{"ps -A": "70 1 claude\n", "ps -o tty= -p 70": "ttys003\n"}} // osascript answers nothing
	if _, err := jump(context.Background(), sess, jumpEnv{ITerm: true}, f.run); err == nil || !strings.Contains(err.Error(), "not in a pane here") {
		t.Fatalf("an application that does not show it is no jump: %v", err)
	}
}

func TestJumpAppOrder(t *testing.T) {
	sess := Session{ID: "abcdef123456", Short: "abcd", PID: pidOf(70)}
	f := &fake{answer: map[string]string{
		"tmux list-panes": "%2 60 /dev/ttys002 $1 @4\n", "ps -A": "60 1 -zsh\n70 60 claude\n", "ps -o tty= -p 70": "ttys003\n", "osascript": "found\n"}}
	if _, err := jump(context.Background(), sess, jumpEnv{Tmux: true, ITerm: true}, f.run); err != nil {
		t.Fatal(err)
	}
	if len(osascriptCalls(f)) != 0 {
		t.Error("a pane inside tmux wins over the application around it")
	}
	// tmux does not show it, so the application is asked
	f = &fake{answer: map[string]string{
		"tmux list-panes": "%9 5 /dev/ttys009 $1 @5\n", "ps -A": "60 1 -zsh\n70 60 claude\n", "ps -o tty= -p 70": "ttys003\n", "osascript": "found\n"}}
	if _, err := jump(context.Background(), sess, jumpEnv{Tmux: true, ITerm: true, Terminal: true}, f.run); err != nil {
		t.Fatal(err)
	}
	if got := osascriptCalls(f); len(got) != 1 || !strings.Contains(got[0], "com.googlecode.iterm2") {
		t.Errorf("iTerm2 is asked before Terminal, and Terminal not once it answers: %q", got)
	}
}

func TestJumpAppNoTTY(t *testing.T) {
	sess := Session{ID: "abcdef123456", Short: "abcd", PID: pidOf(70)}
	for _, tty := range []string{"??\n", "\n", "ttys003; do shell script \"x\"\n", "pts/3\n", "../x\n"} {
		f := &fake{answer: map[string]string{"ps -A": "70 1 claude\n", "ps -o tty= -p 70": tty, "osascript": "found\n"}}
		if _, err := jump(context.Background(), sess, jumpEnv{ITerm: true}, f.run); err == nil {
			t.Errorf("tty %q jumped", tty)
		}
		if len(osascriptCalls(f)) != 0 {
			t.Errorf("tty %q reached osascript", tty)
		}
	}
}

func TestJumpAppAttachedClient(t *testing.T) {
	sess := Session{ID: "93a01840aaaa", Short: "93a0", PID: pidOf(70)}
	f := &fake{answer: map[string]string{
		"ps -A":            "50 1 -zsh\n70 1 claude bg\n90 50 claude attach 93a01840\n",
		"ps -o tty= -p 70": "??\n", "ps -o tty= -p 90": "ttys005\n", "osascript": "found\n"}}
	if _, err := jump(context.Background(), sess, jumpEnv{Terminal: true}, f.run); err != nil {
		t.Fatal(err)
	}
	if got := osascriptCalls(f); len(got) != 1 || !strings.HasSuffix(got[0], "end run /dev/ttys005") {
		t.Errorf("the client's terminal is the one asked for: %q", got)
	}
}

func TestJumpAppDenied(t *testing.T) {
	sess := Session{ID: "abcdef123456", Short: "abcd", PID: pidOf(70)}
	f := &fake{answer: map[string]string{"ps -A": "70 1 claude\n", "ps -o tty= -p 70": "ttys003\n"},
		fail: map[string]error{"osascript": errors.New("osascript: execution error: Not authorized to send Apple events to iTerm2. (-1743)")}}
	_, err := jump(context.Background(), sess, jumpEnv{ITerm: true}, f.run)
	if err == nil || !strings.Contains(err.Error(), "System Settings > Privacy & Security > Automation") || !strings.Contains(err.Error(), "iTerm2") {
		t.Fatalf("a refusal says what to allow: %v", err)
	}
	f.fail = map[string]error{"osascript": errors.New("osascript: boom")}
	if _, err := jump(context.Background(), sess, jumpEnv{ITerm: true}, f.run); err == nil || !strings.Contains(err.Error(), "could not ask iTerm2") {
		t.Fatalf("another failure: %v", err)
	}
}

func TestCurrentEnvDetectsTerminalApps(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want jumpEnv
	}{
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, jumpEnv{ITerm: true}},
		{map[string]string{"TERM_PROGRAM": "tmux", "ITERM_SESSION_ID": "w0t0p0:ABC", "TMUX": "/tmp/x"}, jumpEnv{Tmux: true, ITerm: true}},
		{map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, jumpEnv{Terminal: true}},
		{map[string]string{"TERM_PROGRAM": "ghostty"}, jumpEnv{}},
	} {
		for _, k := range []string{"TERM_PROGRAM", "ITERM_SESSION_ID", "TMUX", "CMUX_SURFACE_ID", "CMUX_WORKSPACE_ID"} {
			t.Setenv(k, tc.env[k])
		}
		if got := currentEnv(); got != tc.want {
			t.Errorf("%v: %+v, want %+v", tc.env, got, tc.want)
		}
	}
}
