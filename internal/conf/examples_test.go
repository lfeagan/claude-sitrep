package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func exConf(t *testing.T, body string) Conf {
	t.Helper()
	dir := t.TempDir()
	c := Conf{File: filepath.Join(dir, "sitrep.conf"), Getenv: func(string) string { return "" }}
	if body != "" {
		if err := os.WriteFile(c.File, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func read(t *testing.T, c Conf) string {
	t.Helper()
	b, _ := os.ReadFile(c.File)
	return string(b)
}

func TestPutExamplesMakesTheFile(t *testing.T) {
	c := exConf(t, "")
	if err := c.PutExamples(); err != nil {
		t.Fatal(err)
	}
	got := read(t, c)
	if got != block() || !strings.HasPrefix(got, examplesBegin) || !strings.HasSuffix(got, examplesEnd+"\n") {
		t.Errorf("file:\n%s", got)
	}
	if fi, _ := os.Stat(c.File); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	before, _ := os.Stat(c.File)
	if err := c.PutExamples(); err != nil || read(t, c) != got {
		t.Errorf("a second call changes nothing: %v", err)
	}
	after, _ := os.Stat(c.File)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("a second call rewrote the file")
	}
}

func TestPutExamplesKeepsTheUsersLines(t *testing.T) {
	user := "SITREP_HUD_CMD='node hud.mjs'\nSITREP_COLORS='attn=#52b35d'\n# my own note\n"
	c := exConf(t, user)
	if err := c.PutExamples(); err != nil {
		t.Fatal(err)
	}
	got := read(t, c)
	if !strings.HasPrefix(got, user+"\n"+examplesBegin) || !strings.HasSuffix(got, block()) {
		t.Errorf("file:\n%s", got)
	}
	if c.Get("SITREP_COLORS") != "attn=#52b35d" || c.FromFile("SITREP_HUD_CMD") != "node hud.mjs" {
		t.Error("the user's settings still read")
	}
	// a file with no final newline
	c = exConf(t, "SITREP_QUEUE_ITEMS=9")
	if err := c.PutExamples(); err != nil || !strings.HasPrefix(read(t, c), "SITREP_QUEUE_ITEMS=9\n\n"+examplesBegin) || c.QueueItems() != 9 {
		t.Errorf("no newline: %v\n%s", err, read(t, c))
	}
}

func TestPutExamplesReplacesAnOldBlockAndMovesItLast(t *testing.T) {
	old := examplesBegin + "\n# SITREP_OLD=1\n" + examplesEnd + "\n"
	c := exConf(t, "A=1\n\n"+old+"\nSITREP_HUD_CMD='x'\n")
	if err := c.PutExamples(); err != nil {
		t.Fatal(err)
	}
	got := read(t, c)
	if strings.Contains(got, "SITREP_OLD") || strings.Count(got, examplesBegin) != 1 || !strings.HasSuffix(got, block()) || !strings.HasPrefix(got, "A=1\n\nSITREP_HUD_CMD='x'\n\n") {
		t.Errorf("file:\n%s", got)
	}
	// a block with no end line was cut by hand: it is the user's, so it is kept and a new block is added
	c = exConf(t, "A=1\n"+examplesBegin+"\n# half\n")
	if err := c.PutExamples(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c); !strings.Contains(got, "# half") || !strings.HasSuffix(got, block()) {
		t.Errorf("cut block:\n%s", got)
	}
}

func TestDropExamples(t *testing.T) {
	c := exConf(t, "")
	c.PutExamples()
	if err := c.DropExamples(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.File); !os.IsNotExist(err) {
		t.Error("the file goes when only the block was in it")
	}
	c = exConf(t, "A=1\nSITREP_HUD_CMD='x'\n")
	c.PutExamples()
	if err := c.DropExamples(); err != nil || read(t, c) != "A=1\nSITREP_HUD_CMD='x'\n" {
		t.Errorf("the user's lines stay exactly: %v %q", err, read(t, c))
	}
	if err := c.DropExamples(); err != nil || read(t, c) != "A=1\nSITREP_HUD_CMD='x'\n" {
		t.Errorf("without a block nothing changes: %v", err)
	}
	// a missing file is fine
	if err := exConf(t, "").DropExamples(); err != nil {
		t.Error(err)
	}
}

func TestExamplesWorkWithTheHudLine(t *testing.T) {
	c := exConf(t, "")
	c.PutExamples()
	if err := c.PutHUD("node 'x y'"); err != nil {
		t.Fatal(err)
	}
	if c.FromFile("SITREP_HUD_CMD") != "node 'x y'" {
		t.Errorf("the saved status line reads back: %q", c.FromFile("SITREP_HUD_CMD"))
	}
	c.PutExamples()
	got := read(t, c)
	if strings.Count(got, "SITREP_HUD_CMD='") != 1 || !strings.HasSuffix(got, block()) {
		t.Errorf("file:\n%s", got)
	}
	if err := c.DropHUD(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, c); !strings.HasSuffix(got, block()) || strings.Contains(got, "SITREP_HUD_CMD='") {
		t.Errorf("dropping the status line leaves the block:\n%s", got)
	}
	c.PutExamples() // setup does this after it drops the line, which also tidies the blank line
	if got := read(t, c); got != block() {
		t.Errorf("tidied:\n%s", got)
	}
}

func TestExamplesRefuseAHugeFile(t *testing.T) {
	big := strings.Repeat("# filler line\n", 6000)
	c := exConf(t, big)
	if err := c.PutExamples(); err == nil || read(t, c) != big {
		t.Errorf("a file over 64 KB is not rewritten: %v", err)
	}
	if err := c.DropExamples(); err == nil || read(t, c) != big {
		t.Errorf("nor edited: %v", err)
	}
}

func TestExamplesAreComments(t *testing.T) {
	c := exConf(t, "")
	c.PutExamples()
	for _, name := range []string{"SITREP_QUEUE_ITEMS", "SITREP_HIGHLIGHT_SECONDS", "SITREP_HIGHLIGHT_SGR", "SITREP_DEBOUNCE", "SITREP_CHECK_UPDATES", "SITREP_THEME",
		"SITREP_COLORS", "SITREP_TUI_INTERVAL", "SITREP_TUI_READONLY", "SITREP_TUI_BELL", "SITREP_HUD_CMD"} {
		if got := c.Get(name); got != "" {
			t.Errorf("%s reads %q from a comment", name, got)
		}
	}
	for i, l := range strings.Split(strings.TrimSuffix(Examples, "\n"), "\n") {
		if !strings.HasPrefix(l, "#") {
			t.Errorf("line %d is not a comment: %q", i+1, l)
		}
		if len(l) > 130 {
			t.Errorf("line %d is long: %d", i+1, len(l))
		}
	}
}

// Every example is a setting the program reads, with a value it accepts: uncommenting them all must give a working configuration.
func TestEveryExampleIsASetting(t *testing.T) {
	var lines []string
	names := map[string]bool{}
	for _, l := range strings.Split(Examples, "\n") {
		if rest, ok := strings.CutPrefix(l, "# SITREP_"); ok && strings.Contains(rest, "=") && !strings.HasPrefix(rest, "HUD_CMD") {
			lines = append(lines, "SITREP_"+rest)
			names["SITREP_"+strings.SplitN(rest, "=", 2)[0]] = true
		}
	}
	c := exConf(t, strings.Join(lines, "\n")+"\n")
	if c.QueueItems() != 5 || c.HighlightSeconds() != 60 || c.HighlightSGR() != "48;5;238" || !c.CheckUpdates() || c.Theme() != "default" ||
		c.Colors() == "" || c.TUIInterval() != 3*time.Second {
		t.Errorf("an example is not accepted: %d %d %q %v %q %q %v", c.QueueItems(), c.HighlightSeconds(), c.HighlightSGR(), c.CheckUpdates(), c.Theme(), c.Colors(), c.TUIInterval())
	}
	for _, n := range []string{"SITREP_QUEUE_ITEMS", "SITREP_HIGHLIGHT_SECONDS", "SITREP_HIGHLIGHT_SGR", "SITREP_DEBOUNCE", "SITREP_CHECK_UPDATES", "SITREP_THEME", "SITREP_COLORS", "SITREP_TUI_INTERVAL", "SITREP_TUI_READONLY", "SITREP_TUI_BELL"} {
		if !names[n] || c.Get(n) == "" {
			t.Errorf("%s has no example, or its example is not read", n)
		}
	}
}
