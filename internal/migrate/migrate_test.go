package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

const id = "13159d08-594f-49e6-bdb5-f37c13cadbec"

func newStore(t *testing.T) state.Store {
	t.Helper()
	s := state.Open(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	return s
}

func put(t *testing.T, s state.Store, name, body string, at time.Time) {
	t.Helper()
	p := filepath.Join(s.Dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var long = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func TestMovesEveryFileOfASessionAndKeepsTimes(t *testing.T) {
	s := newStore(t)
	files := map[string]string{
		id + ".txt": "Objective: x\n", id + ".queue": "a\nb\n", id + ".cwd": "/w ", "." + id + ".state": "waiting\n", "." + id + ".count": "7\n",
		"." + id + ".stamp": "", "." + id + ".pending": "1 abc\n", "." + id + ".pinned": "", "." + id + ".snooze": "123\n", "." + id + ".lock": "",
	}
	i := 0
	for name, body := range files {
		put(t, s, name, body, long.Add(time.Duration(i)*time.Hour))
		i++
	}
	r := Run(s, false)
	if r.Moved != 10 || r.Errors != 0 || r.Left != 0 || r.Deleted != 0 {
		t.Fatalf("%+v", r)
	}
	want := map[string]string{"notes": id + ".txt", "queue": id + ".queue", "cwd": id + ".cwd", "state": "." + id + ".state", "count": "." + id + ".count",
		"stamp": "." + id + ".stamp", "pending": "." + id + ".pending", "pinned": "." + id + ".pinned", "snooze": "." + id + ".snooze", "lock": "." + id + ".lock"}
	for file, old := range want {
		p := s.Path(state.ID(id), file)
		if got := read(t, p); got != files[old] {
			t.Errorf("%s holds %q, want %q", file, got, files[old])
		}
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", file, fi.Mode().Perm())
		}
		if _, err := os.Lstat(filepath.Join(s.Dir, old)); err == nil {
			t.Errorf("%s is still in the root", old)
		}
	}
	if got := s.Sessions(); len(got) != 1 || got[0] != id {
		t.Errorf("sessions %v", got)
	}
	if fi, _ := os.Stat(s.SessionDir(state.ID(id))); fi.Mode().Perm() != 0o700 {
		t.Errorf("session folder mode %v", fi.Mode().Perm())
	}
}

func TestTimesAreKept(t *testing.T) {
	s := newStore(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	put(t, s, "."+id+".state", "working\n", at)
	put(t, s, id+".txt", "x", at.Add(time.Hour))
	Run(s, false)
	for file, want := range map[string]time.Time{"state": at, "notes": at.Add(time.Hour)} {
		fi, err := os.Stat(s.Path(state.ID(id), file))
		if err != nil || !fi.ModTime().Equal(want) {
			t.Errorf("%s: %v, %v; want %v", file, fi.ModTime(), err, want)
		}
	}
}

func TestLeavesWhatIsNotOurs(t *testing.T) {
	s := newStore(t)
	for name, body := range map[string]string{
		"inbox.queue": "inbox item\n", "sitrep.conf": "SITREP_QUEUE_ITEMS=7\n", ".queue.lock": "", "a.b.txt": "x", "x.y.z.queue": "x", "unknown.bogus": "x", ".sitrep.AbCd12": "tmp",
		".abc.state.tmp": "x", "bad name.txt": "x", "noext": "x", ".txt": "x", "inbox.txt": "x", ".inbox.state": "x", "readme": "x",
	} {
		put(t, s, name, body, long)
	}
	os.MkdirAll(filepath.Join(s.Dir, "bin"), 0o700)
	os.WriteFile(filepath.Join(s.Dir, "bin", "sitrep"), []byte("binary"), 0o700)
	os.MkdirAll(filepath.Join(s.Dir, "folder0001.txt"), 0o700) // a folder named like a notes file
	syscall.Mkfifo(filepath.Join(s.Dir, "pipe0001.queue"), 0o600)
	elsewhere := filepath.Join(t.TempDir(), "target")
	os.WriteFile(elsewhere, []byte("secret"), 0o600)
	os.Symlink(elsewhere, filepath.Join(s.Dir, "link0001.txt"))
	before := listing(t, s.Dir)
	r := Run(s, false)
	if r.Moved != 0 || r.Left != 0 || r.Deleted != 0 || r.Errors != 0 || len(r.Lines) != 0 {
		t.Fatalf("%+v", r)
	}
	if after := listing(t, s.Dir); strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("the folder changed:\n%v\n%v", before, after)
	}
	if read(t, elsewhere) != "secret" {
		t.Error("a link was followed")
	}
}

func listing(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel+"|"+fi.Mode().String())
		}
		return nil
	})
	return out
}

