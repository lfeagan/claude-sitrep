package histlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

var t0 = time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)

func newLog(t *testing.T) (Log, *time.Time) {
	now := t0
	return Log{Dir: t.TempDir(), Host: "mbp", On: true, MaxMB: 10, Days: 30, Now: func() time.Time { return now }}, &now
}

func txt(s string) textsafe.Text { return textsafe.Clean(s, 200) }

func TestOffWritesNothing(t *testing.T) {
	l, _ := newLog(t)
	l.On = false
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("x")})
	if _, err := os.Stat(filepath.Join(l.Dir, "log")); !os.IsNotExist(err) {
		t.Fatalf("log folder made while off: %v", err)
	}
	if (Log{}).On {
		t.Fatal("zero Log is on")
	}
	Log{Dir: l.Dir, Host: "mbp"}.Record(Entry{Op: QueueAdd, ID: "inbox"})
}

func TestRecordAndRead(t *testing.T) {
	l, now := newLog(t)
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("fix the flaky test")})
	*now = now.Add(time.Minute)
	l.Record(Entry{Op: QueueMove, ID: "inbox", Dest: "c5d0", Text: txt("fix the flaky test")})
	*now = now.Add(time.Minute)
	l.Record(Entry{Op: QueueReorder, ID: "c5d0", Text: txt("a"), From: 3, To: 1})
	l.Record(Entry{Op: QueueEdit, ID: "c5d0", Text: txt("new"), Old: txt("old")})
	got := Read(l.Dir, Query{})
	if len(got) != 4 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0].Op != QueueAdd || got[1].Dest != "c5d0" || got[2].From != 3 || got[2].To != 1 || got[3].Old.String() != "old" || got[3].Text.String() != "new" {
		t.Fatalf("wrong entries: %+v", got)
	}
	if got := Read(l.Dir, Query{ID: "c5d0"}); len(got) != 3 { // the move counts for its destination
		t.Fatalf("by queue: %d", len(got))
	}
	if got := Read(l.Dir, Query{Op: QueueEdit}); len(got) != 1 {
		t.Fatalf("by op: %d", len(got))
	}
	if got := Read(l.Dir, Query{Limit: 2}); len(got) != 2 || got[1].Op != QueueEdit {
		t.Fatalf("limit: %+v", got)
	}
	if got := Read(l.Dir, Query{Since: t0.Add(90 * time.Second)}); len(got) != 2 {
		t.Fatalf("since: %d", len(got))
	}
}

func TestModes(t *testing.T) {
	l, _ := newLog(t)
	l.Record(Entry{Op: Pin, ID: "c5d0", Text: txt("x")})
	for p, want := range map[string]os.FileMode{filepath.Join(l.Dir, "log"): 0o700, l.hostDir(): 0o700, l.current(): 0o600} {
		fi, err := os.Stat(p)
		if err != nil || fi.Mode().Perm() != want {
			t.Fatalf("%s: %v %v", p, fi, err)
		}
	}
}

func TestDropsBadAndOversize(t *testing.T) {
	l, _ := newLog(t)
	l.Record(Entry{Op: "nope", ID: "inbox"})
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt(strings.Repeat("\"\\", 100))}) // escapes double: still fits
	if got := Read(l.Dir, Query{}); len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	if got := Read(l.Dir, Query{})[0].Text.String(); got != strings.Repeat("\"\\", 100) {
		t.Fatal("text not kept as it was")
	}
}

func TestDoesNotFollowLink(t *testing.T) {
	l, _ := newLog(t)
	if err := l.ensure(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(target, l.current()); err != nil {
		t.Fatal(err)
	}
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("x")})
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("the entry was written through a link")
	}
}

func TestRefusesLinkedFolder(t *testing.T) {
	l, _ := newLog(t)
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(l.Dir, "log")); err != nil {
		t.Fatal(err)
	}
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("x")})
	if m, _ := filepath.Glob(filepath.Join(other, "*")); len(m) != 0 {
		t.Fatalf("wrote through a linked folder: %v", m)
	}
}

func TestRollBySize(t *testing.T) {
	l, _ := newLog(t)
	l.MaxMB = 1 // current rolls at 512 KB
	if err := l.ensure(); err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat("x", 600<<10) // one line is far over maxLine, so write it directly
	if err := os.WriteFile(l.current(), []byte(pad+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("after")})
	if fi, err := os.Stat(l.previous()); err != nil || fi.Size() < 600<<10 {
		t.Fatalf("previous not made: %v", err)
	}
	b, _ := os.ReadFile(l.current())
	if !strings.Contains(string(b), "after") || len(b) > 1000 {
		t.Fatalf("current is not fresh: %d bytes", len(b))
	}
}

