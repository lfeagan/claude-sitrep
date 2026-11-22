package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/difftest"
)

var attnFaded = regexp.MustCompile(`"attn_faded":\s*true,\s*`)

var emptyAlias = regexp.MustCompile(`"alias":\s*"",\s*|,\s*"theme":\s*"",\s*"colors":\s*""|,\s*"context":\s*null`)

// jsonNorm blanks what changes between two runs a second apart (the time, ages, quiet times), and appends the order of every key, so a
// change of the document's field order is a difference too.
func jsonNorm(r difftest.Result) difftest.Result {
	r.Stdout = emptyAlias.ReplaceAllString(r.Stdout, "") // a field the bash version never had; no scenario sets an alias
	r.Stdout = attnFaded.ReplaceAllString(r.Stdout, "")  // also new: the scenarios leave sessions waiting for hours
	if !strings.HasPrefix(strings.TrimSpace(r.Stdout), "{") {
		return r
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(r.Stdout), &doc); err != nil {
		return r
	}
	order := keyOrder(r.Stdout)
	gen, _ := doc["generated"].(float64)
	doc["generated"] = 0
	if ss, ok := doc["sessions"].([]any); ok {
		for _, s := range ss {
			m := s.(map[string]any)
			if changed, _ := m["state_changed"].(float64); gen-changed < 120 {
				m["state_changed"] = 0 // changed by the scenario itself, so it differs by the seconds between the two runs
			}
			m["age"] = 0
			if m["quiet_secs"] != nil {
				m["quiet_secs"] = 0
			}
		}
	}
	b, _ := json.Marshal(doc)
	r.Stdout = string(b) + "\nORDER: " + strings.Join(order, ",")
	return r
}

// keyOrder lists the object keys of a JSON document in the order they appear.
func keyOrder(doc string) []string {
	dec := json.NewDecoder(strings.NewReader(doc))
	var keys []string
	var stack []bool // true: inside an object
	expectKey := false
	for {
		tok, err := dec.Token()
		if err == io.EOF || err != nil {
			return keys
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				stack = append(stack, true)
				expectKey = true
			case '[':
				stack = append(stack, false)
				expectKey = false
			default:
				stack = stack[:len(stack)-1]
				expectKey = len(stack) > 0 && stack[len(stack)-1]
			}
		case string:
			if expectKey {
				keys = append(keys, v)
				expectKey = false
				continue
			}
			expectKey = len(stack) > 0 && stack[len(stack)-1]
		default:
			expectKey = len(stack) > 0 && stack[len(stack)-1]
		}
	}
}

// seedTime is the one moment both runs of a scenario build their files from, so their file times agree.
var seedTime = time.Now().Truncate(time.Second)

type fixture struct {
	id      string
	word    string // working, waiting, ended, or "" for no state file
	ago     time.Duration
	notes   string
	cwd     string
	queue   string
	live    string // "", "local", "foreign", "dead"
	pinned  bool
	snooze  bool
	transAg time.Duration // transcript age; 0 means no transcript
}

