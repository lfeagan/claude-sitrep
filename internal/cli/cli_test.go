package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statetest"
)

func run(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	e := Env{Home: home, Getenv: func(string) string { return "" }, Stdout: &out, Stderr: &errb, Clock: clock.Real{}}
	code := Run(args, e)
	return code, out.String(), errb.String()
}

func TestRunInProcess(t *testing.T) {
	home := t.TempDir()
	if code, out, _ := run(t, home, "add", "inbox", "first"); code != 0 || out != "queued on inbox (now 1)\n" {
		t.Errorf("add: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "rm", "inbox"); code != 0 || out != "first\n" {
		t.Errorf("rm: %d %q", code, out)
	}
	if code, _, errs := run(t, home, "rm", "inbox"); code != 1 || errs != "sitrep: queue empty\n" {
		t.Errorf("rm empty: %d %q", code, errs)
	}
}

func TestErrorTextIsSafe(t *testing.T) {
	_, _, errs := run(t, t.TempDir(), "no\x1b[31m\u202ething")
	if strings.ContainsAny(errs, "\x1b\u202e") || errs != "sitrep: unknown command 'no[31mthing'; run 'sitrep help'\n" {
		t.Errorf("stderr %q", errs)
	}
	_, _, errs = run(t, t.TempDir(), "add", "x\x1b[0m", "text")
	if strings.Contains(errs, "\x1b") {
		t.Errorf("stderr %q", errs)
	}
}

func TestItemNo(t *testing.T) {
	for in, want := range map[string]int{"1": 1, "42": 42} {
		if got, err := itemNo(in); err != nil || got != want {
			t.Errorf("itemNo(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0", "01", "-1", "1x", "x", " 1"} {
		if _, err := itemNo(in); err == nil {
			t.Errorf("itemNo(%q) accepted", in)
		}
	}
	if n, err := itemNo("99999999999999999999999"); err != nil || n < 1<<30 {
		t.Errorf("a huge number is a number past every item: %d, %v", n, err)
	}
}

func TestSnoozeInProcess(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	at := time.Unix(1_700_000_000, 0)
	statetest.PutAt(t, st, "abcd1111.txt", "Objective: x\n", at)
	statetest.PutAt(t, st, ".abcd1111.state", "working\n", at)
	if code, _, errs := run(t, home, "snooze", "abcd"); code != 1 || errs != "sitrep: abcd does not need you, so there is nothing to snooze\n" {
		t.Errorf("a working session: %d %q", code, errs)
	}
	if _, err := os.Stat(statetest.Path(st, ".abcd1111.snooze")); !os.IsNotExist(err) {
		t.Error("a refused snooze left a file")
	}
	statetest.PutAt(t, st, ".abcd1111.state", "waiting\n", at)
	if code, out, _ := run(t, home, "snooze", "abcd"); code != 0 || out != "snoozed abcd until it changes\n" {
		t.Errorf("snooze: %d %q", code, out)
	}
	if b, _ := os.ReadFile(statetest.Path(st, ".abcd1111.snooze")); string(b) != "1700000000\n" {
		t.Errorf("snooze file %q", b)
	}
	if code, out, _ := run(t, home, "snooze", "--off", "abcd"); code != 0 || out != "abcd is no longer snoozed\n" {
		t.Errorf("snooze off: %d %q", code, out)
	}
	if _, err := os.Stat(statetest.Path(st, ".abcd1111.snooze")); !os.IsNotExist(err) {
		t.Error("the snooze file is still there")
	}
	run(t, home, "snooze", "abcd")
	if code, out, _ := run(t, home, "snooze", "abcd", "--off"); code != 0 || out != "abcd is no longer snoozed\n" {
		t.Errorf("--off after the id: %d %q", code, out)
	}
	if code, _, errs := run(t, home, "snooze", "--off", "abcd", "--off"); code != 1 || !strings.Contains(errs, "usage: sitrep snooze") {
		t.Errorf("a second --off is a usage error: %d %q", code, errs)
	}
	if code, _, errs := run(t, home, "snooze", "inbox"); code != 1 || errs != "sitrep: the inbox is not a session\n" {
		t.Errorf("snooze inbox: %d %q", code, errs)
	}
}

// pruneEnv builds a world with one finished session holding two items, and a registry that has one live session.
func pruneEnv(t *testing.T, tty string) (Env, *bytes.Buffer, *bytes.Buffer, string) {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "sitrep")
	st := state.Open(home)
	sess := filepath.Join(home, ".claude", "sessions")
	os.MkdirAll(dir, 0o700)
	os.MkdirAll(sess, 0o755)
	os.WriteFile(filepath.Join(sess, "1.json"), []byte(`{"pid":1,"sessionId":"live0001"}`), 0o600)
	for id, q := range map[string]string{"live0001": "", "done0001": "one\ntwo\n"} {
		statetest.Put(t, st, id+".txt", "Objective: x\n", 0)
		statetest.Put(t, st, id+".cwd", "/w/"+id+"\n", 0)
		if q != "" {
			statetest.Put(t, st, id+".queue", q, 0)
		}
	}
	statetest.Put(t, st, ".done0001.state", "ended\n", 0)
	var out, errb bytes.Buffer
	e := Env{Home: home, Getenv: func(string) string { return "" }, Stdout: &out, Stderr: &errb,
		Clock: clock.Real{}, Alive: func(int) bool { return true },
		Runner: &runner.Fake{Answers: map[string]runner.Result{"ps -o lstart= -p 1": {}}}}
	if tty != "none" {
		e.TTY = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(tty)), nil }
	}
	return e, &out, &errb, dir
}

