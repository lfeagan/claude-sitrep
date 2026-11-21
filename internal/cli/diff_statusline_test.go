package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/difftest"
)

func statusScenario(setup func(home, dir string), sessions ...string) difftest.Scenario {
	return func(t testing.TB, home string, sitrep difftest.Runner) []difftest.Result {
		dir := filepath.Join(home, ".claude", "sitrep")
		setup(home, dir)
		var out []difftest.Result
		for _, sid := range sessions {
			in, _ := json.Marshal(map[string]any{"session_id": sid, "model": map[string]string{"display_name": "X"}})
			out = append(out, sitrep(t, home, "stdin:"+string(in), "statusline"))
		}
		return out
	}
}

func put(home, name, body string) {
	p := difftest.Path(home, name)
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte(body), 0o600)
}

// bashQueueLine matches the queue line as the bash version drew it, with the id last. It need not start a line: a notes file without a final
// newline runs into it.
var bashQueueLine = regexp.MustCompile(`(?m)Queue: (.*)  \[id ([A-Za-z0-9_-]+)\]((?:  Inbox: [0-9]+)?)$`)

// idFirst rewrites the bash queue line into the form the Go version draws, which puts the id right after the word Queue.
func idFirst(r difftest.Result) difftest.Result {
	r.Stdout = bashQueueLine.ReplaceAllString(r.Stdout, "Queue [id $2]: $1$3")
	return r
}

func TestDiffStatusline(t *testing.T) {
	for name, c := range map[string]struct {
		setup    func(home, dir string)
		sessions []string
	}{
		"no notes": {func(home, dir string) {}, []string{"nonotes1"}},
		"notes and queues": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\nProgress:  half\nNow:       compiling\n")
			put(home, "b0000002.txt", "Objective: x\nProgress:  -\nNow:       -\n")
			put(home, "b0000002.queue", "only one\n")
			put(home, "c0000003.txt", "Objective: y\nProgress:  -\nNow:       -\n")
			put(home, "c0000003.queue", "first item\nsecond\nthird\n")
			put(home, "d0000004.txt", "no trailing newline")
			put(home, "e0000005.txt", "Objective: z\n")
			put(home, "e0000005.queue", "\nafter a blank\n")
			put(home, "f0000006.txt", "Objective: w\n")
			put(home, "f0000006.queue", "   padded item   \n")
		}, []string{"a0000001", "b0000002", "c0000003", "d0000004", "e0000005", "f0000006"}},
		"inbox": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "inbox.queue", "i1\ni2\n")
		}, []string{"a0000001", "nonotes2"}},
		"empty inbox file": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "inbox.queue", "")
		}, []string{"a0000001"}},
		"hostile": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: a\x1b[31m\u202eb\u009b\nProgress:  p\x01\nNow:       n\tn\n")
			put(home, "a0000001.queue", "q\x1b[0m\u202e\tx"+strings.Repeat("y", 200)+"\nsecond\n")
		}, []string{"a0000001"}},
		"bad ids": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
		}, []string{"../x", "inbox", "", "a b", "a0000001"}},
		"upstream": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='echo upstream-line'\n")
		}, []string{"a0000001", "nonotes3"}},
		"upstream with colors and trailing newlines": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='printf \"\\033[32mgreen\\033[0m\\n\\n\\n\"'\n")
		}, []string{"a0000001"}},
		"upstream reads the event": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='jq -r .model.display_name'\n")
		}, []string{"a0000001"}},
		"upstream prints nothing": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='true'\n")
		}, []string{"a0000001"}},
		"upstream with two lines": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='printf \"one\\ntwo\\n\"'\n")
		}, []string{"a0000001"}},
		"upstream fails": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='exit 3'\n")
		}, []string{"a0000001"}},
		"upstream points back": {func(home, dir string) {
			put(home, "a0000001.txt", "Objective: build\n")
			put(home, "sitrep.conf", "SITREP_HUD_CMD='echo $SITREP_RENDERING'\n")
		}, []string{"a0000001"}},
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, statusScenario(c.setup, c.sessions...), goSitrep, idFirst) })
	}
}

func TestDiffStatuslineGuardAndBadInput(t *testing.T) {
	sc := func(t testing.TB, home string, sitrep difftest.Runner) []difftest.Result {
		put(home, "a0000001.txt", "Objective: build\n")
		return []difftest.Result{
			sitrep(t, home, "env:SITREP_RENDERING=1", `stdin:{"session_id":"a0000001"}`, "statusline"),
			sitrep(t, home, "stdin:", "statusline"),
			sitrep(t, home, "stdin:{not json", "statusline"),
			sitrep(t, home, "stdin:[]", "statusline"),
			sitrep(t, home, `stdin:{"session_id":"a0000001"} trailing`, "statusline"),
		}
	}
	// jq complains on standard error about input that is not JSON; the Go version is quiet. A numeric session id, which jq prints and
	// the Go version does not accept, is a deliberate difference.
	difftest.Compare(t, sc, goSitrep, idFirst, func(r difftest.Result) difftest.Result { r.Stderr = ""; return r })
}
