package statusline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	want := "Objective: build\nProgress:  half\nNow:       compiling\nQueue [id s1]: 2 items, next: first item  Inbox: 1\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNoNotesAndNoQueue(t *testing.T) {
	got := render(t, nil, `{"session_id":"abcdef"}`, &runner.Fake{})
	if got != "Objective: (set on your first prompt)\nQueue [id abcd]: empty\n" {
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
	next := strings.TrimSuffix(got[strings.Index(got, "next: ")+6:], "\n")
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
	if !strings.HasSuffix(got, "Queue [id s1]: empty\n") {
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
	want := "Objective: (set on your first prompt)\nQueue [id abcd]: empty\n"
	if got := draw("0.5.0"); !strings.HasPrefix(got, want) || !strings.HasSuffix(got, "\n") || !strings.Contains(strings.TrimPrefix(got, want), "0.6.0") {
		t.Errorf("older: %q", got)
	}
	for _, v := range []string{"0.6.0", "0.7.0", ""} {
		if got := draw(v); got != want {
			t.Errorf("version %q: %q", v, got)
		}
	}
}

func TestAliasAfterTheId(t *testing.T) {
	s := state.Open(t.TempDir())
	s.Ensure()
	s.WriteFile(s.Path("abcdef", "notes"), []byte("Objective: x\n"))
	if _, err := s.SetAlias("abcdef", "work"); err != nil {
		t.Fatal(err)
	}
	l := Line{Store: s, Conf: conf.Conf{File: filepath.Join(s.Dir, "sitrep.conf"), Getenv: func(string) string { return "" }}, Run: &runner.Fake{}}
	var out bytes.Buffer
	l.Render(context.Background(), strings.NewReader(`{"session_id":"abcdef"}`), &out)
	if !strings.HasSuffix(out.String(), "Queue [id abcd work]: empty\n") {
		t.Errorf("%q", out.String())
	}
	os.WriteFile(s.Path("abcdef", "alias"), []byte("bad\x1b[31m\n"), 0o600) // a hand-edited file that is no alias
	out.Reset()
	l.Render(context.Background(), strings.NewReader(`{"session_id":"abcdef"}`), &out)
	if strings.ContainsAny(out.String(), "\x1b") || !strings.HasSuffix(out.String(), "Queue [id abcd]: empty\n") {
		t.Errorf("%q", out.String())
	}
}

var ctxNow = time.Unix(1_800_000_000, 0)

func TestContextOf(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want state.Context
		ok   bool
	}{
		"percent and size":         {`{"context_window":{"used_percentage":37.5,"context_window_size":1000000,"current_usage":{"input_tokens":5,"cache_creation_input_tokens":70000,"cache_read_input_tokens":304995}}}`, state.Context{UsedPercent: 37.5, Window: 1_000_000, Tokens: 375_000, At: 1_800_000_000}, true},
		"percent only":             {`{"context_window":{"used_percentage":45,"context_window_size":200000}}`, state.Context{UsedPercent: 45, Window: 200_000, At: 1_800_000_000}, true},
		"zero percent, real usage": {`{"context_window":{"used_percentage":0,"context_window_size":200000,"current_usage":{"input_tokens":50000,"cache_read_input_tokens":50000}}}`, state.Context{UsedPercent: 50, Window: 200_000, Tokens: 100_000, At: 1_800_000_000}, true},
		"null percent, real usage": {`{"context_window":{"used_percentage":null,"context_window_size":200000,"current_usage":{"input_tokens":20000}}}`, state.Context{UsedPercent: 10, Window: 200_000, Tokens: 20_000, At: 1_800_000_000}, true},
		"window and no usage":      {`{"context_window":{"used_percentage":null,"context_window_size":200000,"current_usage":null}}`, state.Context{Window: 200_000, At: 1_800_000_000}, true},
		"zero percent, no usage":   {`{"context_window":{"used_percentage":0,"context_window_size":1000000}}`, state.Context{Window: 1_000_000, At: 1_800_000_000}, true},
		"usage over the window":    {`{"context_window":{"context_window_size":1000,"current_usage":{"input_tokens":5000}}}`, state.Context{UsedPercent: 100, Window: 1000, Tokens: 5000, At: 1_800_000_000}, true},
		"no window":                {`{"context_window":{"used_percentage":45}}`, state.Context{}, false},
		"no context_window":        {`{"session_id":"x"}`, state.Context{}, false},
		"null context_window":      {`{"context_window":null}`, state.Context{}, false},
		"zero size":                {`{"context_window":{"context_window_size":0,"used_percentage":5}}`, state.Context{}, false},
		"fractional size":          {`{"context_window":{"context_window_size":2.5}}`, state.Context{}, false},
		"huge size":                {`{"context_window":{"context_window_size":1e12}}`, state.Context{}, false},
		"percent over 100":         {`{"context_window":{"used_percentage":150,"context_window_size":1000}}`, state.Context{}, false},
		"negative percent":         {`{"context_window":{"used_percentage":-5,"context_window_size":1000}}`, state.Context{UsedPercent: 0, Window: 1000, At: 1_800_000_000}, true},
		"negative tokens":          {`{"context_window":{"context_window_size":1000,"current_usage":{"input_tokens":-5}}}`, state.Context{}, false},
		"text instead of numbers":  {`{"context_window":{"used_percentage":"high","context_window_size":1000}}`, state.Context{}, false},
		"not json":                 {`{not json`, state.Context{}, false},
		"empty":                    {``, state.Context{}, false},
	} {
		got, ok := ContextOf([]byte(c.in), ctxNow)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: %+v %v, want %+v %v", name, got, ok, c.want, c.ok)
		}
	}
}