func TestPruneAsksOnATerminal(t *testing.T) {
	for answer, moved := range map[string]string{"y\n": "2", "n\n": "0", "\n": "0", "": "0", "yes\n": "2"} {
		e, out, errb, dir := pruneEnv(t, answer)
		if code := Run([]string{"prune"}, e); code != 0 {
			t.Fatalf("exit %d: %s", code, errb)
		}
		if !strings.Contains(errb.String(), "Move 2 item(s) from done (done0001) to the inbox? [y/N] ") {
			t.Errorf("prompt %q", errb)
		}
		if want := "0 marked ended, " + moved + " item(s) moved to the inbox\n"; out.String() != want {
			t.Errorf("answer %q: %q, want %q", answer, out, want)
		}
		_, err := os.Stat(filepath.Join(dir, "inbox.queue"))
		if (moved == "2") != (err == nil) {
			t.Errorf("answer %q: inbox exists %v", answer, err == nil)
		}
	}
}

func TestPruneWithoutATerminalLeavesTheItems(t *testing.T) {
	e, out, errb, dir := pruneEnv(t, "none")
	if code := Run([]string{"prune"}, e); code != 0 {
		t.Fatal(code)
	}
	if errb.String() != "2 item(s) left on done (done0001): run prune in a terminal, or use -y\n" || out.String() != "0 marked ended, 0 item(s) moved to the inbox\n" {
		t.Errorf("stderr %q, stdout %q", errb, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.queue")); err == nil {
		t.Error("items moved without an answer")
	}
}

func TestPruneKeepDoesNotAsk(t *testing.T) {
	e, out, _, _ := pruneEnv(t, "y\n")
	if code := Run([]string{"prune", "-k"}, e); code != 0 {
		t.Fatal(code)
	}
	if out.String() != "kept 2 item(s) on done (done0001)\n0 marked ended, 0 item(s) moved to the inbox\n" {
		t.Errorf("%q", out)
	}
}

func TestAnEmptyCommandIsLs(t *testing.T) {
	e, out, _, _ := pruneEnv(t, "none")
	if code := Run([]string{""}, e); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "ID    DIRECTORY") {
		t.Errorf("%q", out)
	}
}

