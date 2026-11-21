package statusline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statetest"
)

func render(t *testing.T, files map[string]string, input string, run runner.Runner) string {
	t.Helper()
	s := state.Open(t.TempDir())
	s.Ensure()
	for name, body := range files {
		statetest.Put(t, s, name, body, 0)
	}
	l := Line{Store: s, Conf: conf.Conf{File: filepath.Join(s.Dir, "sitrep.conf"), Getenv: func(string) string { return "" }}, Run: run, Env: []string{"A=1"}}
	var out bytes.Buffer
	l.Render(context.Background(), strings.NewReader(input), &out)
	return out.String()
}

func TestRender(t *testing.T) {
	got := render(t, map[string]string{
		"s1.txt":      "Objective: build\nProgress:  half\nNow:       compiling\n",
		"s1.queue":    "first item\nsecond\n",
		"inbox.queue": "x\n",
	}, `{"session_id":"s1"}`, &runner.Fake{})
	want := "Objective: build\nProgress:  half\nNow:       compiling\nQueue: 2 items, next: first item  [id s1]  Inbox: 1\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNoNotesAndNoQueue(t *testing.T) {
	got := render(t, nil, `{"session_id":"abcdef"}`, &runner.Fake{})
	if got != "Objective: (set on your first prompt)\nQueue: empty  [id abcd]\n" {
		t.Errorf("%q", got)
	}
}

func TestNothingForAnUnusableSession(t *testing.T) {
	for _, in := range []string{"", "{", "[]", `{"session_id":"../x"}`, `{"session_id":"inbox"}`, `{"session_id":7}`, `{}`} {
		if got := render(t, map[string]string{"x.txt": "Objective: x\n"}, in, &runner.Fake{}); got != "" {
			t.Errorf("%q printed %q", in, got)
		}
	}
}

func TestFilesFromSessionsAreFiltered(t *testing.T) {
	got := render(t, map[string]string{
		"s1.txt":   "Objective: a\x1b[31m\u202eb\nNow: n\tn\n",
		"s1.queue": "q\x1b[0m\u202e\t" + strings.Repeat("y", 200) + "\n",
	}, `{"session_id":"s1"}`, &runner.Fake{})
	if strings.ContainsAny(got, "\x1b\u202e") || !strings.Contains(got, "n\tn") {
		t.Errorf("%q", got)
	}
	next := got[strings.Index(got, "next: ")+6 : strings.Index(got, "  [id")]
	if len([]rune(next)) != 90 {
		t.Errorf("the next item is %d characters, want 90", len([]rune(next)))
	}
}

func TestUpstreamComesFirstAndKeepsItsColors(t *testing.T) {
	fake := &runner.Fake{Answers: map[string]runner.Result{"sh -c echo up": {Out: []byte("\x1b[32mgreen\x1b[0m\n\n\n")}}}
	s := state.Open(t.TempDir())
	s.Ensure()
	os.WriteFile(filepath.Join(s.Dir, "sitrep.conf"), []byte("SITREP_HUD_CMD='echo up'\n"), 0o600)
	l := Line{Store: s, Conf: conf.Open(s.Dir), Run: fake, Env: []string{"A=1"}}
	var out bytes.Buffer
	l.Render(context.Background(), strings.NewReader(`{"session_id":"s1"}`), &out)
	if !strings.HasPrefix(out.String(), "\x1b[32mgreen\x1b[0m\nObjective:") {
		t.Errorf("%q", out.String())
	}
	c := fake.Calls()[0]
	if string(mustRead(t, c.Stdin)) != `{"session_id":"s1"}` || !hasEnv(c.Env, "SITREP_RENDERING=1") || !hasEnv(c.Env, "A=1") {
		t.Errorf("upstream call %+v", c)
	}
}

func mustRead(t *testing.T, r interface{ Read([]byte) (int, error) }) []byte {
	t.Helper()
	var b bytes.Buffer
	b.ReadFrom(r)
	return b.Bytes()
}

func hasEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}

func TestAPipeAsTheQueueDoesNotHang(t *testing.T) {
	// covered by state.ReadFile; here the whole line must still come out
	got := render(t, map[string]string{"s1.txt": "Objective: x\n"}, `{"session_id":"s1"}`, &runner.Fake{})
	if !strings.HasSuffix(got, "[id s1]\n") {
		t.Errorf("%q", got)
	}
}

func TestNoticeOfANewerRelease(t *testing.T) {
	s := state.Open(t.TempDir())
	s.Ensure()
	os.WriteFile(filepath.Join(s.Dir, "update"), []byte("1 0.6.0\n"), 0o600)
	draw := func(version string) string {
		l := Line{Store: s, Conf: conf.Conf{File: filepath.Join(s.Dir, "sitrep.conf"), Getenv: func(string) string { return "" }}, Run: &runner.Fake{}, Version: version}
		var out bytes.Buffer
		l.Render(context.Background(), strings.NewReader(`{"session_id":"abcdef"}`), &out)
		return out.String()
	}
	want := "Objective: (set on your first prompt)\nQueue: empty  [id abcd]\n"
	if got := draw("0.5.0"); !strings.HasPrefix(got, want) || !strings.HasSuffix(got, "\n") || !strings.Contains(strings.TrimPrefix(got, want), "0.6.0") {
		t.Errorf("older: %q", got)
	}
	for _, v := range []string{"0.6.0", "0.7.0", ""} {
		if got := draw(v); got != want {
			t.Errorf("version %q: %q", v, got)
		}
	}
}
