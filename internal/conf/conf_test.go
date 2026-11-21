package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnquote(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:                   "plain",
		`'single quoted'`:         "single quoted",
		`"double \"quoted\" \$x"`: `double "quoted" $x`,
		`a\ b`:                    "a b",
		`'it'\''s'`:               "it's",
		`'a'"b"c`:                 "abc",
		`'open`:                   "open",
		`$HOME`:                   "$HOME",
		``:                        "",
	} {
		if got := Unquote(in); got != want {
			t.Errorf("Unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

func conf(t *testing.T, body string, env map[string]string) Conf {
	t.Helper()
	f := filepath.Join(t.TempDir(), "sitrep.conf")
	if body != "\x00" {
		if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Conf{File: f, Getenv: func(k string) string { return env[k] }}
}

func TestGet(t *testing.T) {
	c := conf(t, "OTHER=1\nSITREP_QUEUE_ITEMS=7\nSITREP_QUEUE_ITEMS=9\nSITREP_HUD_CMD='bash \"$HOME/x\"'\n", nil)
	if got := c.Get("SITREP_QUEUE_ITEMS"); got != "7" {
		t.Errorf("first line wins: got %q", got)
	}
	if got := c.Get("SITREP_HUD_CMD"); got != `bash "$HOME/x"` {
		t.Errorf("quoted value: got %q", got)
	}
	if got := c.Get("MISSING"); got != "" {
		t.Errorf("missing: got %q", got)
	}
	if got := c.Get("ITEMS"); got != "" {
		t.Errorf("a name is matched whole: got %q", got)
	}
	// the environment wins over the file
	c = conf(t, "SITREP_QUEUE_ITEMS=7\n", map[string]string{"SITREP_QUEUE_ITEMS": "3"})
	if got := c.QueueItems(); got != 3 {
		t.Errorf("env: got %d", got)
	}
	if got := conf(t, "\x00", nil).QueueItems(); got != DefaultQueueItems {
		t.Errorf("no file: got %d", got)
	}
}

func TestNumbers(t *testing.T) {
	for body, want := range map[string]int{
		"SITREP_QUEUE_ITEMS=0\n": 0, "SITREP_QUEUE_ITEMS=12\n": 12, "SITREP_QUEUE_ITEMS=x\n": 5, "SITREP_QUEUE_ITEMS=-1\n": 5,
		"SITREP_QUEUE_ITEMS=\n": 5, "SITREP_QUEUE_ITEMS='4'\n": 4, "SITREP_QUEUE_ITEMS=99999999999999999999\n": 5,
	} {
		if got := conf(t, body, nil).QueueItems(); got != want {
			t.Errorf("%q gives %d, want %d", body, got, want)
		}
	}
	if got := conf(t, "SITREP_HIGHLIGHT_SECONDS=0\n", nil).HighlightSeconds(); got != 0 {
		t.Errorf("highlight seconds 0 gives %d", got)
	}
	if got := conf(t, "\x00", nil).HighlightSeconds(); got != DefaultHighlightSeconds {
		t.Errorf("default highlight seconds %d", got)
	}
}

func TestHighlightSGR(t *testing.T) {
	for body, want := range map[string]string{
		"SITREP_HIGHLIGHT_SGR=1;32\n":         "1;32",
		"SITREP_HIGHLIGHT_SGR=48;5;22\n":      "48;5;22",
		"SITREP_HIGHLIGHT_SGR=1m\\033]0;x\n":  DefaultHighlightSGR,
		"SITREP_HIGHLIGHT_SGR=\n":             DefaultHighlightSGR,
		"SITREP_HIGHLIGHT_SGR='31;1'\n":       "31;1",
		"SITREP_HIGHLIGHT_SGR=$'\\x1b[31m'\n": DefaultHighlightSGR,
	} {
		if got := conf(t, body, nil).HighlightSGR(); got != want {
			t.Errorf("%q gives %q, want %q", body, got, want)
		}
	}
}

func TestOpen(t *testing.T) {
	if got := Open("/s").File; got != "/s/sitrep.conf" {
		t.Errorf("File = %q", got)
	}
}

func TestFileThatIsNotSmallAndRegularReadsAsEmpty(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.conf")
	if err := os.WriteFile(big, []byte("X=1\n"+strings.Repeat("#", 70<<10)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (Conf{File: big, Getenv: func(string) string { return "" }}).Get("X"); got != "" {
		t.Errorf("a huge file was read: %q", got)
	}
	if got := (Conf{File: dir, Getenv: func(string) string { return "" }}).Get("X"); got != "" {
		t.Errorf("a directory was read: %q", got)
	}
}
