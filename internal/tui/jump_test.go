package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestParsePanes(t *testing.T) {
	out := strings.Join([]string{
		"%0 100 /dev/ttys001 $0 @0",
		"%12 2045 /dev/ttys004 $3 @7",
		"garbage",
		"%1 notanumber /dev/ttys002 $0 @0",
		"x1 100 /dev/ttys001 $0 @0",   // not a pane id
		"%2 100 /dev/ttys001 main @0", // a name, not a session id
		"%3 100 /dev/ttys001 $0 w1",   // a name, not a window id
		"%4 0 /dev/ttys001 $0 @0",     // pid 0
		"%5 100 /dev/ttys001 $0",      // too few fields
		"",
	}, "\n")
	got := parsePanes(out)
	if len(got) != 2 || got[0].ID != "%0" || got[1].ID != "%12" || got[1].PID != 2045 || got[1].SessionID != "$3" || got[1].WindowID != "@7" {
		t.Errorf("parsePanes kept the wrong lines: %+v", got)
	}
}

func TestParseProcsAndAncestors(t *testing.T) {
	parents := parentMap(parseProcs("  1 0 /sbin/launchd\n 50 1 -zsh\n 60 50 claude --resume abc\n 70 60 sleep 1\n junk\n 80 x y\n"))
	if parents[70] != 60 || parents[60] != 50 || len(parents) != 4 {
		t.Fatalf("parseProcs: %v", parents)
	}
	if got := ancestors(70, parents); !reflect.DeepEqual(got, []int{70, 60, 50}) {
		t.Errorf("the chain stops before process 1: %v", got)
	}
	if got := ancestors(999, parents); !reflect.DeepEqual(got, []int{999}) {
		t.Errorf("an unknown process is its own chain: %v", got)
	}
	loop := map[int]int{10: 20, 20: 30, 30: 10}
	if got := ancestors(10, loop); len(got) != 3 {
		t.Errorf("a loop must end: %v", got)
	}
	deep := map[int]int{}
	for i := 2; i < 500; i++ {
		deep[i] = i - 1
	}
	if got := ancestors(499, deep); len(got) != maxWalk {
		t.Errorf("a deep chain is capped at %d: %d", maxWalk, len(got))
	}
}

func TestFindPane(t *testing.T) {
	panes := []pane{{ID: "%1", PID: 50}, {ID: "%2", PID: 60}, {ID: "%3", PID: 999}}
	if p, ok := findPane(panes, []int{70, 60, 50}); !ok || p.ID != "%2" {
		t.Errorf("the closest ancestor wins: %+v %v", p, ok)
	}
	if _, ok := findPane(panes, []int{70, 71}); ok {
		t.Errorf("no ancestor is a pane process")
	}
	if _, ok := findPane(nil, []int{1}); ok {
		t.Errorf("no panes")
	}
}

func TestTTYFallback(t *testing.T) {
	panes := []pane{{ID: "%1", TTY: "/dev/ttys003"}, {ID: "%2", TTY: "/dev/pts/7"}, {ID: "%3", TTY: "/dev/pts/7"}, {ID: "%4", TTY: ""}}
	if p, ok := paneByTTY(panes, "ttys003"); !ok || p.ID != "%1" {
		t.Errorf("BSD spelling: %+v %v", p, ok)
	}
	if _, ok := paneByTTY(panes, "pts/7"); ok {
		t.Errorf("two panes on one terminal is no answer")
	}
	for _, tty := range []string{"", "?", "??", "-"} {
		if _, ok := paneByTTY(panes, tty); ok {
			t.Errorf("%q names no terminal", tty)
		}
	}
	if normTTY("pts/3") != "/dev/pts/3" || normTTY("/dev/ttys001") != "/dev/ttys001" {
		t.Errorf("normTTY")
	}
}

func TestJumpArgs(t *testing.T) {
	want := []string{"select-window", "-t", "@7", ";", "select-pane", "-t", "%12", ";", "switch-client", "-t", "$3"}
	if got := jumpArgs(pane{ID: "%12", WindowID: "@7", SessionID: "$3"}); !reflect.DeepEqual(got, want) {
		t.Errorf("the one tmux call: %v", got)
	}
}

// fake records every command and answers from a table.
type fake struct {
	calls  []string
	answer map[string]string
	fail   map[string]error
}

