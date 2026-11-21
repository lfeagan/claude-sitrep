package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statetest"
)

var logNow = time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)

// logRun runs a command with the history log on (or off) at a fixed time on a host called mbp.
func logRun(t *testing.T, home string, on bool, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	e := Env{Home: home, Stdout: &out, Stderr: &errb, Clock: clock.Fixed(logNow), Hostname: func() (string, error) { return "mbp.local", nil },
		Getenv: func(k string) string {
			if k == "SITREP_LOG" && on {
				return "on"
			}
			return ""
		}}
	if stdin != "" {
		e.Stdin = strings.NewReader(stdin)
		e.Spawn = func(...string) error { return nil }
	}
	code := Run(args, e)
	return code, out.String(), errb.String()
}

func mustRun(t *testing.T, home string, args ...string) string {
	t.Helper()
	code, out, errs := logRun(t, home, true, "", args...)
	if code != 0 {
		t.Fatalf("%v: %d %q", args, code, errs)
	}
	return out
}

func logHome(t *testing.T) string {
	home := t.TempDir()
	st := state.Open(home)
	statetest.PutAt(t, st, "abcd1111.txt", "Objective: x\n", logNow.Add(-time.Hour))
	return home
}

func TestLogOffWritesNothing(t *testing.T) {
	home := logHome(t)
	if code, _, _ := logRun(t, home, false, "", "add", "inbox", "x"); code != 0 {
		t.Fatal("add failed")
	}
	if _, err := os.Stat(filepath.Join(state.Open(home).Dir, "log")); !os.IsNotExist(err) {
		t.Fatal("log folder made while the log is off")
	}
	_, out, _ := logRun(t, home, false, "", "log")
	if !strings.Contains(out, "no entries\nthe history log is off") {
		t.Fatalf("log: %q", out)
	}
}

func TestLogRecordsChanges(t *testing.T) {
	home := logHome(t)
	mustRun(t, home, "add", "inbox", "one")
	mustRun(t, home, "add", "inbox", "two")
	mustRun(t, home, "add", "inbox", "three")
	mustRun(t, home, "top", "inbox", "3")
	mustRun(t, home, "edit", "inbox", "1", "THREE")
	mustRun(t, home, "move", "inbox", "abcd")
	mustRun(t, home, "rm", "abcd")
	mustRun(t, home, "reorder", "inbox", "1", "1") // no change, no entry
	mustRun(t, home, "pin", "abcd", "the goal")
	mustRun(t, home, "unpin", "abcd")
	mustRun(t, home, "unpin", "abcd") // already off, no entry
	mustRun(t, home, "snooze", "abcd")
	mustRun(t, home, "snooze", "--off", "abcd")
	mustRun(t, home, "snooze", "--off", "abcd") // already off, no entry
	mustRun(t, home, "alias", "abcd", "work")
	mustRun(t, home, "alias", "abcd", "job")
	mustRun(t, home, "alias", "abcd", "--off")
	out := mustRun(t, home, "log", "-n", "0")
	want := strings.Join([]string{
		"2026-11-21 14:00Z  mbp_local  inbox  queue.add      one",
		"2026-11-21 14:00Z  mbp_local  inbox  queue.add      two",
		"2026-11-21 14:00Z  mbp_local  inbox  queue.add      three",
		"2026-11-21 14:00Z  mbp_local  inbox  queue.reorder  3 -> 1: three",
		"2026-11-21 14:00Z  mbp_local  inbox  queue.edit     three -> THREE",
		"2026-11-21 14:00Z  mbp_local  inbox  queue.move     -> abcd: THREE",
		"2026-11-21 14:00Z  mbp_local  abcd   queue.rm       THREE",
		"2026-11-21 14:00Z  mbp_local  abcd   pin            the goal",
		"2026-11-21 14:00Z  mbp_local  abcd   unpin",
		"2026-11-21 14:00Z  mbp_local  abcd   snooze",
		"2026-11-21 14:00Z  mbp_local  abcd   unsnooze",
		"2026-11-21 14:00Z  mbp_local  abcd   alias          work",
		"2026-11-21 14:00Z  mbp_local  abcd   alias          work -> job",
		"2026-11-21 14:00Z  mbp_local  abcd   alias          job removed",
	}, "\n") + "\n"
	if out != want {
		t.Fatalf("log:\n%s\nwant:\n%s", out, want)
	}
}