func TestRunTwiceChangesNothingTheSecondTime(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".txt", "x", long)
	put(t, s, id+".queue", "a\n", long)
	Run(s, false)
	before := listing(t, s.Dir)
	r := Run(s, false)
	if r.Moved != 0 || r.Errors != 0 || len(r.Lines) != 0 || strings.Join(before, ",") != strings.Join(listing(t, s.Dir), ",") {
		t.Errorf("%+v", r)
	}
}

func TestNeverOverwritesNewerState(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".txt", "old", long)
	Run(s, false)
	os.WriteFile(s.Path(state.ID(id), "notes"), []byte("new"), 0o600)
	put(t, s, id+".txt", "an old program wrote this again", long) // a hook of the old program, after the move
	r := Run(s, false)
	if r.Left != 1 || r.Moved != 0 || read(t, s.Path(state.ID(id), "notes")) != "new" {
		t.Fatalf("%+v, notes %q", r, read(t, s.Path(state.ID(id), "notes")))
	}
	if read(t, filepath.Join(s.Dir, id+".txt")) != "an old program wrote this again" {
		t.Error("the flat file was removed")
	}
	if !strings.Contains(r.Lines[0], "left "+id+".txt: sessions/"+id+"/notes exists") {
		t.Errorf("%q", r.Lines)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".txt", "x", long)
	put(t, s, "."+id+".prompt", "x", long)
	before := listing(t, s.Dir)
	r := Run(s, true)
	if r.Moved != 1 || r.Deleted != 1 || strings.Join(before, ",") != strings.Join(listing(t, s.Dir), ",") {
		t.Fatalf("%+v", r)
	}
	for _, l := range r.Lines {
		if !strings.HasPrefix(l, "would ") {
			t.Errorf("%q", l)
		}
	}
	if _, err := os.Stat(s.SessionsDir()); err == nil {
		t.Error("a dry run made the sessions folder")
	}
}

