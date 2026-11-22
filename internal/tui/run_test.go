package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

func tx(s string) textsafe.Text { return textsafe.Clean(s, 0) }

func sample() snapshot.Snapshot {
	pid := 4242
	quiet := int64(30)
	return snapshot.Snapshot{
		Version: 1, Generated: 100, Registry: "ok",
		Settings: snapshot.Settings{QueueItems: 5, HighlightSeconds: 60, HighlightSGR: "48;5;238"},
		Hidden:   snapshot.Hidden{Ended: 1, StrandedItems: 2},
		Inbox:    snapshot.Queue{Count: 1, Items: []textsafe.Text{tx("inbox item")}},
		Sessions: []snapshot.Session{{
			ID: "abcd1234", Short: "abcd", Running: true, State: "working", StateLabel: "working 5m", Age: 300, StateChanged: 70, QuietSecs: &quiet, NeedsInput: true,
			PID: &pid, Pinned: true, Highlight: true, Dir: tx("proj"), Cwd: tx("/w/proj"), Objective: tx("build it"), Progress: tx("half"), Now: tx("compiling"),
			Queue: snapshot.Queue{Count: 2, Items: []textsafe.Text{tx("a"), tx("b")}},
		}},
	}
}

func TestFromSnapshot(t *testing.T) {
	got := FromSnapshot(sample())
	if got.Version != 1 || got.Registry != "ok" || got.Hidden.Ended != 1 || got.Inbox.Count != 1 || got.Inbox.Items[0] != "inbox item" || got.Settings.HighlightSGR != "48;5;238" {
		t.Errorf("%+v", got)
	}
	x := got.Sessions[0]
	if x.ID != "abcd1234" || x.Dir != "proj" || x.Objective != "build it" || x.PID == nil || *x.PID != 4242 || x.QuietSecs == nil || *x.QuietSecs != 30 ||
		!x.NeedsInput || !x.Pinned || !x.Highlight || len(x.Queue.Items) != 2 || x.Queue.Count != 2 {
		t.Errorf("%+v", x)
	}
	// no process: no pid
	s := sample()
	s.Sessions[0].PID = nil
	if FromSnapshot(s).Sessions[0].PID != nil {
		t.Error("a pid appeared")
	}
}

func TestTabByName(t *testing.T) {
	for name, want := range map[string]int{"": tabWatch, "watch": tabWatch, "table": tabTable, "wall": tabWall} {
		if got, err := tabByName(name); err != nil || got != want {
			t.Errorf("%q: %d, %v", name, got, err)
		}
	}
	if _, err := tabByName("nope"); err == nil || !strings.Contains(err.Error(), `unknown tab "nope"`) {
		t.Errorf("%v", err)
	}
	if _, err := tabByName("top"); err == nil || !strings.Contains(err.Error(), "watch, table or wall") {
		t.Errorf("the old name is gone: %v", err)
	}
}

func TestFrame(t *testing.T) {
	eng := Engine{Snapshot: func(context.Context) (snapshot.Snapshot, error) { return sample(), nil }}
	out, err := Frame(context.Background(), eng, "watch", 100, false)
	if err != nil || !strings.Contains(out, "build it") || strings.Contains(out, "\x1b[") || strings.HasSuffix(out, "\n") {
		t.Errorf("%q, %v", out, err)
	}
	if _, err := Frame(context.Background(), eng, "bogus", 100, false); err == nil {
		t.Error("a bad tab was accepted")
	}
	eng.Snapshot = func(context.Context) (snapshot.Snapshot, error) {
		return snapshot.Snapshot{}, errors.New("no registry")
	}
	if _, err := Frame(context.Background(), eng, "table", 100, false); err == nil || err.Error() != "no registry" {
		t.Errorf("%v", err)
	}
}

func TestRoutedSendsOnlySitrepToTheEngine(t *testing.T) {
	var got []string
	eng := Engine{Command: func(_ context.Context, args ...string) (string, error) { got = args; return "answer\n", nil }}
	run := routed(eng)
	if out, err := run(context.Background(), engineName, "snooze", "abcd"); err != nil || out != "answer\n" || strings.Join(got, " ") != "snooze abcd" {
		t.Errorf("%q %v %v", out, err, got)
	}
	got = nil
	if _, err := run(context.Background(), "definitely-not-installed-xyz", "x"); err == nil || got != nil {
		t.Errorf("another program went to the engine or ran: %v %v", err, got)
	}
}

func TestRunNeedsAnEngine(t *testing.T) {
	if err := Run(Options{}, Engine{}); err == nil {
		t.Error("no engine, no error")
	}
	if err := Run(Options{Tab: "bogus"}, Engine{}); err == nil || !strings.Contains(err.Error(), "unknown tab") {
		t.Errorf("%v", err)
	}
}