func TestLogFilters(t *testing.T) {
	home := logHome(t)
	mustRun(t, home, "add", "inbox", "one")
	mustRun(t, home, "move", "inbox", "abcd")
	mustRun(t, home, "add", "abcd", "two")
	if out := mustRun(t, home, "log", "--session", "abcd"); strings.Count(out, "\n") != 2 { // the move counts for its destination
		t.Fatalf("by session: %q", out)
	}
	if out := mustRun(t, home, "log", "--op", "queue.add"); strings.Count(out, "\n") != 2 {
		t.Fatalf("by op: %q", out)
	}
	if out := mustRun(t, home, "log", "-n", "1"); !strings.Contains(out, "two") || strings.Count(out, "\n") != 1 {
		t.Fatalf("limit: %q", out)
	}
	if out := mustRun(t, home, "log", "--since", "1h"); strings.Count(out, "\n") != 3 {
		t.Fatalf("since: %q", out)
	}
	if out := mustRun(t, home, "log", "--since", "2026-11-22"); !strings.Contains(out, "no entries") {
		t.Fatalf("since date: %q", out)
	}
	if out := mustRun(t, home, "log", "--host", "other"); !strings.Contains(out, "no entries") {
		t.Fatalf("host: %q", out)
	}
	for _, bad := range [][]string{{"log", "--op", "rm -rf"}, {"log", "--since", "soon"}, {"log", "-n", "-1"}, {"log", "--session", "../x"}} {
		if code, _, _ := logRun(t, home, true, "", bad...); code == 0 {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestLogJSON(t *testing.T) {
	home := logHome(t)
	mustRun(t, home, "add", "inbox", "one")
	mustRun(t, home, "move", "inbox", "abcd")
	var rows []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(mustRun(t, home, "log", "--json")), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		rows = append(rows, m)
	}
	if len(rows) != 2 || rows[1]["op"] != "queue.move" || rows[1]["dest"] != "abcd1111" || rows[1]["id"] != "inbox" || rows[1]["host"] != "mbp_local" || rows[1]["t"] != "2026-11-21T14:00:00Z" {
		t.Fatalf("rows %v", rows)
	}
}

func TestLogClear(t *testing.T) {
	home := logHome(t)
	mustRun(t, home, "add", "inbox", "one")
	if out := mustRun(t, home, "log", "--clear"); out != "log cleared\n" {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(state.Open(home).Dir, "log")); !os.IsNotExist(err) {
		t.Fatal("log folder left")
	}
	if q, _ := os.ReadFile(state.Open(home).Path(state.Inbox, "queue")); string(q) != "one\n" {
		t.Fatal("clearing the log touched the queue")
	}
}

func TestLogTextIsClean(t *testing.T) {
	home := logHome(t)
	mustRun(t, home, "add", "inbox", "a\x1b[31mred‮")
	b, _ := os.ReadFile(filepath.Join(state.Open(home).Dir, "log", "mbp_local", "current.jsonl"))
	if strings.ContainsAny(string(b), "\x1b‮") || strings.Contains(string(b), `\u001b`) {
		t.Fatalf("raw control text stored: %q", b)
	}
}

func TestLogHookEvents(t *testing.T) {
	home := logHome(t)
	ev := func(name, extra string) string {
		return `{"session_id":"abcd1111","hook_event_name":"` + name + `"` + extra + `,"cwd":"/x"}`
	}
	for _, in := range []string{
		ev("SessionStart", `,"source":"startup"`),
		ev("SessionStart", `,"source":"compact"`), // a compaction is not a start
		ev("UserPromptSubmit", `,"prompt":"+ do the thing"`),
		ev("UserPromptSubmit", `,"prompt":"+pin ship it"`),
		ev("UserPromptSubmit", `,"prompt":"+inbox later"`),
		ev("UserPromptSubmit", `,"prompt":"+take"`),
		ev("SessionEnd", ``),
	} {
		if code, _, _ := logRun(t, home, true, in, "hook"); code != 0 {
			t.Fatalf("hook %s failed", in)
		}
	}
	out := mustRun(t, home, "log", "-n", "0")
	for _, want := range []string{"session.start", "queue.add      do the thing", "pin            ship it", "queue.add      later", "queue.move     -> abcd: later", "session.end"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Count(out, "session.start") != 1 {
		t.Errorf("a compaction was logged as a start:\n%s", out)
	}
}

func TestLogPruneEntry(t *testing.T) {
	e, out, _, dir := pruneEnv(t, "none")
	statetest.Put(t, state.Store{Dir: dir}, ".done0001.state", "waiting\n", 48*time.Hour) // gone, and not yet marked
	e.Getenv = func(k string) string {
		if k == "SITREP_LOG" {
			return "on"
		}
		return ""
	}
	e.Hostname = func() (string, error) { return "mbp", nil }
	if code := Run([]string{"prune", "-k"}, e); code != 0 {
		t.Fatalf("prune: %d %s", code, out)
	}
	if ops := logItems(t, dir); len(ops) != 1 || ops[0] != "prune" {
		t.Fatalf("entries %v (%s)", ops, out)
	}
}

// logItems lists the operations in this host's log of a state folder.
func logItems(t *testing.T, dir string) []string {
	t.Helper()
	var ops []string
	b, _ := os.ReadFile(filepath.Join(dir, "log", "mbp", "current.jsonl"))
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var m struct{ Op string }
		if json.Unmarshal([]byte(l), &m) == nil && m.Op != "" {
			ops = append(ops, m.Op)
		}
	}
	return ops
}

func TestLogSessionAmbiguousIsAnError(t *testing.T) {
	home := logHome(t)
	statetest.PutAt(t, state.Open(home), "abce2222.txt", "Objective: y\n", logNow.Add(-time.Hour))
	if code, _, errs := logRun(t, home, true, "", "log", "--session", "abc"); code == 0 || !strings.Contains(errs, "matches 2") {
		t.Fatalf("ambiguous prefix: %d %q", code, errs)
	}
	if code, out, _ := logRun(t, home, true, "", "log", "--session", "gone0000"); code != 0 || !strings.Contains(out, "no entries") {
		t.Fatalf("a session that is gone: %d %q", code, out)
	}
}
