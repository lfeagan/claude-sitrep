package clitest

import "testing"

func TestSteadyWritesTheClockSeededNumbersAsPlaceholders(t *testing.T) {
	for in, want := range map[string]string{
		"1795303538\n":                     "<epoch>\n",
		"1795303538":                       "<epoch>\n",
		"1795303538 0123456789abcdef\n":    "<epoch> <nonce>\n",
		"62.5 1000000 625000 1795303538\n": "62.5 1000000 625000 <epoch>\n",
		"Objective: build\n":               "Objective: build\n",
		"waiting\n":                        "waiting\n",
		"3\n":                              "3\n", // a count is not a time
		"":                                 "",
	} {
		if got := steady(in); got != want {
			t.Errorf("steady(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathMapsTheOldFlatNamesToTheSessionFolders(t *testing.T) {
	home := Home(t)
	for name, want := range map[string]string{
		"abcd1111.txt":    "/.claude/sitrep/sessions/abcd1111/notes",
		".abcd1111.state": "/.claude/sitrep/sessions/abcd1111/state",
		"abcd1111.queue":  "/.claude/sitrep/sessions/abcd1111/queue",
		"inbox.queue":     "/.claude/sitrep/inbox.queue",
		"sitrep.conf":     "/.claude/sitrep/sitrep.conf",
	} {
		if got := Path(home, name); got != home+want {
			t.Errorf("Path(%q) = %q, want %q", name, got, home+want)
		}
	}
}

func TestStateReadsTheFilesByTheirFlatNames(t *testing.T) {
	home := Home(t)
	Put(t, home, "abcd1111.txt", "Objective: x\n")
	Put(t, home, ".abcd1111.state", "waiting\n")
	Put(t, home, "inbox.queue", "one\n")
	got := State(t, home)
	if len(got) != 3 || got["abcd1111.txt"].Content != "Objective: x\n" || got[".abcd1111.state"].Content != "waiting\n" || got["inbox.queue"].Mode != 0o600 {
		t.Errorf("state: %+v", got)
	}
}