func TestPruneAnswerWithoutNewlineIsNo(t *testing.T) {
	e, out, _, dir := pruneEnv(t, "y") // end of input before the end of the line
	if code := Run([]string{"prune"}, e); code != 0 {
		t.Fatal(code)
	}
	if out.String() != "0 marked ended, 0 item(s) moved to the inbox\n" {
		t.Errorf("%q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "inbox.queue")); err == nil {
		t.Error("items moved")
	}
}

func TestSetupUsage(t *testing.T) {
	for _, args := range [][]string{{"setup", "--bogus"}, {"setup", "--remove", "x"}, {"setup", "-y"}} {
		if code, _, errs := run(t, t.TempDir(), args...); code != 1 || errs != "sitrep: usage: sitrep setup [--remove]\n" {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
}

func TestUpdateCheckCommand(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	st.Ensure()
	f := &runner.Fake{Answers: map[string]runner.Result{
		runner.Key("git", "ls-remote", "--tags", "--refs", "https://github.com/lfeagan/claude-sitrep.git"): {Out: []byte("a\trefs/tags/v9.0.0\n")},
	}}
	env := map[string]string{}
	e := Env{Home: home, Getenv: func(k string) string { return env[k] }, Stdout: io.Discard, Stderr: io.Discard, Clock: clock.Fixed(time.Unix(1_800_000_000, 0)), Runner: f}
	env["SITREP_CHECK_UPDATES"] = "0"
	if Run([]string{"update-check"}, e) != 0 || len(f.Calls()) != 0 {
		t.Errorf("it ran with the setting off: %v", f.Calls())
	}
	delete(env, "SITREP_CHECK_UPDATES")
	if Run([]string{"update-check"}, e) != 0 || len(f.Calls()) != 1 {
		t.Fatalf("calls %v", f.Calls())
	}
	b, _ := os.ReadFile(filepath.Join(st.Dir, "update"))
	if string(b) != "1800000000 9.0.0\n" {
		t.Errorf("record %q", b)
	}
	if c := f.Calls()[0]; !strings.Contains(strings.Join(c.Env, " "), "GIT_TERMINAL_PROMPT=0") {
		t.Errorf("env %v", c.Env)
	}
}

func TestEditCommand(t *testing.T) {
	home := t.TempDir()
	run(t, home, "add", "inbox", "first")
	run(t, home, "add", "inbox", "secnd")
	if code, out, _ := run(t, home, "edit", "inbox", "2", "second", "\x1b[31mitem"); code != 0 || out != "item 2 on inbox: second [31mitem\n" {
		t.Errorf("edit: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "ls", "inbox"); code != 0 || !strings.Contains(out, "1. first") || !strings.Contains(out, "2. second") {
		t.Errorf("ls: %d %q", code, out)
	}
	if code, _, errs := run(t, home, "edit", "inbox", "1", "x", "--item", "other"); code != 1 || !strings.Contains(errs, "queue changed") {
		t.Errorf("guard: %d %q", code, errs)
	}
	if code, out, _ := run(t, home, "edit", "inbox", "1", "x", "--item", "first"); code != 0 || out != "item 1 on inbox: x\n" {
		t.Errorf("guarded edit: %d %q", code, out)
	}
	for _, a := range [][]string{{"edit"}, {"edit", "inbox"}, {"edit", "inbox", "1"}} {
		if code, _, errs := run(t, home, a...); code != 1 || !strings.HasPrefix(errs, "sitrep: usage: sitrep edit") {
			t.Errorf("%v: %d %q", a, code, errs)
		}
	}
	if code, _, errs := run(t, home, "edit", "inbox", "0", "x"); code != 1 || errs != "sitrep: item must be a number\n" {
		t.Errorf("item 0: %d %q", code, errs)
	}
	if code, _, errs := run(t, home, "edit", "inbox", "9", "x"); code != 1 || errs != "sitrep: no item 9 on inbox\n" {
		t.Errorf("item 9: %d %q", code, errs)
	}
}

func TestAliasCommand(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	for _, id := range []string{"c5d05701", "dfc08fd7"} {
		if err := st.WriteFile(st.Path(state.ID(id), "notes"), []byte("Objective: x\n")); err != nil {
			t.Fatal(err)
		}
	}
	st.WriteFile(st.Path("c5d05701", "cwd"), []byte("/work/claude-sitrep\n"))
	if code, out, _ := run(t, home, "alias", "c5d0", "sitrep"); code != 0 || out != "c5d0 is now called sitrep\n" {
		t.Errorf("set: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "alias", "sitrep", "sitrep"); code != 0 || out != "c5d0 is already called sitrep\n" {
		t.Errorf("again: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "add", "sitrep", "an", "item"); code != 0 || out != "queued on sitrep (now 1)\n" {
		t.Errorf("add by alias: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "ls", "sitrep"); code != 0 || !strings.HasPrefix(out, "Session sitrep (c5d0)  /work/claude-sitrep") || !strings.Contains(out, "1. an item") {
		t.Errorf("ls by alias: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "alias"); code != 0 || out != "sitrep  c5d0  claude-sitrep\n" {
		t.Errorf("list: %d %q", code, out)
	}
	if code, _, errs := run(t, home, "alias", "dfc0", "sitrep"); code != 1 || !strings.Contains(errs, "already the alias of c5d0") {
		t.Errorf("taken: %d %q", code, errs)
	}
	if code, _, errs := run(t, home, "alias", "inbox", "x"); code != 1 || errs == "" {
		t.Errorf("inbox: %d %q", code, errs)
	}
	if code, _, errs := run(t, home, "alias", "c5d0"); code != 1 || !strings.HasPrefix(errs, "sitrep: usage: sitrep alias") {
		t.Errorf("usage: %d %q", code, errs)
	}
	if code, out, _ := run(t, home, "alias", "c5d0", "--off"); code != 0 || out != "c5d0 is no longer called sitrep\n" {
		t.Errorf("off: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "alias", "c5d0", "--off"); code != 0 || out != "c5d0 has no alias\n" {
		t.Errorf("off again: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "add", "c5d0", "x"); code != 0 || out != "queued on c5d0 (now 2)\n" {
		t.Errorf("add by id after: %d %q", code, out)
	}
}

func TestTheHookReadsItsSettingsFromTheConfFile(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	st.Ensure()
	os.WriteFile(filepath.Join(st.Dir, "sitrep.conf"), []byte("SITREP_DEBOUNCE=9\n# SITREP_TUI_BELL=0\n"), 0o600)
	env := map[string]string{}
	e := Env{Home: home, Getenv: func(k string) string { return env[k] }}
	if got := e.hook().Getenv("SITREP_DEBOUNCE"); got != "9" {
		t.Errorf("from the file: %q", got)
	}
	env["SITREP_DEBOUNCE"] = "2"
	if got := e.hook().Getenv("SITREP_DEBOUNCE"); got != "2" {
		t.Errorf("the environment wins: %q", got)
	}
	if got := e.conf().Get("SITREP_TUI_BELL"); got != "" {
		t.Errorf("a commented line is not a setting: %q", got)
	}
}

func TestLsJSONCarriesTheContextWindow(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	for _, id := range []string{"c5d05701", "dfc08fd7"} {
		st.WriteFile(st.Path(state.ID(id), "notes"), []byte("Objective: x\n"))
		st.WriteFile(st.Path(state.ID(id), "state"), []byte("waiting\n"))
	}
	if err := st.RecordContext("c5d05701", state.Context{UsedPercent: 37.5, Window: 1_000_000, Tokens: 375_000, At: 1_800_000_000}); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(t, home, "ls", "--json")
	if code != 0 {
		t.Fatalf("%d %s", code, errs)
	}
	var doc struct {
		Sessions []struct {
			ID      string `json:"id"`
			Context *struct {
				UsedPercent float64 `json:"used_percent"`
				Window      int64   `json:"window"`
				Tokens      int64   `json:"tokens"`
				At          int64   `json:"at"`
			} `json:"context"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	seen := map[string]bool{}
	for _, s := range doc.Sessions {
		seen[s.ID] = true
		switch s.ID {
		case "c5d05701":
			if s.Context == nil || s.Context.UsedPercent != 37.5 || s.Context.Window != 1_000_000 || s.Context.Tokens != 375_000 || s.Context.At != 1_800_000_000 {
				t.Errorf("context %+v", s.Context)
			}
		case "dfc08fd7":
			if s.Context != nil {
				t.Errorf("no record is null: %+v", s.Context)
			}
		}
	}
	if !seen["c5d05701"] || !seen["dfc08fd7"] || !strings.Contains(out, `"context": null`) {
		t.Errorf("sessions %v\n%s", seen, out)
	}
	// the plain listing does not change
	if code, text, _ := run(t, home, "ls"); code != 0 || strings.Contains(text, "37") || strings.Contains(text, "CTX") {
		t.Errorf("ls text: %q", text)
	}
}

func TestDoneIsRmUnderAnotherName(t *testing.T) {
	home := t.TempDir()
	for _, text := range []string{"first", "second", "third"} {
		run(t, home, "add", "inbox", text)
	}
	if code, out, _ := run(t, home, "done", "inbox"); code != 0 || out != "first\n" {
		t.Errorf("done: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "done", "inbox", "2"); code != 0 || out != "third\n" {
		t.Errorf("done 2: %d %q", code, out)
	}
	if code, out, errs := run(t, home, "done", "inbox", "--item", "something else"); code != 1 || out != "" || errs == "" {
		t.Errorf("the guard must refuse: %d %q %q", code, out, errs)
	}
	if _, out, _ := run(t, home, "ls", "inbox"); !strings.Contains(out, "1. second") {
		t.Errorf("the guarded done removed something: %q", out)
	}
	if code, _, errs := run(t, home, "done", "inbox", "--item", "second"); code != 0 || errs != "" {
		t.Errorf("a matching guard: %d %q", code, errs)
	}
	if code, _, errs := run(t, home, "done", "inbox"); code != 1 || errs != "sitrep: queue empty\n" {
		t.Errorf("done on an empty queue: %d %q", code, errs)
	}
}

func TestDoneHasItsOwnUsageAndHelp(t *testing.T) {
	home := t.TempDir()
	if code, _, errs := run(t, home, "done"); code != 1 || !strings.HasPrefix(errs, "sitrep: usage: sitrep done <id>") {
		t.Errorf("usage: %d %q", code, errs)
	}
	if _, out, _ := run(t, home, "done", "--help"); !strings.HasPrefix(out, "usage: sitrep done <id> [n]") {
		t.Errorf("help: %q", out)
	}
	if _, out, _ := run(t, home, "help"); !strings.Contains(out, "  done <id> [n]") {
		t.Errorf("the list of commands: %q", out)
	}
	if _, errs := commandsWithoutHelp(); errs != "" {
		t.Error(errs)
	}
}

// commandsWithoutHelp names a command that has no help text, since every command that is offered should explain itself.
func commandsWithoutHelp() (int, string) {
	var missing []string
	for name := range commands {
		if _, ok := help[name]; !ok && !hidden[name] && name != "-h" && name != "--help" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return 0, ""
	}
	return len(missing), "commands without help: " + strings.Join(missing, ", ")
}

func TestUnreadableStateFolderIsNotEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	home := t.TempDir()
	st := state.Open(home)
	if err := st.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(st.Dir, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(st.Dir, 0o700)
	for _, args := range [][]string{{"ls"}, {"ls", "--json"}, {"ls", "inbox"}, {"status"}, {"watch", "--once"}} {
		code, out, errs := run(t, home, args...)
		if code != 1 || out != "" || !strings.HasPrefix(errs, "sitrep: cannot read the state folder "+st.Dir+": ") {
			t.Errorf("%v: %d %q %q", args, code, out, errs)
		}
	}
}

func TestMissingStateFolderIsEmpty(t *testing.T) {
	for _, args := range [][]string{{"ls"}, {"ls", "inbox"}, {"status"}} {
		if code, _, errs := run(t, t.TempDir(), args...); code != 0 {
			t.Errorf("%v: %d %q", args, code, errs)
		}
	}
}

func TestUnreadableQueueIsNotEmpty(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	if err := os.MkdirAll(filepath.Join(st.Dir, "inbox.queue"), 0o700); err != nil { // a folder where the queue file belongs
		t.Fatal(err)
	}
	if code, out, errs := run(t, home, "ls", "inbox"); code != 1 || errs != "sitrep: cannot read the queue of inbox\n" {
		t.Errorf("%d %q %q", code, out, errs)
	}
}

func TestSnoozeOfABlockedSessionKeysOnTheBlock(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	statetest.PutAt(t, st, "abcd1111.txt", "Objective: x\n", time.Unix(1_700_000_000, 0))
	statetest.PutAt(t, st, ".abcd1111.state", "working\n", time.Unix(1_700_000_000, 0))
	if err := os.WriteFile(st.Path("abcd1111", "blocked"), []byte("1700000500 permission\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(t, home, "snooze", "abcd"); code != 0 || out != "snoozed abcd until it changes\n" {
		t.Fatalf("snooze: %d %q", code, out)
	}
	if b, _ := os.ReadFile(st.Path("abcd1111", "snooze")); string(b) != "1700000500\n" {
		t.Errorf("snooze file %q: it must name the time of the block", b)
	}
}

func TestLsQueueShowsABlock(t *testing.T) {
	home := t.TempDir()
	st := state.Open(home)
	statetest.PutAt(t, st, "abcd1111.txt", "Objective: x\n", time.Now().Add(-time.Minute))
	statetest.PutAt(t, st, ".abcd1111.state", "working\n", time.Now().Add(-time.Minute))
	if err := os.WriteFile(st.Path("abcd1111", "blocked"), []byte(strconv.FormatInt(time.Now().Unix()-30, 10)+" permission\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(t, home, "ls", "abcd"); code != 0 || !strings.Contains(out, "(blocked <1m)") {
		t.Errorf("%d %q", code, out)
	}
}
