package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/difftest"
)

// hookWorld is one run of a hook scenario: a home with a stub claude, and a transcript for each session.
type hookWorld struct {
	t       testing.TB
	home    string
	sitrep  difftest.Runner
	results []difftest.Result
}

const stubClaude = `#!/bin/bash
{ echo ARGS:; printf '[%s]\n' "$@"; echo "SUMMARIZING=$SITREP_SUMMARIZING"; echo STDIN:; cat; echo END; } >> "$STUB_REC"
echo x >> "$STUB_CALLS"
[ -e "$STUB_SLOW" ] && sleep 1
printf 'Objective: stub objective\nProgress: stub progress\nNow: stub now\n'
`

func newHookWorld(t testing.TB, home string, sitrep difftest.Runner) *hookWorld {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "claude"), []byte(stubClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	return &hookWorld{t: t, home: home, sitrep: sitrep}
}

func (w *hookWorld) state() string { return filepath.Join(w.home, ".claude", "sitrep") }

func (w *hookWorld) transcript(sid string, lines ...string) string {
	dir := filepath.Join(w.home, ".claude", "projects", "-proj")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, sid+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		w.t.Fatal(err)
	}
	return p
}

func jsonl(typ string, content any) string {
	b, _ := json.Marshal(map[string]any{"type": typ, "message": map[string]any{"content": content}})
	return string(b)
}

// fire sends one event to the hook, with extra environment (NAME=value) after the stub's.
func (w *hookWorld) fire(event map[string]any, env ...string) {
	w.t.Helper()
	b, _ := json.Marshal(event)
	args := []string{"env:PATH=" + filepath.Join(w.home, "bin") + ":" + os.Getenv("PATH"), "env:STUB_CALLS=" + filepath.Join(w.home, "calls"),
		"env:STUB_SLOW=" + filepath.Join(w.home, "slow"), "env:STUB_REC=" + filepath.Join(w.home, "rec")}
	for _, e := range env {
		args = append(args, "env:"+e)
	}
	args = append(args, "stdin:"+string(b), "hook")
	w.results = append(w.results, w.sitrep(w.t, w.home, args...))
}

// rec is what the stub claude saw: its arguments, whether it was told it is the summarizer, and the prompt it was given.
func (w *hookWorld) rec() string {
	b, _ := os.ReadFile(filepath.Join(w.home, "rec"))
	return string(b)
}

func (w *hookWorld) calls() int {
	b, _ := os.ReadFile(filepath.Join(w.home, "calls"))
	return strings.Count(string(b), "x")
}

// settle waits until the stub has been run n times and its answer is in the notes, and nothing is pending.
func (w *hookWorld) settle(sid string, n int) {
	w.t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(w.state(), sid+".txt"))
		_, pend := os.Stat(filepath.Join(w.state(), "."+sid+".pending"))
		if w.calls() >= n && strings.Contains(string(b), "stub progress") && pend != nil {
			time.Sleep(150 * time.Millisecond)
			w.results = append(w.results, difftest.Result{Stdout: fmt.Sprintf("calls=%d\n%s", w.calls(), w.rec())})
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	w.results = append(w.results, difftest.Result{Stdout: fmt.Sprintf("timed out at calls=%d", w.calls())})
}

