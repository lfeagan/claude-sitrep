package state

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"abc123": true, "A_b-9": true, "": false, "../evil": false, "a b": false, "a.b": false, "a\nb": false, "é": false, "inbox": true,
	} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
	if SessionID("inbox") || !SessionID("inbox2") || SessionID("") {
		t.Error("SessionID must refuse exactly the inbox and the empty id")
	}
}

func TestOpenIgnoresConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/elsewhere")
	if got := Open("/home/u").Dir; got != "/home/u/.claude/sitrep" {
		t.Errorf("Open().Dir = %q", got)
	}
	env := func(k string) string { return map[string]string{"CLAUDE_CONFIG_DIR": "/elsewhere"}[k] }
	if got := ConfigDir(env, "/home/u"); got != "/elsewhere" {
		t.Errorf("ConfigDir = %q", got)
	}
	if got := ConfigDir(func(string) string { return "" }, "/home/u"); got != "/home/u/.claude" {
		t.Errorf("ConfigDir default = %q", got)
	}
}

func newStore(t *testing.T) Store {
	t.Helper()
	s := Open(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEnsureTightensTheFolder(t *testing.T) {
	s := Open(t.TempDir())
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(s.Dir); fi.Mode().Perm() != DirMode {
		t.Errorf("mode %v, want %v", fi.Mode().Perm(), DirMode)
	}
}

func TestPaths(t *testing.T) {
	s := Store{Dir: "/d"}
	if s.Path("abc", "txt") != "/d/abc.txt" || s.Hidden("abc", "state") != "/d/.abc.state" {
		t.Errorf("Path %q, Hidden %q", s.Path("abc", "txt"), s.Hidden("abc", "state"))
	}
}

func TestWriteFile(t *testing.T) {
	s := newStore(t)
	p := s.Path("abc", "txt")
	for _, body := range []string{"first\n", "second\n"} {
		if err := s.WriteFile(p, []byte(body)); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(p); string(got) != body {
			t.Errorf("read %q, want %q", got, body)
		}
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != FileMode {
		t.Errorf("mode %v, want %v", fi.Mode().Perm(), FileMode)
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Errorf("a temporary file was left behind: %v", entries)
	}
}

func TestWriteFileFailureLeavesNothing(t *testing.T) {
	s := newStore(t)
	dir := s.Path("adir", "txt")
	if err := os.Mkdir(dir, 0o700); err != nil { // renaming a file over a directory fails
		t.Fatal(err)
	}
	if err := s.WriteFile(dir, []byte("x")); err == nil {
		t.Fatal("want an error")
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Errorf("a temporary file was left behind: %v", entries)
	}
	if err := (Store{Dir: filepath.Join(s.Dir, "missing")}).WriteFile("x", nil); err == nil {
		t.Error("want an error for a missing folder")
	}
}

func TestResolve(t *testing.T) {
	s := newStore(t)
	for _, f := range []string{"15f1aaaa.txt", "15f2bbbb.txt", "9zzz0000.txt", "a.b.txt", "x.queue"} {
		if err := os.WriteFile(filepath.Join(s.Dir, f), nil, FileMode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(s.Dir, "dirdirdir.txt"), DirMode); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{"9z": "9zzz0000", "15f1": "15f1aaaa", "inbox": "inbox", "9zzz0000": "9zzz0000"} {
		if got, err := s.Resolve(in); err != nil || string(got) != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, msg := range map[string]string{
		"15f":      "id '15f' matches 2; run 'sitrep ls'",
		"nope":     "id 'nope' matches 0; run 'sitrep ls'",
		"a":        "id 'a' matches 0; run 'sitrep ls'", // a.b.txt is not a session file
		"dirdir":   "id 'dirdir' matches 0; run 'sitrep ls'",
		"../evil":  "bad id '../evil'",
		"x\033[31": "bad id 'x[31'",
		"":         "bad id ''",
	} {
		if _, err := s.Resolve(in); err == nil || err.Error() != msg {
			t.Errorf("Resolve(%q) error %v, want %q", in, err, msg)
		}
	}
	if strings.Contains(func() string { _, e := s.Resolve("\u009bx"); return e.Error() }(), "\u009b") {
		t.Error("an error message holds a control character")
	}
}

func TestResolveWithoutFolder(t *testing.T) {
	if _, err := Open(t.TempDir()).Resolve("abc"); err == nil || !strings.Contains(err.Error(), "matches 0") {
		t.Errorf("error %v", err)
	}
}

func TestEnsureDoesNotChmodThroughALink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "real")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	s := Open(filepath.Join(base, "home"))
	if err := os.MkdirAll(filepath.Dir(s.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, s.Dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o755 {
		t.Errorf("the target of the link was changed to %v", fi.Mode().Perm())
	}
}

func TestWriteFileStaysInTheFolder(t *testing.T) {
	s := newStore(t)
	if err := s.WriteFile(filepath.Join(s.Dir, "..", "escape"), []byte("x")); err == nil {
		t.Error("a file outside the folder was written")
	}
	if err := s.WriteFile(filepath.Join(s.Dir, "sub", "x"), []byte("x")); err == nil {
		t.Error("a file in a subfolder was written")
	}
}

func TestParseID(t *testing.T) {
	if id, err := ParseID("abc-1"); err != nil || id != "abc-1" {
		t.Errorf("%q, %v", id, err)
	}
	if id, err := ParseID("inbox"); err != nil || id != Inbox {
		t.Errorf("%q, %v", id, err)
	}
	if _, err := ParseID("../x"); err == nil || err.Error() != "bad id '../x'" {
		t.Errorf("%v", err)
	}
}

// promptly runs f and fails the test if it does not return in time: a hostile pipe must not hang a reader.
func promptly(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s hung", what)
	}
}

func TestReadFileRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no mkfifo")
	}
	promptly(t, "ReadFile on a pipe", func() {
		if _, err := ReadFile(fifo, 100); err == nil {
			t.Error("a pipe was read")
		}
	})
	promptly(t, "FirstLine on a pipe", func() {
		if got := FirstLine(fifo); got != "" {
			t.Errorf("FirstLine = %q", got)
		}
	})
	if _, err := ReadFile(dir, 100); err == nil {
		t.Error("a directory was read")
	}
	real := filepath.Join(dir, "real")
	os.WriteFile(real, []byte("0123456789"), 0o600)
	link := filepath.Join(dir, "link")
	os.Symlink(real, link)
	if b, err := ReadFile(link, 4); err != nil || string(b) != "0123" {
		t.Errorf("a link to a file, cut to 4 bytes: %q, %v", b, err)
	}
	if IsRegular(fifo) || IsRegular(dir) || !IsRegular(link) {
		t.Error("IsRegular is wrong")
	}
}

func TestFirstLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	for body, want := range map[string]string{"working\n": "working", " \tworking \t\nrest": "working", "": "", "\nx": "", "a b\n": "a b"} {
		os.WriteFile(p, []byte(body), 0o600)
		if got := FirstLine(p); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", body, got, want)
		}
	}
	if FirstLine(filepath.Join(t.TempDir(), "missing")) != "" {
		t.Error("a missing file has a first line")
	}
}
