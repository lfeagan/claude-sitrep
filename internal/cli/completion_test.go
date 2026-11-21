package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statetest"
)

func sorted(l []string) []string { c := append([]string(nil), l...); sort.Strings(c); return c }

// completionEnv has three sessions: two share their first four characters.
func completionEnv(t *testing.T) Env {
	t.Helper()
	e, _, _, _ := pruneEnv(t, "none")
	for _, id := range []string{"abcd1111-x", "abcd2222-y", "zzzz3333-z"} {
		statetest.Put(t, state.Open(e.Home), id+".txt", "Objective: x\n", 0)
	}
	return e
}

func TestShortestPrefixes(t *testing.T) {
	got := shortestPrefixes([]string{"abcd1111", "abcd2222", "zzzz3333", "ab", "abcd11112"})
	want := []string{"abcd1111", "abcd2", "zzzz", "ab", "abcd11112"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := shortestPrefixes([]string{"15f1db8a-b44f", "15f1c2aa-0000"}); !reflect.DeepEqual(got, []string{"15f1d", "15f1c"}) {
		t.Errorf("uuids: %v", got)
	}
}

func TestCompleteCommandsAndFlags(t *testing.T) {
	e := completionEnv(t)
	ctx := context.Background()
	cmds := e.complete(ctx, []string{""})
	for _, want := range []string{"ls", "add", "rm", "move", "watch", "status", "setup", "prune", "snooze", "help", "completion", "version"} {
		if !contains(cmds, want) {
			t.Errorf("no %q in %v", want, cmds)
		}
	}
	for _, no := range []string{"hook", "hook-summarize", "statusline", "__complete", "-h", "--help"} {
		if contains(cmds, no) {
			t.Errorf("%q is offered", no)
		}
	}
	if !reflect.DeepEqual(e.complete(ctx, []string{"l"}), cmds) {
		t.Error("the shell filters the prefix, so every command is offered")
	}
	for cmd, want := range map[string][]string{
		"ls": {"--all", "--full", "--json", "-a", "-n", "-v"}, "prune": {"--dry-run", "--keep", "--yes", "-k", "-y"}, "setup": {"--remove"}, "snooze": {"--off"},
		"rm": {"--item"}, "watch": {"--interval", "--no-bell", "--once", "--quit-on-jump", "--read-only", "--tab", "--width"},
	} {
		if got := sorted(e.complete(ctx, []string{cmd, "-"})); !reflect.DeepEqual(got, sorted(want)) {
			t.Errorf("%s flags: %v", cmd, got)
		}
	}
	if got := e.complete(ctx, []string{"nosuch", ""}); got != nil {
		t.Errorf("%v", got)
	}
}

func TestCompleteFlagValues(t *testing.T) {
	e := completionEnv(t)
	ctx := context.Background()
	for _, w := range [][]string{{"watch", "--tab", ""}, {"status", "--tab", "t"}} {
		if got := e.complete(ctx, w); !reflect.DeepEqual(got, []string{"top", "watch"}) {
			t.Errorf("%v: %v", w, got)
		}
	}
	for _, w := range [][]string{{"watch", "--interval", ""}, {"ls", "-n", ""}, {"rm", "abcd", "1", "--item", ""}} {
		if got := e.complete(ctx, w); len(got) != 0 {
			t.Errorf("%v offered %v", w, got)
		}
	}
	// after a value, the next word is positional again
	if got := e.complete(ctx, []string{"ls", "-n", "5", ""}); !contains(got, "abcd1") {
		t.Errorf("ls -n 5 <id>: %v", got)
	}
}

func TestCompleteIDs(t *testing.T) {
	e := completionEnv(t)
	ctx := context.Background()
	got := e.complete(ctx, []string{"ls", ""})
	for _, want := range []string{"abcd1", "abcd2", "zzzz", "inbox", "live", "done"} {
		if !contains(got, want) {
			t.Errorf("ls: no %q in %v", want, got)
		}
	}
	// the inbox is no session
	for _, cmd := range []string{"pin", "unpin", "snooze"} {
		got := e.complete(ctx, []string{cmd, ""})
		if contains(got, "inbox") || !contains(got, "zzzz") {
			t.Errorf("%s: %v", cmd, got)
		}
	}
	// move takes two ids, and nothing after them
	if got := e.complete(ctx, []string{"move", "abcd1", ""}); !contains(got, "zzzz") || !contains(got, "inbox") {
		t.Errorf("move second: %v", got)
	}
	if got := e.complete(ctx, []string{"move", "abcd1", "inbox", ""}); len(got) != 0 {
		t.Errorf("move third: %v", got)
	}
	// add takes the text after the id; rm takes a number
	if got := e.complete(ctx, []string{"add", "abcd1", ""}); len(got) != 0 {
		t.Errorf("add text: %v", got)
	}
	if got := e.complete(ctx, []string{"rm", "abcd1", ""}); len(got) != 0 {
		t.Errorf("rm number: %v", got)
	}
	// --json means no id; so does a flag that has none
	if got := e.complete(ctx, []string{"ls", "--json", ""}); len(got) != 0 {
		t.Errorf("ls --json: %v", got)
	}
	if got := e.complete(ctx, []string{"snooze", "--off", ""}); !contains(got, "zzzz") {
		t.Errorf("snooze --off: %v", got)
	}
}

func TestCompleteHelpAndCompletion(t *testing.T) {
	e := completionEnv(t)
	ctx := context.Background()
	if got := e.complete(ctx, []string{"help", ""}); !contains(got, "ls") || contains(got, "hook") {
		t.Errorf("%v", got)
	}
	if got := e.complete(ctx, []string{"help", "ls", ""}); len(got) != 0 {
		t.Errorf("%v", got)
	}
	if got := e.complete(ctx, []string{"completion", ""}); !reflect.DeepEqual(got, []string{"bash"}) {
		t.Errorf("%v", got)
	}
}

func TestCompletionCommand(t *testing.T) {
	e, out, errb, _ := pruneEnv(t, "none")
	if code := Run([]string{"completion", "bash"}, e); code != 0 || !strings.Contains(out.String(), "complete -F _sitrep sitrep") {
		t.Errorf("%d %q", code, out)
	}
	errb.Reset()
	if code := Run([]string{"completion", "zsh"}, e); code != 1 || errb.String() != "sitrep: usage: sitrep completion bash\n" {
		t.Errorf("%d %q", code, errb)
	}
	out.Reset()
	if code := Run([]string{"__complete", "ls", "-"}, e); code != 0 || !strings.Contains(out.String(), "--json\n") {
		t.Errorf("%d %q", code, out)
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// The script, run by real bash, with the real program: the way a user will meet it.
func TestBashScript(t *testing.T) {
	bins := map[string]string{}
	for _, b := range []string{"/bin/bash", "bash"} { // the one macOS ships (3.2) and whichever is first on PATH
		if p, err := exec.LookPath(b); err == nil {
			bins[p] = p
		}
	}
	if len(bins) == 0 {
		t.Skip("no bash")
	}
	home := t.TempDir()
	for _, id := range []string{"abcd1111-x", "abcd2222-y", "zzzz3333-z"} {
		statetest.Put(t, state.Open(home), id+".txt", "Objective: x\n", 0)
	}
	prog := filepath.Join(t.TempDir(), "sitrep")
	build := exec.Command("go", "build", "-o", prog, "./cmd/sitrep")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, bash := range bins {
		for line, want := range map[string]string{
			`sitrep ls abc`:        "abcd1 abcd2",
			`sitrep ls z`:          "zzzz",
			`sitrep wa`:            "watch",
			`sitrep watch --tab t`: "top",
			`sitrep status --tab `: "top watch",
			`sitrep pin i`:         "",
			`sitrep move abcd1 in`: "inbox",
			`sitrep prune --d`:     "--dry-run",
			`sitrep help ver`:      "version",
			`sitrep nosuch x`:      "",
		} {
			script := `eval "$("$P" completion bash)"; COMP_LINE="` + line + `"; read -ra COMP_WORDS <<<"$COMP_LINE"; [[ "$COMP_LINE" == *" " ]] && COMP_WORDS+=(""); COMP_WORDS[0]="$P"; COMP_CWORD=$((${#COMP_WORDS[@]}-1)); _sitrep; printf '%s\n' "${COMPREPLY[*]}"`
			cmd := exec.Command(bash, "-c", script)
			cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "P=" + prog}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("%s %q: %v %s", bash, line, err, out)
				continue
			}
			if got := strings.TrimSpace(string(out)); got != want {
				t.Errorf("%s %q: got %q, want %q", bash, line, got, want)
			}
		}
	}
}
