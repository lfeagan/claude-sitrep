package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func msg(typ string, content any) string {
	b, _ := json.Marshal(map[string]any{"type": typ, "message": map[string]any{"content": content}})
	return string(b)
}

func write(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestValid(t *testing.T) {
	cfg := t.TempDir()
	proj := filepath.Join(cfg, "projects", "p")
	os.MkdirAll(proj, 0o755)
	ok := filepath.Join(proj, "a.jsonl")
	os.WriteFile(ok, []byte("{}"), 0o600)
	outside := filepath.Join(t.TempDir(), "b.jsonl")
	os.WriteFile(outside, []byte("{}"), 0o600)
	link := filepath.Join(proj, "link.jsonl")
	os.Symlink(outside, link)
	fifo := filepath.Join(proj, "fifo.jsonl")
	syscall.Mkfifo(fifo, 0o600)
	for path, want := range map[string]bool{
		ok:                                      true,
		outside:                                 false,
		link:                                    false,
		fifo:                                    false,
		filepath.Join(proj, "missing.jsonl"):    false,
		proj + "/../p/a.jsonl":                  false, // a /../ in the raw path
		cfg + "/projects/../projects/p/a.jsonl": false,
		"":                                      false,
		proj:                                    false, // a directory
		strings.Replace(ok, "/projects/", "/projectsX/", 1): false,
	} {
		if got := Valid(path, cfg); got != want {
			t.Errorf("Valid(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestOpenRefusesALinkAndAPipe(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "r")
	os.WriteFile(real, []byte("x"), 0o600)
	link := filepath.Join(dir, "l")
	os.Symlink(real, link)
	if _, err := open(link); err == nil {
		t.Error("a link was opened")
	}
	fifo := filepath.Join(dir, "f")
	syscall.Mkfifo(fifo, 0o600)
	done := make(chan error)
	go func() { _, err := open(fifo); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a pipe was opened")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("opening a pipe hung")
	}
}

func TestFirstPrompt(t *testing.T) {
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"plain":            {[]string{msg("user", "build the thing")}, "build the thing"},
		"skips wrappers":   {[]string{msg("user", "<command-name>/clear</command-name>"), msg("user", "  &lt;local-command-stdout&gt;"), msg("user", "real one")}, "real one"},
		"skips assistant":  {[]string{msg("assistant", "hello"), msg("user", "mine")}, "mine"},
		"list of parts":    {[]string{msg("user", []map[string]string{{"type": "image"}, {"type": "text", "text": "from parts"}, {"type": "text", "text": "two"}})}, "from parts two"},
		"first line":       {[]string{msg("user", "\n\n   first real line\nsecond")}, "first real line"},
		"blank skipped":    {[]string{msg("user", "  \n "), msg("user", "next")}, "next"},
		"bad line skipped": {[]string{"{not json", msg("user", "after")}, "after"},
		"none":             {[]string{msg("assistant", "x")}, ""},
		"wrapper first line only counts at the start": {[]string{msg("user", "text <b>bold</b>")}, "text <b>bold</b>"},
	} {
		if got := FirstPrompt(write(t, c.lines...)); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
	if FirstPrompt(filepath.Join(t.TempDir(), "missing")) != "" {
		t.Error("a missing transcript has a prompt")
	}
}

func TestFirstPromptWithAHugeLine(t *testing.T) {
	huge := msg("user", strings.Repeat("x", 400<<10))
	if got := FirstPrompt(write(t, huge)); len(got) != 400<<10 {
		t.Errorf("got %d bytes", len(got))
	}
}

func TestRecent(t *testing.T) {
	var lines []string
	for i := 0; i < 60; i++ {
		lines = append(lines, msg("user", "q"+strings.Repeat("a", i)))
	}
	lines = append(lines, msg("assistant", "line one\nline two "+strings.Repeat("z", 700)), `{"type":"system","message":{"content":"skip"}}`, "{bad", msg("user", []map[string]string{{"type": "text", "text": "tail"}}))
	got, _ := Recent(write(t, lines...))
	if strings.Contains(got, "user: qaaaaaaaaaaaaaaaaaaaaaaaa\n") && strings.Contains(got, "user: q\n") {
		t.Error("lines before the last 40 were included")
	}
	for _, want := range []string{"assistant: line one\n", "line two zzz", "user: tail"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, l := range strings.Split(got, "\n") {
		if n := len([]rune(l)); n > 600 {
			t.Errorf("a line has %d characters", n)
		}
	}
	if strings.Contains(got, "skip") {
		t.Error("a system line was included")
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("trailing newline")
	}
}

func TestRecentTailsAt6000BytesOnACharacterBoundary(t *testing.T) {
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, msg("user", strings.Repeat("\u4e16", 300)))
	}
	got, _ := Recent(write(t, lines...))
	if len(got) > 6000 || len(got) < 5990 {
		t.Errorf("%d bytes", len(got))
	}
	if !strings.HasPrefix(got, "\u4e16") && !strings.HasPrefix(got, "user: ") && !strings.HasPrefix(got, "\n") {
		t.Errorf("cut inside a character: %q", got[:8])
	}
}
