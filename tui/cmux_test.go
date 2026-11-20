package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func realTop(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/cmux_top.tsv")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFindCmuxPlaceFromTheRealOutput(t *testing.T) {
	parents := parseCmuxTop(realTop(t))
	// the claude attach client (70018) sits under the shell 19568, which sits in surface 0
	want := cmuxPlace{Window: "window:1000000000", Workspace: "workspace:1000000000", Pane: "pane:1000000000", Surface: "surface:1000000000"}
	if got, ok := findCmuxPlace(parents, []int{70018, 19568}); !ok || got != want {
		t.Errorf("the attach client leads to its surface: %+v %v", got, ok)
	}
	if got, ok := findCmuxPlace(parents, []int{99999, 19568}); !ok || got != want {
		t.Errorf("the first process cmux knows is used: %+v %v", got, ok)
	}
	other := cmuxPlace{Window: "window:1000000000", Workspace: "workspace:1000000000", Pane: "pane:1000000000", Surface: "surface:1000000001"}
	if got, ok := findCmuxPlace(parents, []int{86181}); !ok || got != other {
		t.Errorf("the other surface: %+v %v", got, ok)
	}
	if _, ok := findCmuxPlace(parents, []int{4242, 4243}); ok {
		t.Errorf("a process cmux does not know has no place")
	}
	if _, ok := findCmuxPlace(parents, nil); ok {
		t.Errorf("an empty chain has no place")
	}
}

func TestParseCmuxTopIgnoresBadRows(t *testing.T) {
	out := strings.Join([]string{
		"short\trow",
		"1\t2\t3\tprocess\tabc\tsurface:1\tx",              // a pid that is not a number
		"1\t2\t3\tprocess\t0\tsurface:1\tx",                // pid 0
		"1\t2\t3\tsurface\tsurface:1; rm -rf /\tpane:1\tx", // a ref with shell text
		"1\t2\t3\tsurface\tpane:1\tpane:1\tx",              // a pane ref on a surface row
		"1\t2\t3\tcheese\tpane:1\tpane:1\tx",               // an unknown kind
		"1\t2\t3\tprocess\t10\tsurface:1\tbash",
		"1\t2\t3\tsurface\tsurface:1\tpane:1\tx",
		"1\t2\t3\tpane\tpane:1\tworkspace:1\t",
		"1\t2\t3\tworkspace\tworkspace:1\twindow:1\t",
		"1\t2\t3\twindow\twindow:1\ttotal\t",
	}, "\n")
	parents := parseCmuxTop(out)
	if len(parents) != 4 {
		t.Errorf("only the good rows count: %v", parents)
	}
	if _, ok := findCmuxPlace(parents, []int{10}); !ok {
		t.Errorf("the good chain works")
	}
	loop := map[string]string{"10": "11", "11": "10"}
	if _, ok := findCmuxPlace(loop, []int{10}); ok {
		t.Errorf("a loop ends without a place")
	}
}

func TestCmuxSteps(t *testing.T) {
	p := cmuxPlace{Window: "window:1", Workspace: "workspace:2", Pane: "pane:3", Surface: "surface:4"}
	want := [][]string{
		{"focus-window", "--window", "window:1"},
		{"select-workspace", "--workspace", "workspace:2", "--window", "window:1"},
		{"focus-pane", "--pane", "pane:3", "--workspace", "workspace:2", "--window", "window:1"},
		{"focus-panel", "--panel", "surface:4", "--workspace", "workspace:2", "--window", "window:1"},
	}
	if got := cmuxSteps(p); !reflect.DeepEqual(got, want) {
		t.Errorf("the four fixed commands: %v", got)
	}
}

func TestJumpThroughCmux(t *testing.T) {
	sess := Session{ID: "93a01840-2628-491e-805d-2632373346df", Short: "93a0", PID: pidOf(69856)}
	f := &fake{answer: map[string]string{
		"ps -A":    "69844 1 claude daemon run\n69856 69851 claude bg-spare\n19568 1 -zsh\n70018 19568 claude attach 93a01840\n",
		"cmux top": realTop(t),
	}}
	msg, err := jump(context.Background(), sess, jumpEnv{Cmux: true}, f.run)
	if err != nil || msg != "jumped to 93a0" {
		t.Fatalf("a jump through cmux: %q %v", msg, err)
	}
	got := strings.Join(f.calls, "\n")
	for _, want := range []string{
		"cmux top --all --processes --format tsv",
		"cmux focus-window --window window:1000000000",
		"cmux select-workspace --workspace workspace:1000000000 --window window:1000000000",
		"cmux focus-pane --pane pane:1000000000 --workspace workspace:1000000000 --window window:1000000000",
		"cmux focus-panel --panel surface:1000000000 --workspace workspace:1000000000 --window window:1000000000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a missing call %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "tmux") {
		t.Errorf("tmux is not asked when the dashboard is not in tmux:\n%s", got)
	}
}

func TestJumpCmuxFallsBackAfterTmux(t *testing.T) {
	sess := Session{ID: "93a01840-2628-491e-805d-2632373346df", Short: "93a0", PID: pidOf(69856)}
	f := &fake{answer: map[string]string{
		"ps -A":           "19568 1 -zsh\n70018 19568 claude attach 93a01840\n",
		"tmux list-panes": "%1 5 /dev/ttys009 $1 @1\n", // tmux does not hold it
		"cmux top":        realTop(t),
	}}
	if msg, err := jump(context.Background(), sess, jumpEnv{Tmux: true, Cmux: true}, f.run); err != nil || msg == "" {
		t.Fatalf("tmux has no pane for it, so cmux is tried: %q %v", msg, err)
	}
	got := strings.Join(f.calls, "\n")
	if strings.Contains(got, "select-window") || !strings.Contains(got, "cmux focus-panel") {
		t.Errorf("only the cmux commands run:\n%s", got)
	}
	none := &fake{answer: map[string]string{"ps -A": "5 1 x\n", "tmux list-panes": "", "cmux top": realTop(t)}}
	if _, err := jump(context.Background(), sess, jumpEnv{Tmux: true, Cmux: true}, none.run); err == nil || !strings.Contains(err.Error(), "not in a pane") {
		t.Errorf("neither knows it: %v", err)
	}
	broken := &fake{answer: map[string]string{"ps -A": "19568 1 x\n70018 19568 claude attach 93a01840\n"}, fail: map[string]error{"cmux focus-pane": os.ErrPermission}}
	broken.answer["cmux top"] = realTop(t)
	if _, err := jump(context.Background(), sess, jumpEnv{Cmux: true}, broken.run); err == nil {
		t.Errorf("a failing cmux command is reported")
	}
	if strings.Contains(strings.Join(broken.calls, "\n"), "focus-panel") {
		t.Errorf("a failure stops the remaining commands")
	}
}

func TestCurrentEnv(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("CMUX_SURFACE_ID", "")
	t.Setenv("CMUX_WORKSPACE_ID", "")
	if env := currentEnv(); env.Tmux || env.Cmux {
		t.Errorf("neither: %+v", env)
	}
	t.Setenv("TMUX", "/tmp/tmux-501/default,1,0")
	t.Setenv("CMUX_SURFACE_ID", "abc")
	if env := currentEnv(); !env.Tmux || !env.Cmux {
		t.Errorf("both: %+v", env)
	}
}
