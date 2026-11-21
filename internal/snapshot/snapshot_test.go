package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

var now = time.Unix(1_800_000_000, 0)

type world struct {
	t   *testing.T
	cfg string
	s   state.Store
}

func newWorld(t *testing.T) *world {
	t.Helper()
	home := t.TempDir()
	w := &world{t: t, cfg: filepath.Join(home, ".claude"), s: state.Open(home)}
	if err := w.s.Ensure(); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *world) file(name, body string, age time.Duration) {
	w.t.Helper()
	p := filepath.Join(w.s.Dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		w.t.Fatal(err)
	}
	at := now.Add(-age)
	if err := os.Chtimes(p, at, at); err != nil {
		w.t.Fatal(err)
	}
}

// session writes the notes and a state word, both aged.
func (w *world) session(id, word string, age time.Duration) {
	w.t.Helper()
	w.file(id+".txt", "Objective: obj "+id+"\nProgress:  p\nNow:       n\n", age)
	if word != "" {
		w.file("."+id+".state", word+"\n", age)
	}
}

func (w *world) register(id string, pid int) {
	w.t.Helper()
	dir := filepath.Join(w.cfg, "sessions")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{"pid":`+strings.TrimSpace(string(rune('0'+pid%10)))+`,"sessionId":"`+id+`"}`), 0o600)
}

func (w *world) build(opt Options) Snapshot {
	w.t.Helper()
	b := Builder{
		Store: w.s, ConfigDir: w.cfg, Clock: clock.Fixed(now),
		Conf: conf.Conf{File: filepath.Join(w.s.Dir, "sitrep.conf"), Getenv: func(string) string { return "" }},
		Reg:  live.Registry{Dir: filepath.Join(w.cfg, "sessions"), Run: &runner.Fake{}, Alive: func(int) bool { return true }},
	}
	return b.Build(context.Background(), opt)
}

func byID(s Snapshot, id string) *Session {
	for i := range s.Sessions {
		if string(s.Sessions[i].ID) == id {
			return &s.Sessions[i]
		}
	}
	return nil
}

func TestRecentActivityAndNeedsInput(t *testing.T) {
	w := newWorld(t)
	w.session("fresh001", "working", 30*time.Second)
	w.session("edge0001", "working", 60*time.Second)
	w.session("older001", "working", 61*time.Second)
	w.session("wait0001", "waiting", 5*time.Minute)
	for _, id := range []string{"fresh001", "edge0001", "older001", "wait0001"} {
		w.register(id, 1)
	}
	snap := w.build(Options{Cap: 5})
	for id, hl := range map[string]bool{"fresh001": true, "edge0001": true, "older001": false, "wait0001": false} {
		if got := byID(snap, id).Highlight; got != hl {
			t.Errorf("%s highlight = %v, want %v", id, got, hl)
		}
	}
	if s := byID(snap, "wait0001"); !s.NeedsInput || s.Snoozed || s.StateLabel != "waiting 5m" {
		t.Errorf("waiting session: %+v", s)
	}
	if byID(snap, "fresh001").NeedsInput {
		t.Error("a working session needs no input")
	}
	// snooze stores the time the state changed; once the state changes again the mute is over
	w.file(".wait0001.snooze", "1799999700\n", 0)
	if s := byID(w.build(Options{Cap: 5}), "wait0001"); s.NeedsInput || !s.Snoozed {
		t.Errorf("snoozed session: %+v", s)
	}
	w.file(".wait0001.snooze", "1799999000\n", 0)
	if s := byID(w.build(Options{Cap: 5}), "wait0001"); !s.NeedsInput || s.Snoozed {
		t.Errorf("a stale snooze must not mute: %+v", s)
	}
}

func TestEndedHiddenAndStranded(t *testing.T) {
	w := newWorld(t)
	w.session("live0001", "working", time.Hour)
	w.register("live0001", 1)
	w.session("done0001", "ended", 2*time.Hour)
	w.file("done0001.queue", "a\nb\n", 2*time.Hour)
	w.session("gone0001", "working", 3*time.Hour)
	w.file("gone0001.queue", "c\n", 3*time.Hour)
	w.session("ancient1", "working", 30*time.Hour) // outside the window
	snap := w.build(Options{Cap: 5})
	if len(snap.Sessions) != 1 || snap.Hidden.Ended != 2 || snap.Hidden.StrandedItems != 3 || snap.Registry != "ok" {
		t.Errorf("sessions %d, hidden %+v, registry %s", len(snap.Sessions), snap.Hidden, snap.Registry)
	}
	all := w.build(Options{All: true, Cap: 5})
	if len(all.Sessions) != 3 || all.Hidden.Ended != 0 {
		t.Fatalf("with all: %d sessions, hidden %+v", len(all.Sessions), all.Hidden)
	}
	if s := byID(all, "done0001"); s.Running || s.State != "ended" || s.StateLabel != "ended 2h" || s.NeedsInput || s.PID != nil {
		t.Errorf("ended session: %+v", s)
	}
	// newest first
	var order []string
	for _, s := range all.Sessions {
		order = append(order, string(s.ID))
	}
	if strings.Join(order, ",") != "live0001,done0001,gone0001" {
		t.Errorf("order %v", order)
	}
}

func TestStaleEndedWordWhileAlive(t *testing.T) {
	w := newWorld(t)
	w.session("back0001", "ended", 10*time.Minute) // resumed: the process is alive though the file says ended
	w.register("back0001", 1)
	s := byID(w.build(Options{Cap: 5}), "back0001")
	if s == nil || !s.Running || s.State != "running" || s.StateLabel != "running 10m" {
		t.Errorf("%+v", s)
	}
}

func TestNoRegistry(t *testing.T) {
	w := newWorld(t)
	w.session("a0000001", "working", 10*time.Minute)
	w.session("b0000002", "ended", 10*time.Minute)
	snap := w.build(Options{Cap: 5})
	if snap.Registry != "unavailable" || len(snap.Sessions) != 1 || snap.Hidden.Ended != 1 {
		t.Errorf("%s, %d sessions, hidden %+v", snap.Registry, len(snap.Sessions), snap.Hidden)
	}
}

func TestQueueCaps(t *testing.T) {
	w := newWorld(t)
	w.session("q0000001", "working", time.Hour)
	w.register("q0000001", 1)
	w.file("q0000001.queue", "a\n\n   \nb\nc\n"+strings.Repeat("x", 300)+"\n", time.Hour)
	for cap, want := range map[int]int{-1: 4, 0: 0, 2: 2, 99: 4} {
		q := byID(w.build(Options{Cap: cap}), "q0000001").Queue
		if q.Count != 4 || len(q.Items) != want || q.Items == nil {
			t.Errorf("cap %d: count %d, %d items", cap, q.Count, len(q.Items))
		}
	}
	items := byID(w.build(Options{Cap: -1}), "q0000001").Queue.Items
	if len(items[3].String()) != 200 {
		t.Errorf("a long item is cut to 200 characters, got %d", len(items[3].String()))
	}
}

func TestIgnoresFilesThatAreNotSessions(t *testing.T) {
	w := newWorld(t)
	w.session("good0001", "working", time.Hour)
	w.register("good0001", 1)
	w.file("bad name.txt", "junk", time.Hour)
	w.file("x.y.txt", "junk", time.Hour)
	w.file("inbox.txt", "junk", time.Hour)
	os.Mkdir(filepath.Join(w.s.Dir, "adir0001.txt"), 0o700)
	if snap := w.build(Options{All: true, Cap: 5}); len(snap.Sessions) != 1 {
		t.Errorf("%d sessions", len(snap.Sessions))
	}
}

func TestEveryStringIsClean(t *testing.T) {
	w := newWorld(t)
	w.file("hos00001.txt", "Objective: a\x1b[31m‮b\xff\xfe\tc\nProgress:  p\x01p\nNow:       \u0085n\n", time.Hour)
	w.file(".hos00001.state", "working\n", time.Hour)
	w.file("hos00001.cwd", "/x/\x1b]0;t\x07dir\n", time.Hour)
	w.file("hos00001.queue", "q\x1b[0m\x9f1\n", time.Hour)
	w.file("inbox.queue", "i‮\n", time.Hour)
	w.register("hos00001", 1)
	var buf bytes.Buffer
	if err := Encode(&buf, w.build(Options{Cap: 5})); err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, r := range x {
				if r < 0x20 || (r >= 0x7f && r < 0xa0) || r == 0x202e || r == 0x2028 {
					t.Errorf("string %q holds %U", x, r)
				}
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	if s := byID(w.build(Options{Cap: 5}), "hos00001"); s.Dir.String() != "]0;tdir" || s.Objective.String() != "a[31mb�� c" {
		t.Errorf("dir %q, objective %q", s.Dir, s.Objective)
	}
}

func TestDocumentOrderAndEncoding(t *testing.T) {
	w := newWorld(t)
	w.session("ord00001", "working", time.Hour)
	w.register("ord00001", 1)
	w.file("ord00001.txt", "Objective: a <b> & c\nProgress:  p\nNow:       n\n", time.Hour)
	var buf bytes.Buffer
	if err := Encode(&buf, w.build(Options{Cap: 5})); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "a <b> & c") {
		t.Error("HTML characters were escaped")
	}
	last := -1
	for _, k := range []string{`"version"`, `"generated"`, `"registry"`, `"settings"`, `"hidden"`, `"inbox"`, `"sessions"`, `"id"`, `"short"`, `"running"`, `"state"`, `"state_label"`, `"age"`, `"state_changed"`, `"quiet_secs"`, `"idle"`, `"needs_input"`, `"snoozed"`, `"pid"`, `"pinned"`, `"highlight"`, `"dir"`, `"cwd"`, `"objective"`, `"progress"`, `"now"`, `"queue"`} {
		i := strings.Index(out, k)
		if i < 0 || i < last {
			t.Fatalf("key %s is missing or out of order", k)
		}
		last = i
	}
}

func TestLabelOK(t *testing.T) {
	for s, want := range map[string]bool{"-": false, "working 5m": true, "idle? 12m": true, "ended <1m": true, "waiting": true, "": false, "Working 5m": false, "x 5mm": false, "x 5m extra": false, "x \x1b[0m": false, "x ": false} {
		if got := labelOK(s); got != want {
			t.Errorf("labelOK(%q) = %v, want %v", s, got, want)
		}
	}
}