func (f *fake) run(_ context.Context, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	for prefix, err := range f.fail {
		if strings.HasPrefix(line, prefix) {
			return "", err
		}
	}
	for prefix, out := range f.answer {
		if strings.HasPrefix(line, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func pidOf(n int64) *int64 { return &n }

func TestJump(t *testing.T) {
	sess := Session{ID: "abc", Short: "abcd", PID: pidOf(70)}
	f := &fake{answer: map[string]string{
		"tmux list-panes": "%2 60 /dev/ttys002 $1 @4\n%9 5 /dev/ttys009 $1 @5\n",
		"ps -A":           "50 1 -zsh\n60 50 -zsh\n70 60 claude\n",
	}}
	msg, err := jump(context.Background(), sess, jumpEnv{Tmux: true}, f.run)
	if err != nil || msg != "jumped to abcd" {
		t.Fatalf("a jump: %q %v", msg, err)
	}
	if last := f.calls[len(f.calls)-1]; last != "tmux select-window -t @4 ; select-pane -t %2 ; switch-client -t $1" {
		t.Errorf("the last call is the one tmux command: %q", last)
	}
	if n := strings.Count(strings.Join(f.calls, "\n"), "tmux select-window"); n != 1 {
		t.Errorf("exactly one jump call: %d", n)
	}

	if _, err := jump(context.Background(), sess, jumpEnv{}, f.run); err == nil || !strings.Contains(err.Error(), "not in tmux or cmux") {
		t.Errorf("outside tmux: %v", err)
	}
	if _, err := jump(context.Background(), Session{Short: "x"}, jumpEnv{Tmux: true}, f.run); err == nil || !strings.Contains(err.Error(), "process") {
		t.Errorf("no pid: %v", err)
	}
	other := &fake{answer: map[string]string{"tmux list-panes": "%9 5 /dev/ttys009 $1 @5\n", "ps -A": "50 1 a\n60 50 b\n70 60 c\n", "ps -o tty=": "ttys002\n"}}
	if _, err := jump(context.Background(), Session{ID: "abcdef0123456789", Short: "abcd", PID: pidOf(70)}, jumpEnv{Tmux: true}, other.run); err == nil || !strings.Contains(err.Error(), "not in a pane") || !strings.Contains(err.Error(), "claude attach abcdef01") {
		t.Errorf("no match says how to open a background session: %v", err)
	}
	if strings.Contains(strings.Join(other.calls, "\n"), "select-window") {
		t.Errorf("no match, no jump")
	}
	viaTTY := &fake{answer: map[string]string{"tmux list-panes": "%9 5 /dev/ttys002 $1 @5\n", "ps -A": "70 1 claude\n", "ps -o tty=": "ttys002\n"}}
	if msg, err := jump(context.Background(), sess, jumpEnv{Tmux: true}, viaTTY.run); err != nil || msg == "" {
		t.Errorf("the tty fallback finds the pane: %q %v", msg, err)
	}
	for _, c := range []string{"tmux list-panes", "ps -A", "tmux select-window"} {
		bad := &fake{answer: f.answer, fail: map[string]error{c: errors.New("boom")}}
		if _, err := jump(context.Background(), sess, jumpEnv{Tmux: true}, bad.run); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Errorf("a failing %q is reported: %v", c, err)
		}
	}
}

// the real runner, against fake tmux and ps scripts on PATH
func TestJumpWithFakeCommands(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("tmux", fmt.Sprintf(`echo "tmux $*" >> %q; case "$1" in list-panes) echo "%%7 60 /dev/ttys005 \$2 @3";; esac`, log))
	write("ps", `echo "50 1 -zsh"; echo "60 50 -zsh"; echo "70 60 claude"`)
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	msg, err := jump(context.Background(), Session{Short: "abcd", PID: pidOf(70)}, jumpEnv{Tmux: true}, realRunner)
	if err != nil || msg != "jumped to abcd" {
		t.Fatalf("%q %v", msg, err)
	}
	raw, _ := os.ReadFile(log)
	want := "tmux list-panes -a -F #{pane_id} #{pane_pid} #{pane_tty} #{session_id} #{window_id}\ntmux select-window -t @3 ; select-pane -t %7 ; switch-client -t $2\n"
	if string(raw) != want {
		t.Errorf("the exact tmux calls:\n%s\nwant\n%s", raw, want)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := realRunner(context.Background(), "tmux", "list-panes"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("a missing tmux says so: %v", err)
	}
}

func TestEnterJumpsOnce(t *testing.T) {
	m := loaded(t, tabWatch)
	m.run = (&fake{}).run
	m2, cmd := press(t, m, "enter")
	if cmd == nil || !m2.acting {
		t.Fatalf("enter starts a jump")
	}
	if _, again := press(t, m2, "enter"); again != nil {
		t.Errorf("a second enter while one runs does nothing")
	}
	if _, none := press(t, newModel("x", time.Second, tabWatch), "enter"); none != nil {
		t.Errorf("nothing selected, nothing to jump to")
	}
	d, _ := press(t, m, "d")
	if _, cmd := press(t, d, "enter"); cmd == nil {
		t.Errorf("enter jumps from the detail pane too")
	}
}

func TestJumpResultIsShownThenExpires(t *testing.T) {
	m := loaded(t, tabWatch)
	now := time.Unix(1000, 0)
	m.now = func() time.Time { return now }
	m.acting = true
	next, _ := m.Update(jumpMsg{text: "jumped to 15f1"})
	m = next.(model)
	if m.acting || m.statusText() != "jumped to 15f1" {
		t.Errorf("the result is shown: %q", m.statusText())
	}
	if v := m.View(); !strings.Contains(v.Content, "jumped to 15f1") {
		t.Errorf("the status bar carries it")
	}
	now = now.Add(messageTTL + time.Second)
	if m.statusText() != "" {
		t.Errorf("it expires: %q", m.statusText())
	}
	next, _ = m.Update(jumpMsg{err: errors.New("not in tmux")})
	if got := next.(model).statusText(); got != "jump: not in tmux" {
		t.Errorf("an error is shown: %q", got)
	}
}

func TestQuitOnJump(t *testing.T) {
	m := loaded(t, tabWatch)
	m.quitJump = true
	if _, cmd := m.Update(jumpMsg{text: "jumped to x"}); cmd == nil {
		t.Errorf("--quit-on-jump leaves after a good jump")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("it quits")
	}
	if _, cmd := m.Update(jumpMsg{err: errors.New("no")}); cmd != nil {
		t.Errorf("a failed jump stays")
	}
}

func TestAttachClients(t *testing.T) {
	procs := parseProcs(strings.Join([]string{
		"100 1 /opt/homebrew/Caskroom/claude-code@latest/2.1.296/claude daemon run",
		"200 90 /opt/homebrew/Caskroom/claude-code@latest/2.1.296/claude attach 93a01840",
		"201 91 claude attach 93a01840-2628",
		"202 92 claude attach deadbeef",
		"203 93 claude attach 93a", // too short to name a session
		"204 94 claude --resume 93a01840",
		"205 95 vim attach 93a01840", // not claude
	}, "\n"))
	got := attachClients(procs, "93a01840-2628-491e-805d-2632373346df")
	if !reflect.DeepEqual(got, []int{200, 201}) {
		t.Errorf("only claude attach clients that name this session: %v", got)
	}
	if got := attachClients(procs, "ffffffff"); len(got) != 0 {
		t.Errorf("another session: %v", got)
	}
}

func TestJumpFindsTheClientOfABackgroundSession(t *testing.T) {
	sess := Session{ID: "93a01840-2628-491e-805d-2632373346df", Short: "93a0", PID: pidOf(69856)}
	f := &fake{answer: map[string]string{
		"tmux list-panes": "%4 19568 /dev/ttys003 $1 @2\n",
		// the session's own process hangs off a daemon; the client sits under the pane's shell
		"ps -A": "69844 1 claude daemon run\n69851 69844 claude bg-pty-host\n69856 69851 claude bg-spare\n19568 1 -zsh\n70018 19568 claude attach 93a01840\n",
	}}
	msg, err := jump(context.Background(), sess, jumpEnv{Tmux: true}, f.run)
	if err != nil || msg != "jumped to 93a0" {
		t.Fatalf("a background session is found through its client: %q %v", msg, err)
	}
	if last := f.calls[len(f.calls)-1]; last != "tmux select-window -t @2 ; select-pane -t %4 ; switch-client -t $1" {
		t.Errorf("the jump call: %q", last)
	}
	two := &fake{answer: map[string]string{
		"tmux list-panes": "%4 19568 /dev/ttys003 $1 @2\n%5 19570 /dev/ttys004 $1 @3\n",
		"ps -A":           "69856 69851 claude bg-spare\n19568 1 -zsh\n19570 1 -zsh\n70018 19568 claude attach 93a01840\n70019 19570 claude attach 93a01840\n",
	}}
	if _, err := jump(context.Background(), sess, jumpEnv{Tmux: true}, two.run); err == nil || !strings.Contains(err.Error(), "several terminals") {
		t.Errorf("two clients are ambiguous, so no guess: %v", err)
	}
}

func TestJumpRefusesAnImpossiblePid(t *testing.T) {
	f := &fake{}
	for _, pid := range []int64{0, 1, -5} {
		if _, err := jump(context.Background(), Session{Short: "x", PID: pidOf(pid)}, jumpEnv{Tmux: true}, f.run); err == nil || !strings.Contains(err.Error(), "process") {
			t.Errorf("pid %d: %v", pid, err)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("no command runs for such a pid: %v", f.calls)
	}
}

func TestALookAlikeClientCannotBlockARealMatch(t *testing.T) {
	sess := Session{ID: "93a01840-2628-491e-805d-2632373346df", Short: "93a0", PID: pidOf(70)}
	f := &fake{answer: map[string]string{
		"tmux list-panes": "%2 60 /dev/ttys002 $1 @4\n",
		// the session's own chain leads to pane %2, and two processes claim to be attach clients
		"ps -A": "60 1 -zsh\n70 60 claude\n201 91 claude attach 93a01840\n202 92 claude attach 93a01840\n",
	}}
	if msg, err := jump(context.Background(), sess, jumpEnv{Tmux: true}, f.run); err != nil || msg != "jumped to 93a0" {
		t.Errorf("the session's own chain is tried first: %q %v", msg, err)
	}
}