func procStart(t testing.TB, pid int) string {
	t.Helper()
	cmd := exec.Command("ps", "-o", "lstart=", "-p", fmt.Sprint(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("ps is not usable: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func seedWorld(t testing.TB, home string, withRegistry bool) {
	t.Helper()
	sess := filepath.Join(home, ".claude", "sessions")
	proj := filepath.Join(home, ".claude", "projects", "-tmp-p")
	for _, d := range []string{sess, proj} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	now := seedTime
	ago := func(h float64) time.Duration { return time.Duration(h * float64(time.Hour)) }
	hostile := "Objective: q\"uote back\\slash\ttab \x1b[31mred\x1b]0;x\x07 \u202ebidi \xff\xfe bad\nProgress:  p\x01\x1fp\nNow:       " + strings.Repeat("n", 250) + "\n"
	fx := []fixture{
		{id: "run00001", word: "working", ago: ago(5.33), notes: "Objective: build it\nProgress:  half\nNow:       compiling\n", cwd: "/home/u/proj-one\n", queue: "first\nsecond item\n  third\n", live: "local", transAg: ago(2.2)},
		{id: "wait0002", word: "waiting", ago: ago(7.4), notes: "Objective: wait\nProgress:  p\nNow:       n\n", cwd: "/work/two/\n", queue: "x\n", live: "local"},
		{id: "snz00003", word: "waiting", ago: ago(6.1), notes: "Objective: snoozed\nProgress:  p\nNow:       n\n", cwd: "/a/b/three\n", live: "local", snooze: true},
		{id: "end00004", word: "ended", ago: ago(3.4), notes: "Objective: done\nProgress:  p\nNow:       n\n", cwd: "/a/four\n", queue: "stranded one\nstranded two\n"},
		{id: "old00005", word: "working", ago: ago(4.7), notes: "Objective: gone quiet\nProgress:  p\nNow:       n\n", cwd: "/a/five\n", queue: "left behind\n"},
		{id: "foreign6", word: "working", ago: ago(8.2), notes: "Objective: elsewhere\nProgress:  p\nNow:       n\n", cwd: "/a/six\n", live: "foreign"},
		{id: "pinned07", word: "working", ago: ago(9.3), notes: "Objective: pinned goal\nProgress:  p\nNow:       n\n", cwd: "/a/seven\n", live: "local", pinned: true, transAg: ago(9.1)},
		{id: "ancient8", word: "working", ago: ago(50), notes: "Objective: too old\nProgress:  p\nNow:       n\n", cwd: "/a/eight\n", live: "local"},
		{id: "nostate9", ago: ago(10.2), notes: "Objective: no state file\nProgress:  p\nNow:       n\n", cwd: "/a/nine\n", live: "local"},
		{id: "hostile0", word: "working", ago: ago(11.6), notes: hostile, cwd: "/x/ho\"stile\\dir\t\x1b[0m \n", queue: "item\"1\\\twith\x1funit\n\n   \n\x1b]0;t\aok\n", live: "local"},
		{id: "ended0ff", word: "ended", ago: ago(12.7), notes: "Objective: ended but alive\nProgress:  p\nNow:       n\n", cwd: "/a/ten\n", live: "local"},
	}
	for i, f := range fx {
		write := func(name, body string) {
			if body == "" {
				return
			}
			difftest.Put(t, home, name, body)
		}
		write(f.id+".txt", f.notes)
		write(f.id+".cwd", f.cwd)
		write(f.id+".queue", f.queue)
		mt := now.Add(-f.ago)
		if f.word != "" {
			write("."+f.id+".state", f.word+"\n")
			if err := os.Chtimes(difftest.Path(home, "."+f.id+".state"), mt, mt); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(difftest.Path(home, f.id+".txt"), mt, mt); err != nil {
			t.Fatal(err)
		}
		if f.pinned {
			write("."+f.id+".pinned", "x")
		}
		if f.snooze {
			write("."+f.id+".snooze", fmt.Sprintf("%d\n", mt.Unix()))
		}
		if f.transAg > 0 {
			tr := filepath.Join(proj, f.id+".jsonl")
			if err := os.WriteFile(tr, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			tm := now.Add(-f.transAg)
			if err := os.Chtimes(tr, tm, tm); err != nil {
				t.Fatal(err)
			}
		}
		if withRegistry && f.live != "" {
			reg := map[string]any{"sessionId": f.id, "kind": "interactive"}
			switch f.live {
			case "local":
				reg["pid"], reg["procStart"] = os.Getpid(), procStart(t, os.Getpid())
			case "foreign":
				reg["pid"], reg["procStart"], reg["pidDomain"] = os.Getpid(), "whatever", "some-other-pid-space"
			}
			b, _ := json.Marshal(reg)
			if err := os.WriteFile(filepath.Join(sess, fmt.Sprintf("%d.json", 1000+i)), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	// a registry entry for a dead process, one for a reused pid, and a malformed file
	if withRegistry {
		for name, body := range map[string]string{
			"dead.json":   fmt.Sprintf(`{"pid":4194000,"sessionId":"old00005","procStart":"x"}`),
			"reused.json": fmt.Sprintf(`{"pid":%d,"sessionId":"end00004","procStart":"Thu Jan  1 00:00:00 1970"}`, os.Getpid()),
			"bad.json":    `{not json`,
			"evil.json":   `{"pid":"12; rm -rf /","sessionId":"../x","procStart":"x"}`,
		} {
			if err := os.WriteFile(filepath.Join(sess, name), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, body := range map[string]string{"inbox.queue": "inbox one\ninbox two\n", "bad name.txt": "junk\n", "x.y.txt": "junk\n"} {
		difftest.Put(t, home, name, body)
	}
}

func world(withRegistry bool, steps ...string) difftest.Scenario {
	return worldWith(withRegistry, true, steps...)
}

// tidyWorld is world without the two files that are not sessions (the bash listing shows them as rows, which the Go one deliberately
// does not, since they are not ours) and without the hostile session.
func tidyWorld(withRegistry bool, steps ...string) difftest.Scenario {
	return worldWith(withRegistry, false, steps...)
}

func worldWith(withRegistry, junk bool, steps ...string) difftest.Scenario {
	return func(t testing.TB, home string, sitrep difftest.Runner) []difftest.Result {
		seedWorld(t, home, withRegistry)
		if !junk {
			os.Remove(difftest.Path(home, "bad name.txt"))
			os.Remove(difftest.Path(home, "x.y.txt"))
			// hand-edited text (control characters, blank lines, a field over 200 characters) is drawn from the cleaned snapshot, which
			// differs from the bash on purpose; the hostile session is covered by the JSON and render tests
			for _, ext := range []string{".txt", ".cwd", ".queue"} {
				os.Remove(difftest.Path(home, "hostile0"+ext))
			}
			os.Remove(difftest.Path(home, ".hostile0.state"))
		}
		var out []difftest.Result
		for _, l := range steps {
			out = append(out, sitrep(t, home, strings.Split(l, "|")...))
		}
		return out
	}
}

func TestDiffJSON(t *testing.T) {
	for name, sc := range map[string]difftest.Scenario{
		"with registry": world(true, "ls|--json", "ls|--json|-a", "ls|--json|-a|-n|1", "ls|--json|--full", "ls|--json|-n|0", "ls|--json|-a|--full"),
		"no registry":   world(false, "ls|--json", "ls|--json|-a"),
		"errors":        world(true, "ls|--json|abcd", "ls|--full", "ls|abcd|--json", "ls|--json|-n", "ls|--json|-n|x", "ls|-n|-1|--json"),
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, sc, goSitrep, jsonNorm) })
	}
}

func TestDiffSnoozeAndPrune(t *testing.T) {
	var _ = bytes.NewReader
	for name, sc := range map[string]difftest.Scenario{
		"snooze":            world(true, "snooze|wait", "ls|--json", "snooze|--off|wait", "ls|--json", "snooze|inbox", "snooze|nope", "snooze", "snooze|--off", "snooze|a|b", "snooze|end0", "snooze|--off|end0"),
		"prune dry run":     world(true, "prune|--dry-run", "ls|--json|-a"),
		"prune keep":        world(true, "prune|-k", "ls|--json|-a", "prune|-k"),
		"prune yes":         world(true, "prune|-y", "ls|--json|-a", "prune|-y"),
		"prune ask":         world(true, "prune"),
		"prune usage":       world(true, "prune|-y|-k", "prune|--bogus", "prune|-y|--dry-run"),
		"prune no registry": world(false, "prune", "prune|-y"),
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, sc, goSitrep, jsonNorm) })
	}
}

func TestDiffPlainListing(t *testing.T) {
	for name, sc := range map[string]difftest.Scenario{
		"default":     tidyWorld(true, "ls", "ls|-a", "env:COLUMNS=40|ls", "env:COLUMNS=60|ls|-a|-v", "env:COLUMNS=9|ls"),
		"verbose":     tidyWorld(true, "ls|-v", "ls|-v|-a", "ls|-n|2", "ls|-n|0", "ls|-a|-v|-n|1", "ls|-n|99", "env:COLUMNS=50|ls|-n|3"),
		"no registry": tidyWorld(false, "ls", "ls|-v", "ls|-a|-n|2"),
		"one queue":   tidyWorld(true, "ls|inbox", "ls|run0", "ls|wait", "ls|end0", "ls|zzzz", "ls|nope", "ls|../x", "ls|a|b", "ls|-v|run0"),
		"errors":      tidyWorld(true, "ls|-n", "ls|-n|x", "ls|--full", "ls|-n|-1"),
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, sc, goSitrep, jsonNorm) })
	}
}
