package render

import (
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

func tx(s string) textsafe.Text { return textsafe.Clean(s, 0) }

func queueOf(items ...string) snapshot.Queue {
	q := snapshot.Queue{Count: len(items), Items: []textsafe.Text{}}
	for _, i := range items {
		q.Items = append(q.Items, tx(i))
	}
	return q
}

func session(id, dir, label string, q snapshot.Queue) snapshot.Session {
	return snapshot.Session{ID: "x" + "id", Short: id, Dir: tx(dir), StateLabel: label, Objective: tx("obj " + id), Progress: tx("prog"), Now: tx("now"), Queue: q}
}

func TestEmpty(t *testing.T) {
	if got := Frame(snapshot.Snapshot{}, Opts{Cols: 100}); len(got) != 0 {
		t.Errorf("%q", got)
	}
}

func TestFrame(t *testing.T) {
	snap := snapshot.Snapshot{
		Inbox:    queueOf("one", "two", "three"),
		Sessions: []snapshot.Session{session("aaaa", "proj", "working 5m", queueOf("a", "b")), session("bbbb", "a-much-longer-name", "waiting 1h", queueOf())},
		Hidden:   snapshot.Hidden{Ended: 2, StrandedItems: 3},
	}
	want := strings.Join([]string{
		"Inbox: 3 items, next: one",
		"ID    DIRECTORY           STATE       QUEUE",
		"aaaa  proj                working 5m  2 items, next: a",
		"    Objective: obj aaaa",
		"    Now:       now",
		"bbbb  a-much-longer-name  waiting 1h  empty",
		"    Objective: obj bbbb",
		"    Now:       now",
		"+2 ended (--all)",
		"3 items stranded in ended sessions (sitrep prune)",
	}, "\n")
	snap.Inbox.Items = snap.Inbox.Items[:1] // the snapshot is built with Cap 1 here
	snap.Sessions[0].Queue.Items = snap.Sessions[0].Queue.Items[:1]
	if got := strings.Join(Frame(snap, Opts{Cols: 100}), "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestVerboseListsQueues(t *testing.T) {
	snap := snapshot.Snapshot{
		Inbox:    queueOf("one", "two", "three"),
		Sessions: []snapshot.Session{session("aaaa", "proj", "working 5m", queueOf("a", "b", "c"))},
	}
	snap.Inbox.Items = snap.Inbox.Items[:2]
	snap.Sessions[0].Queue.Items = snap.Sessions[0].Queue.Items[:2]
	want := strings.Join([]string{
		"Inbox: 3 items",
		"    1. one",
		"    2. two",
		"    +1 more",
		"ID    DIRECTORY  STATE       QUEUE",
		"aaaa  proj       working 5m  3 items",
		"    Objective: obj aaaa",
		"    Progress:  prog",
		"    Now:       now",
		"    Queue:     1. a",
		"               2. b",
		"               +1 more",
	}, "\n")
	if got := strings.Join(Frame(snap, Opts{Verbose: true, Items: 2, Cols: 100}), "\n"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// -n 0 adds the Progress line but lists no queue
	got := Frame(snap, Opts{Verbose: true, Items: 0, Cols: 100})
	if !strings.Contains(strings.Join(got, "\n"), "Progress:") || strings.Contains(strings.Join(got, "\n"), "Queue:") {
		t.Errorf("%q", got)
	}
}

func TestCuttingAndWidths(t *testing.T) {
	long := strings.Repeat("word ", 40)
	snap := snapshot.Snapshot{
		Inbox:    queueOf(long),
		Sessions: []snapshot.Session{session("aaaa", "世界", "working 5m", queueOf(long))},
	}
	snap.Inbox.Items = snap.Inbox.Items[:1]
	snap.Sessions[0].Queue.Items = snap.Sessions[0].Queue.Items[:1]
	for _, l := range Frame(snap, Opts{Cols: 40}) {
		if strings.HasPrefix(l, "    Objective") || strings.HasPrefix(l, "    Now") {
			continue // fields are not cut here
		}
		if w := textsafe.Width(tx(l)); w > 40 && strings.HasPrefix(l, "Inbox") {
			t.Errorf("inbox line is %d columns: %q", w, l)
		}
	}
	lines := Frame(snap, Opts{Cols: 40})
	if !strings.HasSuffix(lines[0], "…") {
		t.Errorf("the inbox line should end in an ellipsis: %q", lines[0])
	}
	// the queue column keeps at least 10 columns, however narrow the terminal
	row := Frame(snap, Opts{Cols: 30})[2]
	if !strings.Contains(row, "…") || textsafe.Width(tx(row[strings.LastIndex(row, "  ")+2:])) > 10 {
		t.Errorf("row %q", row)
	}
	// a wide directory name pads by columns
	if !strings.HasPrefix(lines[2], "aaaa  世界       working 5m") {
		t.Errorf("padding: %q", lines[2])
	}
}

func TestEveryLineIsClean(t *testing.T) {
	s := session("aaaa", "d\x1b]0;x\x07ir", "working 5m", queueOf("q\x1b[31m\u202e1"))
	s.Objective = textsafe.Clean("o\x1b[0m\u009b", 0)
	for _, l := range Frame(snapshot.Snapshot{Sessions: []snapshot.Session{s}}, Opts{Verbose: true, Items: 5, Cols: 100}) {
		for _, r := range l {
			if r < 0x20 || (r >= 0x7f && r < 0xa0) || r == 0x202e {
				t.Errorf("line %q holds %U", l, r)
			}
		}
	}
}

func TestOptsCap(t *testing.T) {
	if (Opts{Verbose: true, Items: 7}).Cap() != 7 || (Opts{Verbose: true, Items: 0}).Cap() != 1 || (Opts{Items: 7}).Cap() != 1 {
		t.Error("Cap is wrong")
	}
}
