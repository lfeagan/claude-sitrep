package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

var t0 = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)

func TestParse(t *testing.T) {
	for in, want := range map[string]Version{"v0.5.0": {0, 5, 0}, "1.20.300": {1, 20, 300}, "v10.0.1": {10, 0, 1}} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "bash-final", "v1.2", "v1.2.3.4", "v1..3", "v1.2.x", "v-1.2.3", "v1.2.3-rc1", "v1.2.1234567890", "v1.2.3\x1b[31m", " v1.2.3", "V1.2.3", "v+1.2.3"} {
		if v, ok := Parse(in); ok || v != (Version{}) {
			t.Errorf("Parse(%q) = %v, %v", in, v, ok)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.5.1", "0.5.0", true}, {"0.10.0", "0.9.9", true}, {"1.0.0", "0.99.99", true}, {"0.5.0", "0.5.0", false}, {"0.4.9", "0.5.0", false}} {
		a, _ := Parse(c.a)
		b, _ := Parse(c.b)
		if a.Newer(b) != c.want {
			t.Errorf("%s newer than %s: want %v", c.a, c.b, c.want)
		}
	}
}

func TestNewest(t *testing.T) {
	out := "aaa\trefs/tags/bash-final\nbbb\trefs/tags/v0.3.0\nccc\trefs/tags/v0.10.0\nddd\trefs/tags/v0.9.0\neee\trefs/tags/v9.9.9-evil\nfff\trefs/heads/main\ngarbage\n"
	if v, ok := Newest([]byte(out)); !ok || v.String() != "0.10.0" {
		t.Errorf("Newest = %v, %v", v, ok)
	}
	if _, ok := Newest([]byte("aaa\trefs/tags/bash-final\n")); ok {
		t.Error("a repository with no release tags has a newest release")
	}
	if _, ok := Newest(nil); ok {
		t.Error("no output has a newest release")
	}
}

func TestClaim(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	if !Claim(st, t0) {
		t.Fatal("the first check is not due")
	}
	if Claim(st, t0.Add(time.Hour)) {
		t.Error("a second claim within the interval")
	}
	if !Claim(st, t0.Add(Interval+time.Second)) {
		t.Error("no claim after the interval")
	}
	if Claim(st, t0.Add(Interval+2*time.Second)) {
		t.Error("claimed twice")
	}
	if !Claim(st, t0.Add(Interval+2*time.Second).Add(-3*Interval)) {
		t.Error("a record from the future (the clock moved back) is as good as none")
	}
}

func TestClaimKeepsWhatIsKnown(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	if err := write(st, Record{Checked: t0.Unix(), Latest: Version{0, 6, 0}}); err != nil {
		t.Fatal(err)
	}
	if !Claim(st, t0.Add(Interval+time.Second)) {
		t.Fatal("not due")
	}
	if r := Read(st); r.Latest != (Version{0, 6, 0}) || r.Checked != t0.Add(Interval+time.Second).Unix() {
		t.Errorf("record %+v", r)
	}
}

func fake(out string, err error) *runner.Fake {
	return &runner.Fake{Answers: map[string]runner.Result{
		runner.Key("git", "ls-remote", "--tags", "--refs", Repo): {Out: []byte(out), Err: err},
	}}
}

func TestCheck(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	f := fake("x\trefs/tags/v0.5.1\ny\trefs/tags/v0.5.0\n", nil)
	Check(context.Background(), f, st, t0, Repo, []string{"GIT_TERMINAL_PROMPT=0"})
	if r := Read(st); r.Latest != (Version{0, 5, 1}) || r.Checked != t0.Unix() {
		t.Errorf("record %+v", r)
	}
	c := f.Calls()
	if len(c) != 1 || !c[0].NewGroup || len(c[0].Env) != 2 || c[0].Dir != st.Dir {
		t.Errorf("calls %+v", c)
	}
}

func TestCheckFailureKeepsTheRecord(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	_ = write(st, Record{Checked: 5, Latest: Version{0, 5, 1}})
	Check(context.Background(), fake("", errors.New("no network")), st, t0, Repo, nil)
	Check(context.Background(), fake("x\trefs/tags/bash-final\n", nil), st, t0, Repo, nil)
	if r := Read(st); r != (Record{Checked: 5, Latest: Version{0, 5, 1}}) {
		t.Errorf("record %+v", r)
	}
}

func TestReadIgnoresBadFiles(t *testing.T) {
	for _, body := range []string{"", "garbage\n", "1\n", "1 v9.9.9-evil\n", "1 0.5.1 extra\n", "1 \x1b[31m\n"} {
		st := state.Store{Dir: t.TempDir()}
		if err := os.WriteFile(filepath.Join(st.Dir, file), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := Read(st); r.Latest != (Version{}) {
			t.Errorf("%q gave %+v", body, r)
		}
	}
	if r := Read(state.Store{Dir: t.TempDir()}); r != (Record{}) {
		t.Errorf("missing file gave %+v", r)
	}
}

func TestNotice(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	if n := Notice(st, "0.5.0"); n != "" {
		t.Errorf("notice without a check: %q", n)
	}
	_ = write(st, Record{Checked: 1, Latest: Version{0, 5, 1}})
	if n := Notice(st, "0.5.0"); !strings.Contains(n, "0.5.1") || !strings.Contains(n, "you have 0.5.0") || strings.ContainsAny(n, "\x1b\n") {
		t.Errorf("notice %q", n)
	}
	for _, cur := range []string{"0.5.1", "0.6.0", "dev", ""} {
		if n := Notice(st, cur); n != "" {
			t.Errorf("notice for %q: %q", cur, n)
		}
	}
}

func TestRecordIsOwnerOnly(t *testing.T) {
	st := state.Store{Dir: t.TempDir()}
	if err := write(st, Record{Checked: 1}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(st.Dir, file)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, %v", fi, err)
	}
}

func TestTheCheckDoesNotRunInTheUsersProject(t *testing.T) {
	st := state.Store{Dir: filepath.Join(t.TempDir(), "sitrep")}
	os.MkdirAll(st.Dir, 0o700)
	f := fake("x\trefs/tags/v0.5.1\n", nil)
	Check(context.Background(), f, st, t0, Repo, []string{"GIT_TERMINAL_PROMPT=0"})
	c := f.Calls()[0]
	if c.Dir != st.Dir {
		t.Errorf("git ran in %q, not in the state folder", c.Dir)
	}
	found := false
	for _, e := range c.Env {
		if e == "GIT_CEILING_DIRECTORIES="+filepath.Dir(st.Dir) {
			found = true
		}
	}
	if !found || !strings.Contains(strings.Join(c.Env, " "), "GIT_TERMINAL_PROMPT=0") {
		t.Errorf("the environment keeps the caller's and adds a ceiling for the search for a repository: %v", c.Env)
	}
}
