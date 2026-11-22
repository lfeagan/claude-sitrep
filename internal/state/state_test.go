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

func TestEnsureNeverWidensAFolder(t *testing.T) {
	for _, c := range []struct{ before, after os.FileMode }{{0o755, 0o700}, {0o775, 0o700}, {0o750, 0o700}, {0o700, 0o700}, {0o500, 0o500}, {0o555, 0o500}} {
		dir := filepath.Join(t.TempDir(), "state")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, c.before); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o700) })
		if err := (Store{Dir: dir}).Ensure(); err != nil {
			t.Fatalf("%v: %v", c.before, err)
		}
		if fi, _ := os.Stat(dir); fi.Mode().Perm() != c.after {
			t.Errorf("a folder of %v became %v, want %v", c.before, fi.Mode().Perm(), c.after)
		}
	}
}

func TestPaths(t *testing.T) {
	s := Store{Dir: "/d"}
	for got, want := range map[string]string{
		s.Path("abc", "notes"): "/d/sessions/abc/notes", s.Path("abc", "state"): "/d/sessions/abc/state", s.Path("inbox", "queue"): "/d/inbox.queue",
		s.SessionDir("abc"): "/d/sessions/abc", s.SessionDir("inbox"): "/d", s.SessionsDir(): "/d/sessions",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestWriteFile(t *testing.T) {
	s := newStore(t)
	p := s.Path("abc", "notes")
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
	entries, _ := os.ReadDir(s.SessionDir("abc"))
	if len(entries) != 1 {
		t.Errorf("a temporary file was left behind: %v", entries)
	}
	for _, d := range []string{s.SessionsDir(), s.SessionDir("abc")} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != DirMode {
			t.Errorf("%s: %v", d, err)
		}
	}
	// the root takes a file too, and nowhere else does
	if err := s.WriteFile(filepath.Join(s.Dir, "inbox.queue"), []byte("x")); err != nil {
		t.Errorf("a file in the root: %v", err)
	}
	for _, bad := range []string{filepath.Join(s.Dir, "sessions", "x y", "notes"), filepath.Join(s.Dir, "sessions", "inbox", "notes"), filepath.Join(s.Dir, "sessions", "a", "b", "c"),
		filepath.Join(s.Dir, "other", "notes"), filepath.Join(s.Dir, "sessions", "..", "..", "evil")} {
		if err := s.WriteFile(bad, []byte("x")); err == nil {
			t.Errorf("wrote %s", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "sessions", "x y")); err == nil {
		t.Error("a folder was made for an invalid id")
	}
}

func TestWriteFileFailureLeavesNothing(t *testing.T) {
	s := newStore(t)
	dir := s.Path("adir", "notes")
	if err := os.MkdirAll(dir, 0o700); err != nil { // renaming a file over a directory fails
		t.Fatal(err)
	}
	if err := s.WriteFile(dir, []byte("x")); err == nil {
		t.Fatal("want an error")
	}
	entries, _ := os.ReadDir(s.SessionDir("adir"))
	if len(entries) != 1 {
		t.Errorf("a temporary file was left behind: %v", entries)
	}
	if err := (Store{Dir: filepath.Join(s.Dir, "missing")}).WriteFile("x", nil); err == nil {
		t.Error("want an error for a missing folder")
	}
}

func TestResolve(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"15f1aaaa", "15f2bbbb", "9zzz0000"} {
		os.MkdirAll(s.SessionDir(ID(id)), DirMode)
		os.WriteFile(s.Path(ID(id), "notes"), nil, FileMode)
	}
	os.MkdirAll(filepath.Join(s.SessionsDir(), "a.b"), DirMode) // not a valid id
	os.WriteFile(filepath.Join(s.SessionsDir(), "a.b", "notes"), nil, FileMode)
	os.MkdirAll(filepath.Join(s.SessionsDir(), "nonotes1"), DirMode) // a folder without notes is no session
	os.MkdirAll(filepath.Join(s.SessionsDir(), "dirnotes1", "notes"), DirMode)
	os.WriteFile(filepath.Join(s.SessionsDir(), "filefile"), nil, FileMode) // a file is no session
	real := filepath.Join(t.TempDir(), "elsewhere")
	os.MkdirAll(real, DirMode)
	os.WriteFile(filepath.Join(real, "notes"), nil, FileMode)
	os.Symlink(real, filepath.Join(s.SessionsDir(), "linked01")) // a link is never followed
	for in, want := range map[string]string{"9z": "9zzz0000", "15f1": "15f1aaaa", "inbox": "inbox", "9zzz0000": "9zzz0000"} {
		if got, err := s.Resolve(in); err != nil || string(got) != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, msg := range map[string]string{
		"15f":      "id '15f' matches 2; run 'sitrep ls'",
		"nope":     "id 'nope' matches 0; run 'sitrep ls'",
		"a":        "id 'a' matches 0; run 'sitrep ls'", // a.b is not a valid id
		"nonotes":  "id 'nonotes' matches 0; run 'sitrep ls'",
		"dirnotes": "id 'dirnotes' matches 0; run 'sitrep ls'",
		"filefile": "id 'filefile' matches 0; run 'sitrep ls'",
		"linked":   "id 'linked' matches 0; run 'sitrep ls'",
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
	if b, err := ReadFile(real, 4); err != nil || string(b) != "0123" {
		t.Errorf("a file, cut to 4 bytes: %q, %v", b, err)
	}
	if b, err := ReadFile(link, 100); err == nil {
		t.Errorf("a link to a file was read: %q", b)
	}
	if got := FirstLine(link); got != "" {
		t.Errorf("FirstLine follows a link: %q", got)
	}
	if b, err := ReadFileFollow(link, 4); err != nil || string(b) != "0123" {
		t.Errorf("a file that may be a link on purpose is followed: %q, %v", b, err)
	}
	if IsRegular(fifo) || IsRegular(dir) || IsRegular(link) || !IsRegular(real) {
		t.Error("IsRegular is wrong: a link to a file is not a regular file")
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

func TestSessions(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"bbbb", "aaaa"} {
		s.EnsureSession(ID(id))
		os.WriteFile(s.Path(ID(id), "notes"), nil, FileMode)
	}
	s.EnsureSession("cccc") // no notes yet
	if got := s.Sessions(); len(got) != 2 || got[0] != "aaaa" || got[1] != "bbbb" {
		t.Errorf("%v", got)
	}
	if got := (Store{Dir: filepath.Join(t.TempDir(), "none")}).Sessions(); len(got) != 0 {
		t.Errorf("%v", got)
	}
}

func TestEnsureSession(t *testing.T) {
	s := Open(t.TempDir())
	if err := s.EnsureSession("abc"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSession("abc"); err != nil {
		t.Errorf("twice: %v", err)
	}
	if fi, err := os.Stat(s.SessionDir("abc")); err != nil || fi.Mode().Perm() != DirMode {
		t.Errorf("%v", err)
	}
	if err := s.EnsureSession("inbox"); err != nil {
		t.Errorf("inbox: %v", err)
	}
}

func TestParseLegacy(t *testing.T) {
	for name, want := range map[string]Legacy{
		"abc.txt": {ID: "abc", File: "notes"}, ".abc.state": {ID: "abc", File: "state"}, "abc.queue": {ID: "abc", File: "queue"}, "abc.cwd": {ID: "abc", File: "cwd"},
		".abc.count": {ID: "abc", File: "count"}, ".abc.stamp": {ID: "abc", File: "stamp"}, ".abc.pending": {ID: "abc", File: "pending"}, ".abc.pinned": {ID: "abc", File: "pinned"},
		".abc.snooze": {ID: "abc", File: "snooze"}, ".abc.lock": {ID: "abc", File: "lock"}, ".abc.prompt": {ID: "abc", Obsolete: true},
		"13159d08-594f-49e6-bdb5-f37c13cadbec.txt": {ID: "13159d08-594f-49e6-bdb5-f37c13cadbec", File: "notes"},
	} {
		if got, ok := ParseLegacy(name); !ok || got != want {
			t.Errorf("%q: %+v, %v", name, got, ok)
		}
	}
	for _, name := range []string{"inbox.queue", "sitrep.conf", "bin", "a.b.txt", "x.y.z.txt", ".queue.lock", "abc.bogus", ".abc", "abc", ".abc.state.tmp", ".sitrep.XXXX", "bad name.txt", "abc.state", ".abc.txt", ".abc.queue", ".abc.cwd", "abc.count", "abc.prompt", "..txt", ".txt", "a b.txt", "\u00e9.txt", ""} {
		if got, ok := ParseLegacy(name); ok {
			t.Errorf("%q parsed as %+v", name, got)
		}
	}
}

func TestLocate(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == EnvDir {
				return v
			}
			return ""
		}
	}
	for name, c := range map[string]struct {
		getenv func(string) string
		want   string
		bad    bool
	}{
		"unset":            {env(""), "/home/u/.claude/sitrep", false},
		"no environment":   {nil, "/home/u/.claude/sitrep", false},
		"absolute":         {env("/mnt/nas/sitrep"), "/mnt/nas/sitrep", false},
		"cleaned":          {env("/mnt/nas//sitrep/./"), "/mnt/nas/sitrep", false},
		"relative":         {env("state"), "", true},
		"home shorthand":   {env("~/state"), "", true},
		"the root":         {env("/"), "", true},
		"the root, spelt":  {env("/a/.."), "", true},
		"the home folder":  {env("/home/u"), "", true},
		"above the home":   {env("/home"), "", true},
		"claude's folder":  {env("/home/u/.claude"), "", true},
		"its registry":     {env("/home/u/.claude/sessions/"), "", true},
		"its transcripts":  {env("/home/u/.claude/projects"), "", true},
		"inside claude's":  {env("/home/u/.claude/demo"), "/home/u/.claude/demo", false},
		"beside the home":  {env("/home/u-demo"), "/home/u-demo", false},
		"below the home":   {env("/home/u/demo-state"), "/home/u/demo-state", false},
		"a null byte":      {env("/a\x00b"), "", true},
		"a control string": {env("rel\x1b[31m"), "", true},
	} {
		got, err := Locate("/home/u", c.getenv)
		if c.bad {
			if err == nil || strings.ContainsAny(err.Error(), "\x1b\x00") || !strings.Contains(err.Error(), EnvDir) {
				t.Errorf("%s: want a clean error naming the variable, got %v", name, err)
			}
			continue
		}
		if err != nil || got.Dir != c.want {
			t.Errorf("%s: Locate = %q, %v; want %q", name, got.Dir, err, c.want)
		}
	}
}

func TestLocateRefusesTheConfigFolderWhereverItIs(t *testing.T) {
	getenv := func(dir string) func(string) string {
		return func(k string) string { return map[string]string{EnvDir: dir, "CLAUDE_CONFIG_DIR": "/data/claude"}[k] }
	}
	for _, dir := range []string{"/data/claude", "/data/claude/sessions", "/data"} {
		if _, err := Locate("/home/u", getenv(dir)); err == nil {
			t.Errorf("%s was accepted", dir)
		}
	}
	if s, err := Locate("/home/u", getenv("/data/demo-state")); err != nil || s.Dir != "/data/demo-state" {
		t.Errorf("a folder beside it: %q %v", s.Dir, err)
	}
}

func TestTryLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	unlock, ok := TryLock(p)
	if !ok {
		t.Fatal("free lock not taken")
	}
	if _, ok := TryLock(p); ok {
		t.Fatal("a held lock was taken")
	}
	unlock()
	if u, ok := TryLock(p); !ok {
		t.Fatal("released lock not taken")
	} else {
		u()
	}
}

func TestTryLockRefusesLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "lock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, ok := TryLock(link); ok {
		t.Fatal("took a lock through a link")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("made a file through a link")
	}
}
