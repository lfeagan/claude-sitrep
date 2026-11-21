package difftest

import (
	"testing"
)

// addAndList is the scenario every port step reuses as a template: change state, then read it back.
func addAndList(t testing.TB, home string, sitrep Runner) []Result {
	return []Result{
		sitrep(t, home, "add", "inbox", "first item"),
		sitrep(t, home, "add", "inbox", "second item"),
		sitrep(t, home, "ls", "inbox"),
		sitrep(t, home, "rm", "inbox", "1"),
		sitrep(t, home, "ls", "inbox"),
	}
}

func TestOracleRuns(t *testing.T) {
	oracle := Oracle(t)
	home := Home(t)
	got := addAndList(t, home, oracle)
	if got[0].Code != 0 || got[0].Stdout != "queued on inbox (now 1)\n" {
		t.Fatalf("add: %+v", got[0])
	}
	want := "Inbox\n 1. second item\n"
	if got[4].Stdout != want {
		t.Fatalf("ls after rm = %q, want %q", got[4].Stdout, want)
	}
	if s := State(t, home); s["inbox.queue"] != (File{Mode: 0o600, Content: "second item\n"}) {
		t.Fatalf("state = %v", s)
	}
}

// The Go side has no queue commands yet (Phase 2), so this proves the comparison itself: the oracle agrees with itself, and a runner
// that answers differently is noticed.
func TestDiffLogic(t *testing.T) {
	oracle := Oracle(t)
	if d := Diff(t, addAndList, oracle, oracle); len(d) != 0 {
		t.Fatalf("the oracle differs from itself: %v", d)
	}
	liar := func(t testing.TB, home string, args ...string) Result { return Result{Code: 1, Stderr: "no"} }
	if d := Diff(t, addAndList, oracle, liar); len(d) == 0 {
		t.Fatal("Diff did not notice a different answer")
	}
}