func ev(sid, event string, kv ...any) map[string]any {
	m := map[string]any{"session_id": sid, "hook_event_name": event, "cwd": "/tmp/proj"}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func hookScenario(f func(w *hookWorld)) difftest.Scenario {
	return func(t testing.TB, home string, sitrep difftest.Runner) []difftest.Result {
		w := newHookWorld(t, home, sitrep)
		f(w)
		return w.results
	}
}

// stateNorm drops the stamp, which holds nothing, and blanks nothing else: the hook's files are compared whole.
func TestDiffHookPrompts(t *testing.T) {
	for name, f := range map[string]func(w *hookWorld){
		"plain prompts": func(w *hookWorld) {
			w.fire(ev("sess0001", "SessionStart", "source", "startup"))
			w.fire(ev("sess0001", "UserPromptSubmit", "prompt", "fix the flaky test"))
			w.fire(ev("sess0001", "UserPromptSubmit", "prompt", "second prompt\nwith two lines"))
			w.fire(ev("sess0001", "Stop"))
			w.fire(ev("sess0001", "SessionEnd"))
			w.fire(ev("sess0002", "UserPromptSubmit", "prompt", strings.Repeat("long ", 100)))
			w.fire(ev("sess0003", "UserPromptSubmit", "prompt", "trailing newlines\n\n"))
			w.fire(map[string]any{"session_id": "sess0004", "hook_event_name": "UserPromptSubmit", "prompt": "no cwd"})
		},
		"placeholder objective": func(w *hookWorld) {
			w.fire(ev("sess0005", "UserPromptSubmit", "prompt", "<command-name>/clear</command-name>"))
			w.fire(ev("sess0005", "UserPromptSubmit", "prompt", "real work now"))
			w.fire(ev("sess0006", "UserPromptSubmit", "prompt", "&lt;command-message&gt;x"))
			w.fire(ev("sess0006", "UserPromptSubmit", "prompt", "after"))
		},
		"plus commands": func(w *hookWorld) {
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "start"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+ write the docs"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+ second item"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+pin The pinned goal"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+inbox   look at nightly job"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+inbox another"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+take"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+take 1"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+take 1"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+take x"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+take 0"))
			w.fire(ev("sess0007", "UserPromptSubmit", "prompt", "+take  2"))
			w.fire(ev("sess0008", "UserPromptSubmit", "prompt", "+ first thing in a fresh session"))
			w.fire(ev("sess0009", "UserPromptSubmit", "prompt", "+take"))
		},
		"plus commands that go to claude": func(w *hookWorld) {
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "start"))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+ line one\nline two"))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+ "+strings.Repeat("x", 201)))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+ "+strings.Repeat("x", 200)))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+ "))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+"))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+pin "))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+pin   "))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+inbox "))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+pinned goal"))
			w.fire(ev("sess0010", "UserPromptSubmit", "prompt", "+takeout"))
		},
		"hostile text": func(w *hookWorld) {
			w.fire(ev("sess0011", "UserPromptSubmit", "prompt", "hi \x1b[31mRED\x1b]0;title\a there \u009b31m \u202eX \u200d\ufeff tab\there", "cwd", "/x/\x1b[0mdir\u202e"))
			w.fire(ev("sess0011", "UserPromptSubmit", "prompt", "+ q\x1b[0m\u009f \u202e item"))
			w.fire(ev("sess0011", "UserPromptSubmit", "prompt", "+pin pin\x1b[0m\u202e text"))
			w.fire(ev("sess0011", "UserPromptSubmit", "prompt", "+inbox in\x07box"))
			w.fire(ev("sess0012", "UserPromptSubmit", "prompt", "héllo 世界 \U0001F600"))
		},
		"bad input": func(w *hookWorld) {
			w.fire(ev("../evil", "UserPromptSubmit", "prompt", "x"))
			w.fire(ev("inbox", "UserPromptSubmit", "prompt", "x"))
			w.fire(ev("", "UserPromptSubmit", "prompt", "x"))
			w.fire(ev("a b", "UserPromptSubmit", "prompt", "x"))
			w.fire(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "no id"})
			w.fire(ev("sess0013", "SomethingElse", "prompt", "x"))
			w.fire(ev("sess0013", "UserPromptSubmit", "prompt", "x"), "SITREP_SUMMARIZING=1")
			w.results = append(w.results, w.sitrep(w.t, w.home, "stdin:{not json", "hook"))
			w.results = append(w.results, w.sitrep(w.t, w.home, "stdin:", "hook"))
		},
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, hookScenario(f), goSitrep) })
	}
}

func TestDiffHookStates(t *testing.T) {
	for name, f := range map[string]func(w *hookWorld){
		"start stop end": func(w *hookWorld) {
			w.fire(ev("sess0020", "SessionStart", "source", "startup"))
			w.fire(ev("sess0020", "UserPromptSubmit", "prompt", "go"))
			w.fire(ev("sess0020", "SessionStart", "source", "compact"))
			w.fire(ev("sess0020", "Stop"))
			w.fire(ev("sess0020", "SessionStart", "source", "resume"))
			w.fire(ev("sess0020", "SessionEnd", "cwd", "/ignored"))
		},
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, hookScenario(f), goSitrep) })
	}
}

