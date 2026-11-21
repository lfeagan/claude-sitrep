package prune

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

const now = 1_800_000_000

func setup(t *testing.T, registered ...string) Pruner {
	t.Helper()
	home := t.TempDir()
	s := state.Open(home)
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, ".claude")
	os.MkdirAll(filepath.Join(cfg, "sessions"), 0o755)
	for i, id := range registered {
		body := `{"pid":` + string(rune('1'+i)) + `,"sessionId":"` + id + `"}`
		os.WriteFile(filepath.Join(cfg, "sessions", id+".json"), []byte(body), 0o600)
	}
	return Pruner{Store: s, ConfigDir: cfg, Reg: live.Registry{Dir: filepath.Join(cfg, "sessions"), Run: &runner.Fake{}, Alive: func(int) bool { return true }}}
}

func session(t *testing.T, p Pruner, id, word, cwd, queue string, age time.Duration) {
	t.Helper()
	at := time.Unix(now, 0).Add(-age)
	write := func(name, body string) {
		path := filepath.Join(p.Store.Dir, name)
		os.WriteFile(path, []byte(body), 0o600)
		os.Chtimes(path, at, at)
	}
	write(id+".txt", "Objective: x\n")
	if word != "" {
		write("."+id+".state", word+"\n")
	}
	if cwd != "" {
		write(id+".cwd", cwd)
	}
	if queue != "" {
		write(id+".queue", queue)
	}
}

func TestPlan(t *testing.T) {
	p := setup(t, "alive001")
	session(t, p, "alive001", "working", "/a/alive", "", time.Hour)
	session(t, p, "done0001", "ended", "/a/b/done\n", "x\ny\n", time.Hour)
	session(t, p, "gone0001", "working", "", "z\n", time.Hour)
	session(t, p, "new00001", "working", "/a/new", "", 30*time.Second) // not registered yet, but inside the grace period
	acts, err := p.Plan(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 2 {
		t.Fatalf("%+v", acts)
	}
	if a := acts[0]; a.ID != "done0001" || a.Dir.String() != "done" || a.Mark || a.Items != 2 {
		t.Errorf("ended session: %+v", a)
	}
	if a := acts[1]; a.ID != "gone0001" || a.Dir.String() != "?" || !a.Mark || a.Items != 1 {
		t.Errorf("gone session: %+v", a)
	}
}

func TestPlanNeedsARegistry(t *testing.T) {
	p := setup(t)
	session(t, p, "gone0001", "working", "", "", time.Hour)
	_, err := p.Plan(context.Background(), now)
	var nr *NoRegistryError
	if !errors.As(err, &nr) || err.Error() != "cannot read the live-session registry in "+filepath.Join(p.ConfigDir, "sessions")+"; nothing changed" {
		t.Errorf("%v", err)
	}
}

func TestMarkEnded(t *testing.T) {
	p := setup(t, "alive001")
	if err := p.MarkEnded("gone0001"); err != nil {
		t.Fatal(err)
	}
	if got := live.Word(p.Store, "gone0001"); got != "ended" {
		t.Errorf("word %q", got)
	}
	if fi, _ := os.Stat(p.Store.Hidden("gone0001", "state")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}
