package state

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func ctxStore(t *testing.T) Store {
	t.Helper()
	s := Store{Dir: t.TempDir()}
	if err := s.WriteFile(s.Path("c5d05701", "notes"), []byte("Objective: x\n")); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRecordAndReadContext(t *testing.T) {
	s := ctxStore(t)
	c := Context{UsedPercent: 37.5, Window: 1_000_000, Tokens: 375_000, At: 1_800_000_000}
	if err := s.RecordContext("c5d05701", c); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Context("c5d05701")
	if !ok || got != c {
		t.Errorf("read back %+v %v", got, ok)
	}
	b, _ := os.ReadFile(s.Path("c5d05701", "context"))
	if string(b) != "37.5 1000000 375000 1800000000\n" {
		t.Errorf("file %q", b)
	}
	if fi, _ := os.Stat(s.Path("c5d05701", "context")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
	if m, _ := filepath.Glob(filepath.Join(s.SessionDir("c5d05701"), ".sitrep.*")); len(m) != 0 {
		t.Errorf("temporary files left: %v", m)
	}
}

func TestContextIsRewrittenOnlyWhenNeeded(t *testing.T) {
	s := ctxStore(t)
	c := Context{UsedPercent: 10, Window: 200_000, Tokens: 20_000, At: 1000}
	s.RecordContext("c5d05701", c)
	path := s.Path("c5d05701", "context")
	mark := func() string { b, _ := os.ReadFile(path); return string(b) }
	// the same numbers a minute later: not rewritten
	c.At = 1060
	s.RecordContext("c5d05701", c)
	if got := mark(); !strings.HasSuffix(got, " 1000\n") {
		t.Errorf("rewritten for nothing: %q", got)
	}
	// the same numbers after the age limit: the time moves, so it means last seen
	c.At = 1000 + ContextMaxAge
	s.RecordContext("c5d05701", c)
	if got := mark(); !strings.HasSuffix(got, " 1300\n") {
		t.Errorf("not refreshed: %q", got)
	}
	// new numbers at once
	c2 := Context{UsedPercent: 11.2, Window: 200_000, Tokens: 22_400, At: 1301}
	s.RecordContext("c5d05701", c2)
	if got, _ := s.Context("c5d05701"); got != c2 {
		t.Errorf("new numbers: %+v", got)
	}
	// a clock that moved back is as good as an old record
	c3 := c2
	c3.At = 5
	s.RecordContext("c5d05701", c3)
	if got, _ := s.Context("c5d05701"); got.At != 5 {
		t.Errorf("a record from the future: %+v", got)
	}
}

func TestContextOfAnUnknownSessionMakesNothing(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, id := range []ID{"ghost001", Inbox} {
		if err := s.RecordContext(id, Context{UsedPercent: 1, Window: 1, At: 1}); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "sessions")); !os.IsNotExist(err) {
		t.Error("a folder was made")
	}
	// a folder with no notes is no session either
	os.MkdirAll(s.SessionDir("nonotes1"), 0o700)
	s.RecordContext("nonotes1", Context{UsedPercent: 1, Window: 1, At: 1})
	if _, err := os.Stat(s.Path("nonotes1", "context")); err == nil {
		t.Error("recorded for a folder with no notes")
	}
	// a missing or read-only folder is no error
	ro := ctxStore(t)
	os.Chmod(ro.SessionDir("c5d05701"), 0o500)
	defer os.Chmod(ro.SessionDir("c5d05701"), 0o700)
	ro.RecordContext("c5d05701", Context{UsedPercent: 1, Window: 100, At: 1}) // the error, if any, is dropped by the caller
}

func TestContextValidation(t *testing.T) {
	s := ctxStore(t)
	for _, bad := range []Context{
		{UsedPercent: -1, Window: 100}, {UsedPercent: 100.1, Window: 100}, {UsedPercent: math.NaN(), Window: 100}, {UsedPercent: math.Inf(1), Window: 100},
		{UsedPercent: 5, Window: 0}, {UsedPercent: 5, Window: MaxWindow + 1}, {UsedPercent: 5, Window: 100, Tokens: -1}, {UsedPercent: 5, Window: 100, Tokens: MaxTokens + 1},
		{UsedPercent: 5, Window: 100, At: -1},
	} {
		s.RecordContext("c5d05701", bad)
		if _, ok := s.Context("c5d05701"); ok {
			t.Errorf("%+v recorded", bad)
		}
	}
}

func TestHostileContextFiles(t *testing.T) {
	s := ctxStore(t)
	path := s.Path("c5d05701", "context")
	write := func(b string) { os.Remove(path); os.WriteFile(path, []byte(b), 0o600) }
	for name, body := range map[string]string{
		"empty": "", "three fields": "1 2 3\n", "five fields": "1 2 3 4 5\n", "text": "a b c d\n", "nan": "NaN 100 1 1\n", "inf": "Inf 100 1 1\n",
		"negative": "-1 100 1 1\n", "over 100": "101 100 1 1\n", "zero window": "5 0 1 1\n", "huge window": "5 999999999999 1 1\n", "escape": "5 100 1 1\x1b[31m\n",
		"megabyte": strings.Repeat("1", 1<<20), "float tokens": "5 100 1.5 1\n", "hex": "5 0x64 1 1\n",
	} {
		write(body)
		if c, ok := s.Context("c5d05701"); ok {
			t.Errorf("%s: read %+v", name, c)
		}
	}
	write("5 100 1 1")
	if c, ok := s.Context("c5d05701"); !ok || c.UsedPercent != 5 {
		t.Errorf("no newline: %+v %v", c, ok)
	}
	os.Remove(path)
	target := filepath.Join(t.TempDir(), "elsewhere")
	os.WriteFile(target, []byte("5 100 1 1\n"), 0o600)
	os.Symlink(target, path)
	if _, ok := s.Context("c5d05701"); ok {
		t.Error("a link")
	}
	os.Remove(path)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Context("c5d05701"); ok { // must not block
		t.Error("a pipe")
	}
	if _, ok := s.Context(Inbox); ok {
		t.Error("the inbox has no context")
	}
}
