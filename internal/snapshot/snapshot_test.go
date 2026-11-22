package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statetest"
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
	statetest.PutAt(w.t, w.s, name, body, now.Add(-age))
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
	// in the new layout: a folder without notes, a folder whose notes is a folder, a file where a session folder belongs, and a link
	os.MkdirAll(w.s.SessionDir("nonotes1"), 0o700)
	os.MkdirAll(w.s.Path("dirnotes", "notes"), 0o700)
	os.WriteFile(filepath.Join(w.s.SessionsDir(), "filefile"), []byte("x"), 0o600)
	elsewhere := t.TempDir()
	os.WriteFile(filepath.Join(elsewhere, "notes"), []byte("Objective: x\n"), 0o600)
	os.Symlink(elsewhere, filepath.Join(w.s.SessionsDir(), "linked01"))
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

func TestHostilePipesDoNotHangTheListing(t *testing.T) {
	w := newWorld(t)
	w.session("pipe0001", "working", time.Hour)
	w.register("pipe0001", 1)
	for _, name := range []string{"pipe0001.queue", "pipe0001.cwd", "inbox.queue", ".pipe0001.snooze"} {
		if err := syscall.Mkfifo(statetest.Path(w.s, name), 0o600); err != nil {
			t.Skip("no mkfifo")
		}
	}
	// a pipe where the state file or the notes should be
	w.session("pipe0002", "", time.Hour)
	os.Remove(statetest.Path(w.s, ".pipe0002.state"))
	syscall.Mkfifo(statetest.Path(w.s, ".pipe0002.state"), 0o600)
	done := make(chan Snapshot)
	go func() { done <- w.build(Options{All: true, Cap: 5}) }()
	select {
	case snap := <-done:
		if len(snap.Sessions) == 0 || snap.Inbox.Count != 0 {
			t.Errorf("%d sessions, inbox %d", len(snap.Sessions), snap.Inbox.Count)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the listing hung on a pipe")
	}
}

// block writes the record the permission hook leaves, at seconds before now, and a transcript last written at mtime seconds before now
// (none when mtime is 0).
func (w *world) block(id string, at, mtime time.Duration) {
	w.t.Helper()
	if err := w.s.EnsureSession(state.ID(id)); err != nil {
		w.t.Fatal(err)
	}
	body := strconv.FormatInt(now.Add(-at).Unix(), 10) + " permission\n"
	if err := os.WriteFile(w.s.Path(state.ID(id), "blocked"), []byte(body), 0o600); err != nil {
		w.t.Fatal(err)
	}
	if mtime == 0 {
		return
	}
	dir := filepath.Join(w.cfg, "projects", "p")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, id+".jsonl")
	os.WriteFile(p, []byte("{}\n"), 0o600)
	os.Chtimes(p, now.Add(-mtime), now.Add(-mtime))
}

func TestBlockedSessionNeedsInput(t *testing.T) {
	w := newWorld(t)
	w.session("blk00001", "working", 30*time.Second)
	w.register("blk00001", 1)
	w.block("blk00001", 30*time.Second, 29*time.Second) // written one second after the dialog opened: the write that follows it
	s := byID(w.build(Options{Cap: 5}), "blk00001")
	if s.Blocked != "permission" || !s.NeedsInput || s.Snoozed || s.State != "working" || s.StateLabel != "blocked <1m" {
		t.Fatalf("blocked session: %+v", s)
	}

	// the user answered: the transcript moved on, so the session is working again
	w.block("blk00001", 30*time.Second, 5*time.Second)
	if s := byID(w.build(Options{Cap: 5}), "blk00001"); s.Blocked != "" || s.NeedsInput || !strings.HasPrefix(s.StateLabel, "working") {
		t.Fatalf("answered dialog: %+v", s)
	}

	// no transcript to judge by: the block stands until its file is removed
	w.block("blk00001", 3*time.Minute, 0)
	os.RemoveAll(filepath.Join(w.cfg, "projects"))
	if s := byID(w.build(Options{Cap: 5}), "blk00001"); s.Blocked != "permission" || s.StateLabel != "blocked 3m" {
		t.Fatalf("no transcript: %+v", s)
	}
}