func TestRollByAge(t *testing.T) {
	l, now := newLog(t)
	l.Days = 10
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("old")})
	*now = now.Add(4 * 24 * time.Hour) // under 5 days: no roll
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("mid")})
	if _, err := os.Stat(l.previous()); !os.IsNotExist(err) {
		t.Fatal("rolled too early")
	}
	*now = now.Add(2 * 24 * time.Hour)
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("new")})
	if _, err := os.Stat(l.previous()); err != nil {
		t.Fatal("did not roll at half the age")
	}
	b, _ := os.ReadFile(l.current())
	if strings.Count(string(b), "\n") != 1 {
		t.Fatalf("current should hold the new entry only: %q", b)
	}
}

func TestRollSkippedWhenLocked(t *testing.T) {
	l, _ := newLog(t)
	l.MaxMB = 1
	if err := l.ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.current(), []byte(strings.Repeat("x", 600<<10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	unlock, ok := state.TryLock(filepath.Join(l.hostDir(), "lock"))
	if !ok {
		t.Fatal("no lock")
	}
	defer unlock()
	start := time.Now()
	l.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("x")})
	if time.Since(start) > time.Second {
		t.Fatal("waited for the lock")
	}
	if _, err := os.Stat(l.previous()); !os.IsNotExist(err) {
		t.Fatal("rolled without the lock")
	}
	if !strings.Contains(readAll(t, l.current()), `"op":"queue.add"`) {
		t.Fatal("entry not appended")
	}
}

func readAll(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReaderSkipsBadLinesRepeatsAndOld(t *testing.T) {
	l, now := newLog(t)
	if err := l.ensure(); err != nil {
		t.Fatal(err)
	}
	good := `{"e":"aa","t":"2026-11-21T14:00:00Z","host":"mbp","id":"inbox","op":"queue.add","text":"one"}`
	old := `{"e":"bb","t":"2026-09-01T14:00:00Z","host":"mbp","id":"inbox","op":"queue.add","text":"old"}`
	badID := `{"e":"cc","t":"2026-11-21T14:00:00Z","host":"mbp","id":"../x","op":"queue.add"}`
	badOp := `{"e":"dd","t":"2026-11-21T14:00:00Z","host":"mbp","id":"inbox","op":"rm -rf"}`
	otherHost := `{"e":"ee","t":"2026-11-21T14:00:00Z","host":"evil","id":"inbox","op":"queue.add"}`
	torn := `{"e":"ff","t":"2026-11-21T14:00:0`
	body := strings.Join([]string{old, "garbage", good, good, badID, badOp, otherHost, torn}, "\n")
	if err := os.WriteFile(l.current(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Read(l.Dir, Query{Days: 30, Now: *now})
	if len(got) != 1 || got[0].E != "aa" {
		t.Fatalf("got %+v", got)
	}
}

func TestReaderCleansText(t *testing.T) {
	l, _ := newLog(t)
	if err := l.ensure(); err != nil {
		t.Fatal(err)
	}
	line := `{"e":"aa","t":"2026-11-21T14:00:00Z","host":"mbp","id":"inbox","op":"queue.add","text":"a\u001b[31mred‮b"}` + "\n"
	if err := os.WriteFile(l.current(), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Read(l.Dir, Query{})
	if len(got) != 1 || got[0].Text.String() != "a[31mredb" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadsHostsMergedAndOneHost(t *testing.T) {
	a, now := newLog(t)
	b := Log{Dir: a.Dir, Host: "desk", On: true, Now: func() time.Time { return now.Add(time.Minute) }}
	a.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("from mbp")})
	b.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("from desk")})
	if got := Read(a.Dir, Query{}); len(got) != 2 || got[0].Host != "mbp" || got[1].Host != "desk" {
		t.Fatalf("merge: %+v", got)
	}
	if got := Read(a.Dir, Query{Host: "desk"}); len(got) != 1 {
		t.Fatalf("host: %+v", got)
	}
	if got := Read(a.Dir, Query{Host: "../x"}); got != nil {
		t.Fatal("a bad host name was read")
	}
}

func TestClear(t *testing.T) {
	a, _ := newLog(t)
	b := Log{Dir: a.Dir, Host: "desk", On: true, Now: a.Now}
	a.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("x")})
	b.Record(Entry{Op: QueueAdd, ID: "inbox", Text: txt("y")})
	if err := a.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.hostDir()); !os.IsNotExist(err) {
		t.Fatal("host folder left")
	}
	if got := Read(a.Dir, Query{}); len(got) != 1 || got[0].Host != "desk" {
		t.Fatalf("another host's log was touched: %+v", got)
	}
	if err := a.Clear(); err != nil {
		t.Fatalf("clearing nothing failed: %v", err)
	}
}

func TestHost(t *testing.T) {
	for in, want := range map[string]string{"mbp": "mbp", "mbp.local": "mbp_local", "../x": "_x", "": "host", "inbox": "host", ".": "host", "a b": "a_b"} {
		if got := Host(in); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Host(strings.Repeat("a", 100)); len(got) != 64 {
		t.Errorf("long host: %d", len(got))
	}
}