func TestTheStatusLineRecordsTheContextWindow(t *testing.T) {
	s := state.Open(t.TempDir())
	s.Ensure()
	s.WriteFile(s.Path("abcdef", "notes"), []byte("Objective: x\n"))
	l := Line{Store: s, Conf: conf.Conf{File: filepath.Join(s.Dir, "sitrep.conf"), Getenv: func(string) string { return "" }}, Run: &runner.Fake{}, Now: func() time.Time { return ctxNow }}
	draw := func(in string) string {
		var out bytes.Buffer
		l.Render(context.Background(), strings.NewReader(in), &out)
		return out.String()
	}
	plain := draw(`{"session_id":"abcdef"}`)
	with := draw(`{"session_id":"abcdef","context_window":{"used_percentage":12.5,"context_window_size":200000,"current_usage":{"input_tokens":25000}}}`)
	if plain != with {
		t.Errorf("the line is the same with or without the window:\n%q\n%q", plain, with)
	}
	if c, ok := s.Context("abcdef"); !ok || c.UsedPercent != 12.5 || c.Window != 200000 || c.Tokens != 25000 || c.At != ctxNow.Unix() {
		t.Errorf("record %+v %v", c, ok)
	}
	// an unknown session, the inbox and a bad id make no folder
	for _, id := range []string{"ghost001", "inbox", "../x"} {
		draw(`{"session_id":"` + id + `","context_window":{"used_percentage":5,"context_window_size":1000}}`)
	}
	if _, err := os.Stat(s.SessionDir("ghost001")); !os.IsNotExist(err) {
		t.Error("a folder was made for an unknown session")
	}
	// no window, no record change
	draw(`{"session_id":"abcdef","context_window":{"used_percentage":99}}`)
	if c, _ := s.Context("abcdef"); c.UsedPercent != 12.5 {
		t.Errorf("a window-less input changed the record: %+v", c)
	}
	// a folder that cannot be written does not stop the line
	os.Chmod(s.SessionDir("abcdef"), 0o500)
	defer os.Chmod(s.SessionDir("abcdef"), 0o700)
	if got := draw(`{"session_id":"abcdef","context_window":{"used_percentage":50,"context_window_size":1000}}`); got != plain {
		t.Errorf("a read-only folder changes the line: %q", got)
	}
}

func renderWith(t *testing.T, setting string) string {
	t.Helper()
	s := state.Open(t.TempDir())
	s.Ensure()
	statetest.Put(t, s, "s1.txt", "Objective: build\nProgress:  half\nNow:       compiling\n", 0)
	statetest.Put(t, s, "s1.queue", "first item\n", 0)
	get := func(n string) string {
		if n == "SITREP_STATUS_LINES" {
			return setting
		}
		return ""
	}
	l := Line{Store: s, Conf: conf.Conf{File: filepath.Join(s.Dir, "sitrep.conf"), Getenv: get}, Run: &runner.Fake{}}
	var out bytes.Buffer
	l.Render(context.Background(), strings.NewReader(`{"session_id":"s1"}`), &out)
	return out.String()
}

func TestStatusLinesChoosesWhichLinesShow(t *testing.T) {
	const (
		obj  = "Objective: build\n"
		prog = "Progress:  half\n"
		now  = "Now:       compiling\n"
		q    = "Queue [id s1]: 1 item, next: first item\n"
	)
	for setting, want := range map[string]string{
		"":                             obj + prog + now + q,
		"objective,progress,now,queue": obj + prog + now + q,
		"now,queue":                    now + q,
		"NOW, Queue":                   now + q,
		"objective now":                obj + now,
		"queue":                        q,
		"objective":                    obj,
		"now,now,queue":                now + q,
		"nwo":                          obj + prog + now + q, // no known name: everything shows
		",":                            obj + prog + now + q,
		"queue,bogus":                  q,
	} {
		if got := renderWith(t, setting); got != want {
			t.Errorf("%q: got %q, want %q", setting, got, want)
		}
	}
}

func TestStatusLinesWithNoNotesYet(t *testing.T) {
	s := state.Open(t.TempDir())
	s.Ensure()
	l := Line{Store: s, Conf: conf.Conf{File: filepath.Join(s.Dir, "sitrep.conf"), Getenv: func(n string) string {
		if n == "SITREP_STATUS_LINES" {
			return "queue"
		}
		return ""
	}}, Run: &runner.Fake{}}
	var out bytes.Buffer
	l.Render(context.Background(), strings.NewReader(`{"session_id":"abcdef"}`), &out)
	if out.String() != "Queue [id abcd]: empty\n" {
		t.Errorf("%q", out.String())
	}
}