func TestBlockedIsIgnoredWhenItDoesNotApply(t *testing.T) {
	w := newWorld(t)
	w.session("wait0001", "waiting", 2*time.Minute)
	w.register("wait0001", 1)
	w.block("wait0001", 30*time.Second, 0)
	if s := byID(w.build(Options{Cap: 5}), "wait0001"); s.Blocked != "" || !s.NeedsInput || s.StateLabel != "waiting 2m" {
		t.Errorf("a waiting session is not blocked: %+v", s)
	}
	w.session("bad00001", "working", 30*time.Second)
	w.register("bad00001", 1)
	if err := w.s.EnsureSession("bad00001"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", "abc permission\n", "1800000000 other\n", "1800000000\n", "-5 permission\n", "0 permission\n", "1800000000 permission extra\n", "\x1b[31m 1800000000 permission\n"} {
		os.WriteFile(w.s.Path("bad00001", "blocked"), []byte(body), 0o600)
		if s := byID(w.build(Options{Cap: 5}), "bad00001"); s.Blocked != "" || s.NeedsInput {
			t.Errorf("record %q gave %+v", body, s)
		}
	}
}

func TestBlockedSnoozeKeysOnTheBlock(t *testing.T) {
	w := newWorld(t)
	w.session("blk00001", "working", 30*time.Second)
	w.register("blk00001", 1)
	w.block("blk00001", 20*time.Second, 0)
	at := strconv.FormatInt(now.Add(-20*time.Second).Unix(), 10)
	os.WriteFile(w.s.Path("blk00001", "snooze"), []byte(at+"\n"), 0o600)
	if s := byID(w.build(Options{Cap: 5}), "blk00001"); s.NeedsInput || !s.Snoozed || s.Blocked != "permission" {
		t.Errorf("snoozed block: %+v", s)
	}
	os.WriteFile(w.s.Path("blk00001", "snooze"), []byte("1799999000\n"), 0o600)
	if s := byID(w.build(Options{Cap: 5}), "blk00001"); !s.NeedsInput || s.Snoozed {
		t.Errorf("a snooze from before the block must not mute it: %+v", s)
	}
}

func TestBlockedIsLeftOutOfTheJSONWhenEmpty(t *testing.T) {
	w := newWorld(t)
	w.session("fresh001", "working", 30*time.Second)
	w.register("fresh001", 1)
	var buf strings.Builder
	if err := Encode(&buf, w.build(Options{Cap: 5})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"blocked"`) {
		t.Errorf("the field must be absent unless set: %s", buf.String())
	}
	w.block("fresh001", 10*time.Second, 0)
	buf.Reset()
	if err := Encode(&buf, w.build(Options{Cap: 5})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"blocked":"permission"`) && !strings.Contains(buf.String(), `"blocked": "permission"`) {
		t.Errorf("the field is missing: %s", buf.String())
	}
}

func (w *world) buildEnv(env map[string]string) Snapshot {
	w.t.Helper()
	b := Builder{
		Store: w.s, ConfigDir: w.cfg, Clock: clock.Fixed(now),
		Conf: conf.Conf{File: filepath.Join(w.s.Dir, "sitrep.conf"), Getenv: func(n string) string { return env[n] }},
		Reg:  live.Registry{Dir: filepath.Join(w.cfg, "sessions"), Run: &runner.Fake{}, Alive: func(int) bool { return true }},
	}
	return b.Build(context.Background(), Options{Cap: 5})
}

func TestNeedsInputFadesAfterTheSetTime(t *testing.T) {
	w := newWorld(t)
	for id, age := range map[string]time.Duration{"new00001": 9 * time.Minute, "edge0001": 10 * time.Minute, "old00001": 10*time.Minute + time.Second} {
		w.session(id, "waiting", age)
		w.register(id, 1)
	}
	w.session("work0001", "working", 3*time.Hour)
	w.register("work0001", 1)
	snap := w.buildEnv(nil)
	for id, want := range map[string]bool{"new00001": false, "edge0001": false, "old00001": true, "work0001": false} {
		if got := byID(snap, id).AttnFaded; got != want {
			t.Errorf("%s faded = %v, want %v", id, got, want)
		}
	}
	if s := byID(snap, "old00001"); !s.NeedsInput {
		t.Error("a faded session still needs input")
	}
	if s := byID(w.buildEnv(map[string]string{"SITREP_ATTN_FADE": "0"}), "old00001"); s.AttnFaded {
		t.Error("0 never fades")
	}
	if s := byID(w.buildEnv(map[string]string{"SITREP_ATTN_FADE": "5"}), "new00001"); !s.AttnFaded {
		t.Error("a shorter time fades sooner")
	}
	if s := byID(w.buildEnv(map[string]string{"SITREP_ATTN_FADE": "soon"}), "old00001"); !s.AttnFaded {
		t.Error("a bad value is the default")
	}
}

func TestASnoozedOrAnsweredSessionIsNotFaded(t *testing.T) {
	w := newWorld(t)
	w.session("snz00001", "waiting", time.Hour)
	w.register("snz00001", 1)
	w.file(".snz00001.snooze", strconv.FormatInt(now.Add(-time.Hour).Unix(), 10)+"\n", 0)
	if s := byID(w.buildEnv(nil), "snz00001"); !s.Snoozed || s.AttnFaded {
		t.Errorf("a snoozed session does not need input, so nothing fades: %+v", s)
	}
}

func TestABlockFadesFromWhenTheDialogOpened(t *testing.T) {
	w := newWorld(t)
	w.session("blk00001", "working", 2*time.Hour) // started working long ago, blocked just now
	w.register("blk00001", 1)
	w.block("blk00001", 30*time.Second, 29*time.Second)
	if s := byID(w.buildEnv(nil), "blk00001"); !s.NeedsInput || s.AttnFaded {
		t.Errorf("a new block is not faded: %+v", s)
	}
	w.block("blk00001", 11*time.Minute, 0)
	os.RemoveAll(filepath.Join(w.cfg, "projects"))
	if s := byID(w.buildEnv(nil), "blk00001"); !s.NeedsInput || !s.AttnFaded {
		t.Errorf("an old block fades: %+v", s)
	}
}

func TestAttnFadedIsLeftOutOfTheJSONUnlessSet(t *testing.T) {
	w := newWorld(t)
	w.session("wait0001", "waiting", time.Minute)
	w.register("wait0001", 1)
	var buf strings.Builder
	if err := Encode(&buf, w.build(Options{Cap: 5})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "attn_faded") {
		t.Errorf("the field must be absent unless set: %s", buf.String())
	}
}

func TestLinkedStateFilesAreNotFollowed(t *testing.T) {
	w := newWorld(t)
	w.session("link0001", "waiting", time.Minute)
	w.register("link0001", 1)
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("/secret/place\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"queue", "cwd"} {
		p := w.s.Path("link0001", name)
		os.Remove(p)
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
	}
	s := byID(w.build(Options{Cap: 5}), "link0001")
	if s.Queue.Count != 0 || s.Dir.String() != "?" || s.Cwd.String() != "" {
		t.Errorf("a linked queue and cwd read as nothing, never as the file they point at: %+v", s)
	}
	if !s.NeedsInput {
		t.Error("the session is still listed, and still waits")
	}
}

func TestPinnedIsARegularFileNotALink(t *testing.T) {
	w := newWorld(t)
	w.session("pin00001", "working", time.Minute)
	w.register("pin00001", 1)
	target := filepath.Join(t.TempDir(), "elsewhere")
	os.WriteFile(target, []byte("x"), 0o600)
	if err := os.Symlink(target, w.s.Path("pin00001", "pinned")); err != nil {
		t.Fatal(err)
	}
	if byID(w.build(Options{Cap: 5}), "pin00001").Pinned {
		t.Error("a link named pinned must not pin the session")
	}
	os.Remove(w.s.Path("pin00001", "pinned"))
	os.WriteFile(w.s.Path("pin00001", "pinned"), []byte("x"), 0o600)
	if !byID(w.build(Options{Cap: 5}), "pin00001").Pinned {
		t.Error("a regular pinned file pins it")
	}
}

func TestThemeSettingsAreReadAgainWithEverySnapshot(t *testing.T) {
	w := newWorld(t)
	w.session("live0001", "working", time.Minute)
	w.register("live0001", 1)
	if got := w.build(Options{Cap: 5}).Settings; got.Theme != "" || got.Colors != "" {
		t.Fatalf("nothing is set: %+v", got)
	}
	os.WriteFile(filepath.Join(w.s.Dir, "sitrep.conf"), []byte("SITREP_THEME=pastel\nSITREP_COLORS='attn=#ff0000'\n"), 0o600)
	if got := w.build(Options{Cap: 5}).Settings; got.Theme != "pastel" || got.Colors != "attn=#ff0000" {
		t.Errorf("a running dashboard would not see a saved change: %+v", got)
	}
}

func TestALongNoteEndsInAnEllipsis(t *testing.T) {
	w := newWorld(t)
	w.file("l0000001.txt", "Objective: "+strings.Repeat("word ", 100)+"\nProgress:  short\nNow:       n\n", time.Hour)
	w.file("l0000001.state", "working\n", time.Hour)
	w.register("l0000001", 1)
	x := byID(w.build(Options{Cap: -1}), "l0000001")
	if got := x.Objective.String(); !strings.HasSuffix(got, "…") {
		t.Errorf("a cut Objective says so: %q", got)
	}
	if got := x.Progress.String(); got != "short" {
		t.Errorf("a short one is as it was: %q", got)
	}
}