func TestObsoletePromptFilesAreDeleted(t *testing.T) {
	s := newStore(t)
	put(t, s, "."+id+".prompt", "p", long)
	r := Run(s, false)
	if r.Deleted != 1 {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Lstat(filepath.Join(s.Dir, "."+id+".prompt")); err == nil {
		t.Error("still there")
	}
	// a session that has only an obsolete file does not become a folder
	if _, err := os.Stat(s.SessionDir(state.ID(id))); err == nil {
		t.Error("a folder was made for a deleted file")
	}
}

func TestSeveralSessionsAndTheInbox(t *testing.T) {
	s := newStore(t)
	for _, sid := range []string{"aaaa", "bbbb", "cccc-1"} {
		put(t, s, sid+".txt", "n "+sid, long)
		put(t, s, sid+".queue", "q "+sid, long)
	}
	put(t, s, "inbox.queue", "i", long)
	r := Run(s, false)
	if r.Moved != 6 {
		t.Fatalf("%+v", r)
	}
	for _, sid := range []string{"aaaa", "bbbb", "cccc-1"} {
		if read(t, s.Path(state.ID(sid), "queue")) != "q "+sid || read(t, s.Path(state.ID(sid), "notes")) != "n "+sid {
			t.Errorf("%s", sid)
		}
	}
	if read(t, filepath.Join(s.Dir, "inbox.queue")) != "i" {
		t.Error("the inbox moved")
	}
	if len(s.Sessions()) != 3 {
		t.Errorf("%v", s.Sessions())
	}
}

func TestNotesAreMovedLast(t *testing.T) {
	s := newStore(t)
	for _, f := range []string{id + ".queue", "." + id + ".state", id + ".txt", "." + id + ".count", id + ".cwd"} {
		put(t, s, f, "x", long)
	}
	r := Run(s, false)
	if len(r.Lines) != 5 || !strings.HasPrefix(r.Lines[4], "moved "+id+".txt ") {
		t.Errorf("%q", r.Lines)
	}
}

func TestAQueueInBothPlacesIsMergedOldItemsFirst(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".queue", "old one\nold two", long) // no final newline
	put(t, s, id+".txt", "x", long)
	s.EnsureSession(state.ID(id))
	os.WriteFile(s.Path(state.ID(id), "queue"), []byte("new one\n"), 0o600) // a hook of the new program queued this after the upgrade
	r := Run(s, false)
	if r.Merged != 1 || r.Errors != 0 {
		t.Fatalf("%+v: %q", r, r.Lines)
	}
	if got := read(t, s.Path(state.ID(id), "queue")); got != "old one\nold two\nnew one\n" {
		t.Errorf("queue %q", got)
	}
	if _, err := os.Lstat(filepath.Join(s.Dir, id+".queue")); err == nil {
		t.Error("the old queue is still there")
	}
	if fi, _ := os.Stat(s.Path(state.ID(id), "queue")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func TestAnInterruptedRunIsFinished(t *testing.T) {
	s := newStore(t)
	put(t, s, "."+id+".state", "working\n", long)
	s.EnsureSession(state.ID(id))
	// the link was made and the old name was not yet removed: the same file under both names
	if err := os.Link(filepath.Join(s.Dir, "."+id+".state"), s.Path(state.ID(id), "state")); err != nil {
		t.Fatal(err)
	}
	r := Run(s, false)
	if r.Moved != 1 || r.Left != 0 || r.Errors != 0 {
		t.Fatalf("%+v: %q", r, r.Lines)
	}
	if _, err := os.Lstat(filepath.Join(s.Dir, "."+id+".state")); err == nil {
		t.Error("the old name is still there")
	}
	if read(t, s.Path(state.ID(id), "state")) != "working\n" {
		t.Error("the state was lost")
	}
}

func TestLooseModesAreTightened(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".txt", "x", long)
	os.Chmod(filepath.Join(s.Dir, id+".txt"), 0o644)
	Run(s, false)
	if fi, _ := os.Stat(s.Path(state.ID(id), "notes")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func TestASessionsFolderThatIsNotAFolderStopsTheRun(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".txt", "x", long)
	os.Symlink(t.TempDir(), s.SessionsDir())
	before := listing(t, s.Dir)
	r := Run(s, false)
	if r.Errors != 1 || r.Moved != 0 || strings.Join(before, ",") != strings.Join(listing(t, s.Dir), ",") {
		t.Errorf("%+v %q", r, r.Lines)
	}
	os.Remove(s.SessionsDir())
	os.WriteFile(s.SessionsDir(), []byte("a file"), 0o600)
	if r := Run(s, false); r.Errors != 1 || r.Moved != 0 {
		t.Errorf("a file: %+v", r)
	}
}

func TestASessionFolderThatIsALinkIsNeverWrittenThrough(t *testing.T) {
	s := newStore(t)
	put(t, s, id+".queue", "x", long)
	elsewhere := t.TempDir()
	os.MkdirAll(s.SessionsDir(), 0o700)
	os.Symlink(elsewhere, s.SessionDir(state.ID(id)))
	r := Run(s, false)
	if r.Errors != 1 || r.Moved != 0 {
		t.Fatalf("%+v %q", r, r.Lines)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Errorf("a file was written through the link: %v", entries)
	}
	if read(t, filepath.Join(s.Dir, id+".queue")) != "x" {
		t.Error("the file was lost")
	}
}

func TestPending(t *testing.T) {
	s := newStore(t)
	if Pending(s) != 0 {
		t.Error("an empty folder has pending files")
	}
	put(t, s, id+".txt", "x", long)
	put(t, s, "."+id+".state", "x", long)
	put(t, s, "."+id+".prompt", "x", long) // obsolete: not worth a note
	put(t, s, "inbox.queue", "x", long)
	if got := Pending(s); got != 2 {
		t.Errorf("Pending = %d", got)
	}
	Run(s, false)
	if Pending(s) != 0 {
		t.Error("pending files after a run")
	}
}

func TestOnlyTheNamesOfEachKindAreMoved(t *testing.T) {
	s := newStore(t)
	for _, name := range []string{"abc.state", ".abc.txt", ".abc.queue", ".abc.cwd", "abc.count"} { // the wrong shape for their kind
		put(t, s, name, "x", long)
	}
	if r := Run(s, false); r.Moved != 0 || len(r.Lines) != 0 {
		t.Errorf("%+v %q", r, r.Lines)
	}
}

func TestAnErrorIsReportedAndTheRestIsMoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	s := newStore(t)
	put(t, s, "aaaa.txt", "x", long)
	put(t, s, "bbbb.txt", "y", long)
	os.MkdirAll(s.SessionsDir(), 0o700)
	os.MkdirAll(s.SessionDir("aaaa"), 0o500) // the files of aaaa cannot be moved in
	defer os.Chmod(s.SessionDir("aaaa"), 0o700)
	r := Run(s, false)
	if r.Errors != 1 || r.Moved != 1 {
		t.Fatalf("%+v: %q", r, r.Lines)
	}
	if read(t, s.Path("bbbb", "notes")) != "y" || read(t, filepath.Join(s.Dir, "aaaa.txt")) != "x" {
		t.Error("the failed file was lost, or the good one was not moved")
	}
	// and a run after the problem is gone finishes the job
	os.Chmod(s.SessionDir("aaaa"), 0o700)
	if r := Run(s, false); r.Moved != 1 || r.Errors != 0 {
		t.Errorf("%+v", r)
	}
}

func TestNoStateFolder(t *testing.T) {
	r := Run(state.Store{Dir: filepath.Join(t.TempDir(), "none")}, false)
	if r.Moved+r.Errors+r.Left+r.Deleted != 0 {
		t.Errorf("%+v", r)
	}
}

func TestAStateFolderThatIsALink(t *testing.T) {
	real := t.TempDir()
	s := state.Store{Dir: filepath.Join(t.TempDir(), "sitrep")}
	if err := os.Symlink(real, s.Dir); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(real, id+".txt"), []byte("x"), 0o600)
	if r := Run(s, false); r.Moved != 1 || r.Errors != 0 {
		t.Fatalf("%+v", r)
	}
	if read(t, filepath.Join(real, "sessions", id, "notes")) != "x" {
		t.Error("not moved into the folder the link points to")
	}
	if fi, _ := os.Lstat(s.Dir); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
}

func TestHostileFileNamesArePrintedSafely(t *testing.T) {
	s := newStore(t)
	put(t, s, "ab\x1b[31mcd.txt", "x", long) // not a valid id: left alone and not printed at all
	put(t, s, id+".txt", "x", long)
	r := Run(s, false)
	for _, l := range r.Lines {
		if strings.ContainsAny(l, "\x1b") {
			t.Errorf("%q", l)
		}
	}
}
