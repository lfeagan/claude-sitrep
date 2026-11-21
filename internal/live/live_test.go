package live

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

func TestAge(t *testing.T) {
	for secs, want := range map[int64]string{-5: "<1m", 0: "<1m", 59: "<1m", 60: "1m", 3599: "59m", 3600: "1h", 86399: "23h", 86400: "1d", 200000: "2d"} {
		if got := Age(secs); got != want {
			t.Errorf("Age(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestLabel(t *testing.T) {
	for _, c := range []struct {
		word            string
		changed, now, q int64
		hasQ            bool
		want            string
	}{
		{"-", 0, 100, 0, false, "-"},
		{"waiting", 0, 300, 0, false, "waiting 5m"},
		{"working", 0, 300, 10, true, "working 5m"},
		{"working", 0, 300, 601, true, "idle? 10m"},
		{"working", 0, 300, 600, true, "working 5m"},
		{"working", 0, 300, 9999, false, "working 5m"},
		{"waiting", 0, 300, 9999, true, "waiting 5m"}, // only a working session can be idle
		{"ended", 0, 7200, 0, false, "ended 2h"},
	} {
		if got := Label(c.word, c.changed, c.now, c.q, c.hasQ); got != c.want {
			t.Errorf("Label(%+v) = %q, want %q", c, got, c.want)
		}
	}
}

func TestDead(t *testing.T) {
	reg := Set{OK: true, Entries: []Entry{{ID: "live1", PID: 5}}}
	for _, c := range []struct {
		set     Set
		id, w   string
		changed int64
		want    bool
	}{
		{reg, "live1", "ended", 0, false},   // a live process has this id, whatever the file says
		{reg, "gone1", "ended", 1000, true}, // not registered and said so
		{reg, "gone1", "working", 1000 - Grace - 1, true},
		{reg, "gone1", "working", 1000 - Grace, false}, // inside the grace period
		{Set{}, "gone1", "working", 0, false},          // no registry: only its own word counts
		{Set{}, "gone1", "ended", 1000, true},
	} {
		if got := Dead(c.set, c.id, c.w, c.changed, 1000); got != c.want {
			t.Errorf("Dead(%+v) = %v, want %v", c, got, c.want)
		}
	}
}

func TestWordAllowList(t *testing.T) {
	s := state.Open(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	s.EnsureSession("s1")
	for body, want := range map[string]string{"working\n": "working", "waiting": "waiting", "ended\nextra\n": "ended", "\x1b[31mworking\n": "-", "bogus\n": "-", "": "-"} {
		if err := os.WriteFile(s.Path("s1", "state"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := Word(s, "s1"); got != want {
			t.Errorf("Word(%q) = %q, want %q", body, got, want)
		}
	}
	if got := Word(s, "missing"); got != "-" {
		t.Errorf("missing: %q", got)
	}
}

func TestChanged(t *testing.T) {
	s := state.Open(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	s.EnsureSession("s1")
	notes := s.Path("s1", "notes")
	if err := os.WriteFile(notes, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1_700_000_000, 0)
	os.Chtimes(notes, at, at)
	if got, ok := Changed(s, "s1"); !ok || got != at.Unix() {
		t.Errorf("notes time: %d, %v", got, ok)
	}
	later := time.Unix(1_700_000_500, 0)
	os.WriteFile(s.Path("s1", "state"), []byte("working\n"), 0o600)
	os.Chtimes(s.Path("s1", "state"), later, later)
	if got, _ := Changed(s, "s1"); got != later.Unix() {
		t.Errorf("state file time: %d", got)
	}
	if _, ok := Changed(s, "none"); ok {
		t.Error("a session with no files has a time")
	}
}

func TestQuiet(t *testing.T) {
	cfg := t.TempDir()
	for _, p := range []string{"a", "b"} {
		os.MkdirAll(filepath.Join(cfg, "projects", p), 0o755)
	}
	set := func(path string, ago int64) {
		os.WriteFile(path, []byte("{}"), 0o600)
		at := time.Unix(10_000-ago, 0)
		os.Chtimes(path, at, at)
	}
	set(filepath.Join(cfg, "projects", "a", "s1.jsonl"), 500)
	set(filepath.Join(cfg, "projects", "b", "s1.jsonl"), 90)
	if got, ok := Quiet(cfg, "s1", 10_000); !ok || got != 90 {
		t.Errorf("the shortest counts: %d, %v", got, ok)
	}
	if _, ok := Quiet(cfg, "other", 10_000); ok {
		t.Error("no transcript, but a quiet time")
	}
	// a link is not a transcript
	set(filepath.Join(cfg, "real.jsonl"), 5)
	os.Symlink(filepath.Join(cfg, "real.jsonl"), filepath.Join(cfg, "projects", "a", "s2.jsonl"))
	if _, ok := Quiet(cfg, "s2", 10_000); ok {
		t.Error("a symlinked transcript counted")
	}
}

func writeReg(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRegistry(t *testing.T) {
	dir := t.TempDir()
	writeReg(t, dir, "1.json", `{"pid":11,"sessionId":"alive111","procStart":"Mon Nov 20 10:00:00 2026"}`)
	writeReg(t, dir, "2.json", `{"pid":22,"sessionId":"dead2222","procStart":"x"}`)
	writeReg(t, dir, "3.json", `{"pid":33,"sessionId":"reused33","procStart":"Mon Nov 20 10:00:00 2026"}`)
	writeReg(t, dir, "4.json", `{"pid":44,"sessionId":"foreign4","procStart":"y","pidDomain":"elsewhere"}`)
	writeReg(t, dir, "5.json", `{"pid":"55","sessionId":"strpid55","procStart":""}`)
	writeReg(t, dir, "6.json", `{"pid":66,"sessionId":"../evil","procStart":"x"}`)
	writeReg(t, dir, "7.json", `{"pid":"1; rm","sessionId":"badpid77","procStart":"x"}`)
	writeReg(t, dir, "8.json", `{not json`)
	writeReg(t, dir, "9.json", `{"pid":99,"sessionId":"pserr999","procStart":"z"}`)
	writeReg(t, dir, "10.key", `secret`)
	ps := func(pid string, out string, err error) (string, runner.Result) {
		return "ps -o lstart= -p " + pid, runner.Result{Out: []byte(out), Err: err}
	}
	answers := map[string]runner.Result{}
	for _, a := range [][3]any{{"11", "Mon Nov 20  10:00:00 2026\n", nil}, {"33", "Tue Jan  1 00:00:00 2030\n", nil}, {"55", "", nil}, {"99", "", errors.New("ps failed")}} {
		k, r := ps(a[0].(string), a[1].(string), nil)
		if a[2] != nil {
			r.Err = a[2].(error)
		}
		answers[k] = r
	}
	r := Registry{Dir: dir, Run: &runner.Fake{Answers: answers}, Domain: "darwin", Alive: func(pid int) bool { return pid != 22 }}
	set := r.Read(context.Background())
	got := map[string]int{}
	for _, e := range set.Entries {
		got[string(e.ID)] = e.PID
	}
	want := map[string]int{"alive111": 11, "foreign4": 0, "strpid55": 55, "pserr999": 99}
	if !set.OK || len(got) != len(want) {
		t.Fatalf("got %v (ok %v), want %v", got, set.OK, want)
	}
	for id, pid := range want {
		if g, ok := got[id]; !ok || g != pid {
			t.Errorf("%s: got %d, %v; want %d", id, g, ok, pid)
		}
	}
	if pid, ok := set.PID("foreign4"); ok || pid != 0 {
		t.Errorf("a process in another pid space has no pid here: %d, %v", pid, ok)
	}
	if !set.Has("foreign4") || set.Has("dead2222") {
		t.Error("Has is wrong")
	}
}

func TestRegistryEmptyOrAbsent(t *testing.T) {
	r := Registry{Dir: filepath.Join(t.TempDir(), "missing"), Run: &runner.Fake{}, Alive: func(int) bool { return true }}
	if set := r.Read(context.Background()); set.OK || len(set.Entries) != 0 {
		t.Errorf("absent folder: %+v", set)
	}
	dir := t.TempDir()
	writeReg(t, dir, "1.json", `{"pid":1,"sessionId":"dead1111","procStart":""}`)
	r = Registry{Dir: dir, Run: &runner.Fake{}, Alive: func(int) bool { return false }}
	if set := r.Read(context.Background()); set.OK {
		t.Errorf("nothing live is not OK: %+v", set)
	}
}

func TestFoldersWithGlobCharactersAreRead(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "cfg[1]*?")
	writeReg(t, filepath.Join(cfg, "sessions"), "1.json", `{"pid":7,"sessionId":"glob0001","procStart":""}`)
	r := Registry{Dir: filepath.Join(cfg, "sessions"), Run: &runner.Fake{}, Alive: func(int) bool { return true }}
	if set := r.Read(context.Background()); !set.OK || !set.Has("glob0001") {
		t.Errorf("the registry was not read: %+v", set)
	}
	tr := filepath.Join(cfg, "projects", "p[x]", "glob0001.jsonl")
	os.MkdirAll(filepath.Dir(tr), 0o755)
	os.WriteFile(tr, []byte("{}"), 0o600)
	at := time.Unix(9_000, 0)
	os.Chtimes(tr, at, at)
	if got, ok := Quiet(cfg, "glob0001", 10_000); !ok || got != 1000 {
		t.Errorf("Quiet = %d, %v", got, ok)
	}
}

func TestStateWordTrimsLikeRead(t *testing.T) {
	s := state.Open(t.TempDir())
	s.Ensure()
	s.EnsureSession("s1")
	for body, want := range map[string]string{" working \t\n": "working", "\twaiting": "waiting"} {
		os.WriteFile(s.Path("s1", "state"), []byte(body), 0o600)
		if got := Word(s, "s1"); got != want {
			t.Errorf("Word(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestChangedNeedsRegularFiles(t *testing.T) {
	s := state.Open(t.TempDir())
	s.Ensure()
	s.EnsureSession("s1")
	os.WriteFile(s.Path("s1", "notes"), nil, 0o600)
	at := time.Unix(1_700_000_000, 0)
	os.Chtimes(s.Path("s1", "notes"), at, at)
	os.Mkdir(s.Path("s1", "state"), 0o700) // a directory where the state file should be: fall back to the notes
	if got, ok := Changed(s, "s1"); !ok || got != at.Unix() {
		t.Errorf("Changed = %d, %v", got, ok)
	}
}