func TestDiffHookSummarizer(t *testing.T) {
	user := func(s string) string { return jsonl("user", s) }
	for name, f := range map[string]func(w *hookWorld){
		"stop summarizes": func(w *hookWorld) {
			tp := w.transcript("sess0030", user("build it"), jsonl("assistant", "ok building"))
			w.fire(ev("sess0030", "UserPromptSubmit", "prompt", "build it", "transcript_path", tp))
			w.fire(ev("sess0030", "Stop", "transcript_path", tp))
			w.settle("sess0030", 1)
		},
		"stop inside the window is deferred": func(w *hookWorld) {
			tp := w.transcript("sess0031", user("build it"))
			w.fire(ev("sess0031", "UserPromptSubmit", "prompt", "build it", "transcript_path", tp))
			w.fire(ev("sess0031", "Stop", "transcript_path", tp), "SITREP_DEBOUNCE=3")
			w.settle("sess0031", 1)
			w.fire(ev("sess0031", "Stop", "transcript_path", tp), "SITREP_DEBOUNCE=3")
			w.fire(ev("sess0031", "Stop", "transcript_path", tp), "SITREP_DEBOUNCE=3") // already pending
			w.settle("sess0031", 2)
		},
		"a newer prompt drops the summary": func(w *hookWorld) {
			os.WriteFile(filepath.Join(w.home, "slow"), nil, 0o600)
			tp := w.transcript("sess0032", user("first"))
			w.fire(ev("sess0032", "UserPromptSubmit", "prompt", "first", "transcript_path", tp))
			w.fire(ev("sess0032", "Stop", "transcript_path", tp))
			w.fire(ev("sess0032", "UserPromptSubmit", "prompt", "second, while it was summarizing", "transcript_path", tp))
			time.Sleep(4 * time.Second)
			w.results = append(w.results, difftest.Result{Stdout: fmt.Sprintf("calls=%d\n%s", w.calls(), w.rec())})
		},
		"pinned objective survives": func(w *hookWorld) {
			tp := w.transcript("sess0033", user("x"))
			w.fire(ev("sess0033", "UserPromptSubmit", "prompt", "x", "transcript_path", tp))
			w.fire(ev("sess0033", "UserPromptSubmit", "prompt", "+pin keep this goal", "transcript_path", tp))
			w.fire(ev("sess0033", "Stop", "transcript_path", tp))
			w.settle("sess0033", 1)
		},
		"resume seeds the objective": func(w *hookWorld) {
			tp := w.transcript("sess0034", user("<command-name>/clear</command-name>"), user("  \n\n  the real first line\nsecond line"), user("later"))
			w.fire(ev("sess0034", "SessionStart", "source", "resume", "transcript_path", tp))
			w.settle("sess0034", 1)
			w.fire(ev("sess0035", "SessionStart", "source", "resume", "transcript_path", w.transcript("sess0035", user("<command-name>x</command-name>"))))
		},
		"an unsafe transcript path is not read": func(w *hookWorld) {
			outside := filepath.Join(w.home, "outside.jsonl")
			os.WriteFile(outside, []byte(user("from outside")+"\n"), 0o600)
			w.fire(ev("sess0036", "UserPromptSubmit", "prompt", "the prompt", "transcript_path", outside))
			w.fire(ev("sess0036", "Stop", "transcript_path", outside))
			real := w.transcript("sess0037", user("secret"))
			link := filepath.Join(w.home, ".claude", "projects", "-proj", "link0037.jsonl")
			os.Symlink(real, link)
			w.fire(ev("sess0037", "UserPromptSubmit", "prompt", "via link", "transcript_path", link))
			w.fire(ev("sess0037", "Stop", "transcript_path", link))
			dots := filepath.Join(w.home, ".claude", "projects", "-proj", "..", "-proj", "sess0037.jsonl")
			w.fire(ev("sess0038", "UserPromptSubmit", "prompt", "via dots", "transcript_path", dots))
			w.fire(ev("sess0038", "Stop", "transcript_path", dots))
			time.Sleep(500 * time.Millisecond)
			w.results = append(w.results, difftest.Result{Stdout: fmt.Sprintf("calls=%d", w.calls())})
		},
	} {
		t.Run(name, func(t *testing.T) { difftest.Compare(t, hookScenario(f), goSitrep) })
	}
}

// The hook must return at once: the summarizer is a detached child, and Claude Code waits for the hook, not for it.
func TestHookReturnsBeforeTheSummarizerFinishes(t *testing.T) {
	home := difftest.Home(t)
	w := newHookWorld(t, home, goSitrep)
	if err := os.WriteFile(filepath.Join(home, "bin", "claude"), []byte("#!/bin/bash\ncat >/dev/null\nsleep 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tp := w.transcript("sess0040", jsonl("user", "work"))
	start := time.Now()
	w.fire(ev("sess0040", "Stop", "transcript_path", tp), "SITREP_SUMMARIZE_TIMEOUT=2")
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("the hook took %v: it waited for the summarizer", d)
	}
	time.Sleep(3500 * time.Millisecond) // let the child finish before the temporary folder goes
}

// The deadline ends claude and everything it started.
func TestSummarizerDeadlineKillsTheWholeGroup(t *testing.T) {
	home := difftest.Home(t)
	w := newHookWorld(t, home, goSitrep)
	pidFile := filepath.Join(home, "grandchild.pid")
	script := "#!/bin/bash\ncat >/dev/null\nsleep 31 &\necho $! > " + pidFile + "\nsleep 30\n"
	os.WriteFile(filepath.Join(home, "bin", "claude"), []byte(script), 0o755)
	tp := w.transcript("sess0041", jsonl("user", "work"))
	w.fire(ev("sess0041", "Stop", "transcript_path", tp), "SITREP_SUMMARIZE_TIMEOUT=1")
	time.Sleep(3500 * time.Millisecond)
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the stub never started its child: %v", err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		t.Errorf("a process started by claude (pid %d) outlived the deadline", pid)
		syscall.Kill(pid, syscall.SIGKILL)
	}
}
