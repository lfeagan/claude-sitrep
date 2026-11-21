package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/difftest"
)

var goSitrep difftest.Runner

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sitrep-cli-test")
	if err != nil {
		panic(err)
	}
	if goSitrep, err = difftest.BuildInto(dir); err != nil {
		os.RemoveAll(dir)
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// seed makes session files, so that ids resolve: two that share a prefix, and one on its own.
func seed(t testing.TB, home string, objective string) {
	t.Helper()
	for _, id := range []string{"abcd1111", "abce2222", "zzzz3333"} {
		difftest.Put(t, home, id+".txt", objective)
	}
}

func scenario(steps ...[]string) difftest.Scenario {
	return func(t testing.TB, home string, sitrep difftest.Runner) []difftest.Result {
		seed(t, home, "Objective: old goal\nProgress:  p\nNow:       n\n")
		var out []difftest.Result
		for _, s := range steps {
			out = append(out, sitrep(t, home, s...))
		}
		return out
	}
}

func cmds(lines ...string) [][]string {
	var out [][]string
	for _, l := range lines {
		out = append(out, strings.Split(l, "|"))
	}
	return out
}

func TestDiffQueueCommands(t *testing.T) {
	long := strings.Repeat("x", 250)
	for name, steps := range map[string][][]string{
		"add": cmds("add|zz|first", "add|zz|second one", "add|inbox|to the inbox", "add|zz|joined|words|here", "add|zz|", "add|zz|   ",
			"add|zz|tab\there\x1b[31mred\u202e", "add|zz|"+long, "add|abc|x", "add|nope|x", "add|../x|x", "add|zz", "add", "add|inbox|世界 emoji \U0001F600"),
		"rm": cmds("add|zz|a", "add|zz|b", "add|zz|c", "rm|zz", "rm|zz|2", "rm|zz|9", "rm|zz|0", "rm|zz|x", "rm|zz|-1", "rm|zz|1", "rm|zz|1", "rm|nope", "rm",
			"rm|inbox", "add|inbox|q", "rm|inbox|1|--item|q", "add|zz|z", "rm|zz|1|--item|wrong", "rm|zz|1|--item|z  ", "rm|zz|--item", "rm|zz|--item|a|--item|b"),
		"top": cmds("add|zz|a", "add|zz|b", "add|zz|c", "top|zz|3", "top|zz|1", "top|zz|4", "top|zz|0", "top|zz|b", "top|zz", "top|zz|2|--item|a", "top|zz|2|--item|nope"),
		"reorder": cmds("add|zz|a", "add|zz|b", "add|zz|c", "add|zz|d", "reorder|zz|1|3", "reorder|zz|4|1", "reorder|zz|2|2", "reorder|zz|1|9", "reorder|zz|9|1",
			"reorder|zz|1", "reorder|zz|1|2|--item|b", "reorder|zz|1|2|--item|zzz", "reorder|zz|x|1"),
		"move": cmds("add|zz|a", "add|zz|b", "add|zz|c", "move|zz|inbox", "move|zz|inbox|2", "move|inbox|abcd", "move|inbox|abcd|1|--item|nope", "move|inbox|abcd|--item|a",
			"move|zz|zz", "move|zz|inbox|9", "move|zz|inbox|0", "move|zz|inbox|x", "move|zz", "move|zz|inbox|1|extra", "move|zz|inbox|--item", "move|nope|inbox", "move|zz|nope",
			"move|inbox|zz|1|--item|a|b"),
		"pin":  cmds("pin|zz|ship the thing", "pin|zz|   ", "pin|inbox|x", "pin|abcd|x", "pin|abcd1|multi|word|goal\x1b[0m\u202e", "pin|nope|x", "pin|zz", "unpin|zz", "unpin|zz", "unpin|inbox", "unpin", "unpin|nope"),
		"help": cmds("add|--help", "rm|-h", "top|--help", "reorder|--help", "move|--help", "pin|--help", "unpin|--help", "help|--help", "nosuch|--help", "nosuch", "help|nosuch"),
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, scenario(steps...), goSitrep) })
	}
}
