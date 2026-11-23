package queue

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/histlog"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

func newQueue(t *testing.T, id state.ID, body string) Queue {
	t.Helper()
	s := state.Open(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	s.EnsureSession(id)
	q := Queue{Store: s}
	if body != "" {
		if err := os.WriteFile(q.file(id), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return q
}

func read(t *testing.T, q Queue, id state.ID) string {
	t.Helper()
	b, err := os.ReadFile(q.file(id))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLabel(t *testing.T) {
	for id, want := range map[state.ID]string{"inbox": "inbox", "15f1aaaa-bbbb": "15f1", "ab": "ab", "abcd": "abcd"} {
		if got := Label(id); got != want {
			t.Errorf("Label(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestAddAndCount(t *testing.T) {
	q := newQueue(t, "s1", "")
	for i, want := range []int{1, 2} {
		n, err := q.Add("s1", textsafe.Clean("item", 0))
		if err != nil || n != want {
			t.Fatalf("add %d: %d, %v", i, n, err)
		}
	}
	if _, err := q.Add("s1", textsafe.Clean("  \t ", 0)); !errors.Is(err, ErrBlank) {
		t.Errorf("blank add: %v", err)
	}
	if fi, _ := os.Stat(q.file("s1")); fi.Mode().Perm() != state.FileMode {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if got := q.Count("nothing"); got != 0 {
		t.Errorf("count of a missing queue = %d", got)
	}
}

func TestAddCreatesTheFolder(t *testing.T) {
	q := Queue{Store: state.Open(t.TempDir())}
	if _, err := q.Add("inbox", textsafe.Clean("x", 0)); err != nil {
		t.Fatal(err)
	}
}

func TestRemove(t *testing.T) {
	q := newQueue(t, "s1", "a\nb\nc\n")
	got, err := q.Remove("s1", 2, nil)
	if err != nil || got.String() != "b" || read(t, q, "s1") != "a\nc\n" {
		t.Fatalf("got %q, %v, file %q", got, err, read(t, q, "s1"))
	}
	var no *NoItemError
	if _, err := q.Remove("s1", 3, nil); !errors.As(err, &no) || err.Error() != "no item 3 on s1" {
		t.Errorf("past the end: %v", err)
	}
	if _, err := q.Remove("s1", 0, nil); !errors.As(err, &no) {
		t.Errorf("item 0: %v", err)
	}
	if _, err := q.Remove("missing", 1, nil); !errors.Is(err, ErrEmpty) {
		t.Errorf("missing queue: %v", err)
	}
}

func TestGuard(t *testing.T) {
	q := newQueue(t, "s1", "a\nb\n")
	var ch *ChangedError
	for _, wrong := range []string{"x", "b"} { // b is item 2, not item 1
		g := wrong
		if _, err := q.Remove("s1", 1, &g); !errors.As(err, &ch) || err.Error() != "queue changed; run 'sitrep ls s1'" {
			t.Errorf("guard %q: %v", wrong, err)
		}
	}
	if read(t, q, "s1") != "a\nb\n" {
		t.Error("a refused remove changed the queue")
	}
	g := "a  " // trailing spaces do not count
	if _, err := q.Remove("s1", 1, &g); err != nil {
		t.Errorf("guard with trailing spaces: %v", err)
	}
}

func TestReorder(t *testing.T) {
	for _, c := range []struct {
		from, to int
		want     string
	}{
		{3, 1, "c\na\nb\nd\n"}, {1, 3, "b\nc\na\nd\n"}, {2, 2, "a\nb\nc\nd\n"}, {4, 1, "d\na\nb\nc\n"}, {1, 4, "b\nc\nd\na\n"},
	} {
		q := newQueue(t, "s1", "a\nb\nc\nd\n")
		if _, err := q.Reorder("s1", c.from, c.to, nil); err != nil || read(t, q, "s1") != c.want {
			t.Errorf("%d to %d: %v, file %q, want %q", c.from, c.to, err, read(t, q, "s1"), c.want)
		}
	}
	q := newQueue(t, "s1", "a\nb\n")
	var rg *RangeError
	for _, ft := range [][2]int{{3, 1}, {1, 3}, {0, 1}, {1, 0}} {
		if _, err := q.Reorder("s1", ft[0], ft[1], nil); !errors.As(err, &rg) || err.Error() != "s1 has 2 items" {
			t.Errorf("%v: %v", ft, err)
		}
	}
	g := "b"
	var ch *ChangedError
	if _, err := q.Reorder("s1", 1, 2, &g); !errors.As(err, &ch) {
		t.Errorf("guard: %v", err)
	}
}

func TestMove(t *testing.T) {
	q := newQueue(t, "s1", "a\nb\n")
	if err := os.WriteFile(q.file("inbox"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := q.Move("s1", "inbox", 2, nil); err != nil || got.String() != "b" {
		t.Fatalf("%q, %v", got, err)
	}
	if read(t, q, "s1") != "a\n" || read(t, q, "inbox") != "x\nb\n" {
		t.Errorf("s1 %q, inbox %q", read(t, q, "s1"), read(t, q, "inbox"))
	}
	var no *NoItemError
	if _, err := q.Move("s1", "inbox", 5, nil); !errors.As(err, &no) {
		t.Errorf("past the end: %v", err)
	}
	g := "nope"
	var ch *ChangedError
	if _, err := q.Move("s1", "inbox", 1, &g); !errors.As(err, &ch) || read(t, q, "inbox") != "x\nb\n" {
		t.Errorf("guard: %v, inbox %q", err, read(t, q, "inbox"))
	}
}

func TestDisplayIsSafe(t *testing.T) {
	q := newQueue(t, "s1", "ok\x1b]0;t\x07 ‮x\n")
	got, err := q.Remove("s1", 1, nil)
	if err != nil || got.String() != "ok]0;t x" {
		t.Errorf("%q, %v", got, err)
	}
}

func TestLines(t *testing.T) {
	q := newQueue(t, "s1", "a\n\nb\n")
	if n := q.Count("s1"); n != 2 {
		t.Errorf("count %d, want 2 (blank lines do not count)", n)
	}
	got, _ := q.lines("s1")
	if !reflect.DeepEqual(got, []string{"a", "", "b"}) {
		t.Errorf("lines %q", got)
	}
}

// Changes to a queue must not lose one another's work: an append that lands between a reorder's read and its rewrite used to vanish.
func TestConcurrentChangesLoseNothing(t *testing.T) {
	q := newQueue(t, "s1", "seed\n")
	const writers, each = 8, 40
	var wg sync.WaitGroup
	stop := make(chan struct{})
	var errs atomic.Int64
	wg.Add(1)
	go func() { // keeps rewriting the file
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if n := q.Count("s1"); n > 1 {
				if _, err := q.Reorder("s1", 1, n, nil); err != nil {
					errs.Add(1)
				}
			}
		}
	}()
	var adders sync.WaitGroup
	for w := 0; w < writers; w++ {
		adders.Add(1)
		go func(w int) {
			defer adders.Done()
			for i := 0; i < each; i++ {
				if _, err := q.Add("s1", textsafe.Clean(fmt.Sprintf("w%d-%d", w, i), 0)); err != nil {
					errs.Add(1)
				}
			}
		}(w)
	}
	adders.Wait()
	close(stop)
	wg.Wait()
	if errs.Load() != 0 {
		t.Errorf("%d operations failed", errs.Load())
	}
	if got := q.Count("s1"); got != 1+writers*each {
		t.Errorf("%d items, want %d: appends were lost", got, 1+writers*each)
	}
}

func TestMovesBetweenQueuesLoseAndDuplicateNothing(t *testing.T) {
	q := newQueue(t, "s1", "")
	for i := 0; i < 100; i++ {
		q.Add("inbox", textsafe.Clean(fmt.Sprintf("item%d", i), 0))
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if _, err := q.Move("inbox", "s1", 1, nil); err != nil {
					return
				}
			}
		}()
	}
	wg.Wait()
	if a, b := q.Count("inbox"), q.Count("s1"); a != 0 || b != 100 {
		t.Errorf("inbox %d, session %d", a, b)
	}
	seen := map[string]bool{}
	lines, _ := q.lines("s1")
	for _, l := range lines {
		if seen[l] {
			t.Errorf("%q twice", l)
		}
		seen[l] = true
	}
}

func TestChangesWorkBeforeTheFolderExists(t *testing.T) {
	q := Queue{Store: state.Open(t.TempDir())}
	if _, err := q.Remove("inbox", 1, nil); !errors.Is(err, ErrEmpty) {
		t.Errorf("%v", err)
	}
	if _, err := q.Reorder("inbox", 1, 1, nil); err == nil {
		t.Error("reordering nothing worked")
	}
}

func TestEdit(t *testing.T) {
	q := newQueue(t, "s1", "a\nb\nc\n")
	old, err := q.Edit("s1", 2, textsafe.Clean("better", MaxItem), nil)
	if err != nil || old.String() != "b" || read(t, q, "s1") != "a\nbetter\nc\n" {
		t.Errorf("edit: %v, %q, file %q", err, old, read(t, q, "s1"))
	}
	g := "better"
	if _, err := q.Edit("s1", 2, textsafe.Clean("again", MaxItem), &g); err != nil || read(t, q, "s1") != "a\nagain\nc\n" {
		t.Errorf("guarded: %v, file %q", err, read(t, q, "s1"))
	}
	var ch *ChangedError
	if _, err := q.Edit("s1", 2, textsafe.Clean("x", MaxItem), &g); !errors.As(err, &ch) || read(t, q, "s1") != "a\nagain\nc\n" {
		t.Errorf("stale guard: %v, file %q", err, read(t, q, "s1"))
	}
	var no *NoItemError
	for _, n := range []int{0, 4, -1} {
		if _, err := q.Edit("s1", n, textsafe.Clean("x", MaxItem), nil); !errors.As(err, &no) {
			t.Errorf("item %d: %v", n, err)
		}
	}
	if _, err := q.Edit("s1", 1, textsafe.Clean("  ", MaxItem), nil); !errors.Is(err, ErrBlank) || read(t, q, "s1") != "a\nagain\nc\n" {
		t.Errorf("blank: %v, file %q", err, read(t, q, "s1"))
	}
	if _, err := newQueue(t, "s2", "").Edit("s2", 1, textsafe.Clean("x", MaxItem), nil); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty: %v", err)
	}
}

func TestEditKeepsTheOtherLinesAsTheyAre(t *testing.T) {
	q := newQueue(t, "s1", "a \n\nc\n") // a trailing space, and a blank line, which counts as a position but holds no item
	var no *NoItemError
	if _, err := q.Edit("s1", 2, textsafe.Clean("z", MaxItem), nil); !errors.As(err, &no) || read(t, q, "s1") != "a \n\nc\n" {
		t.Errorf("the blank line: %v, %q", err, read(t, q, "s1"))
	}
	if _, err := q.Edit("s1", 3, textsafe.Clean("z", MaxItem), nil); err != nil || read(t, q, "s1") != "a \n\nz\n" {
		t.Errorf("%v, %q", err, read(t, q, "s1"))
	}
}

func TestLogRecordsOnlyWhatSucceeded(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	l := histlog.Log{Dir: st.Dir, Host: "h", On: true}
	q := Queue{Store: st, Log: l}
	for _, s := range []string{"a", "b"} {
		if _, err := q.Add(state.Inbox, textsafe.Clean(s, 200)); err != nil {
			t.Fatal(err)
		}
	}
	guard := "wrong"
	if _, err := q.Remove(state.Inbox, 1, &guard); err == nil { // refused: nothing changed, nothing logged
		t.Fatal("guard ignored")
	}
	if _, err := q.Reorder(state.Inbox, 2, 1, nil); err != nil {
		t.Fatal(err)
	}
	var ops []string
	for _, it := range histlog.Read(st.Dir, histlog.Query{}) {
		ops = append(ops, it.Op)
	}
	if strings.Join(ops, " ") != "queue.add queue.add queue.reorder" {
		t.Fatalf("ops %v", ops)
	}
	// the zero Log is off
	if _, err := (Queue{Store: st}).Add(state.Inbox, textsafe.Clean("c", 200)); err != nil {
		t.Fatal(err)
	}
	if got := len(histlog.Read(st.Dir, histlog.Query{})); got != 3 {
		t.Fatalf("%d entries", got)
	}
}

func TestAddRefusesALinkAndAPipeInPlaceOfTheQueue(t *testing.T) {
	q := newQueue(t, "abc123", "")
	victim := q.Store.Dir + "/victim"
	os.WriteFile(victim, []byte("keep\n"), 0o600)
	if err := os.Symlink(victim, q.file("abc123")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Add("abc123", textsafe.Clean("hello", 0)); err == nil {
		t.Error("an item was added through a link")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep\n" {
		t.Errorf("the target of the link changed: %q", b)
	}
	os.Remove(q.file("abc123"))
	if err := syscall.Mkfifo(q.file("abc123"), 0o600); err != nil {
		t.Skip("no fifo here")
	}
	done := make(chan error, 1)
	go func() { _, err := q.Add("abc123", textsafe.Clean("hello", 0)); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("an item was added to a pipe")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("adding to a pipe blocked while holding the queue lock")
	}
	// the lock is free again, so another change goes through
	if _, err := q.Add("inbox", textsafe.Clean("other", 0)); err != nil {
		t.Errorf("the lock stayed held: %v", err)
	}
}

func TestAddStopsAtTheSizeCap(t *testing.T) {
	q := newQueue(t, "abc123", "")
	if err := os.WriteFile(q.file("abc123"), make([]byte, MaxFileBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Add("abc123", textsafe.Clean("one more", 0)); !errors.Is(err, ErrFull) {
		t.Errorf("a full queue file takes no more: %v", err)
	}
}
