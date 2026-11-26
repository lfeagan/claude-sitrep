package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func aliasStore(t *testing.T, ids ...string) Store {
	t.Helper()
	s := Store{Dir: t.TempDir()}
	for _, id := range ids {
		if err := s.WriteFile(s.Path(ID(id), "notes"), []byte("Objective: x\n")); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestValidAlias(t *testing.T) {
	for _, ok := range []string{"a", "sitrep", "mbe-2", "a1", strings.Repeat("a", 32)} {
		if !ValidAlias(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "inbox", "1a", "-a", "A", "Sitrep", "a_b", "a b", "a.b", "../x", "a\x1b[31m", "a\n", "é", "ａ", strings.Repeat("a", 33), "a‮b"} {
		if ValidAlias(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSetAndClearAlias(t *testing.T) {
	s := aliasStore(t, "c5d05701", "dfc08fd7")
	if changed, err := s.SetAlias("c5d05701", "sitrep"); !changed || err != nil {
		t.Fatalf("set: %v, %v", changed, err)
	}
	if s.Alias("c5d05701") != "sitrep" || s.Alias("dfc08fd7") != "" {
		t.Errorf("aliases %v", s.Aliases())
	}
	if changed, err := s.SetAlias("c5d05701", "sitrep"); changed || err != nil {
		t.Errorf("same name twice: %v, %v", changed, err)
	}
	if changed, err := s.SetAlias("c5d05701", "work"); !changed || err != nil || s.Alias("c5d05701") != "work" {
		t.Errorf("replace: %v, %v, %q", changed, err, s.Alias("c5d05701"))
	}
	var taken *AliasTakenError
	if _, err := s.SetAlias("dfc08fd7", "work"); !errors.As(err, &taken) || taken.Holder != "c5d05701" || !strings.Contains(err.Error(), "sitrep alias c5d0 --off") {
		t.Errorf("taken: %v", err)
	}
	if old, err := s.ClearAlias("c5d05701"); old != "work" || err != nil {
		t.Errorf("clear: %q, %v", old, err)
	}
	if old, err := s.ClearAlias("c5d05701"); old != "" || err != nil {
		t.Errorf("clear again: %q, %v", old, err)
	}
	if _, err := s.SetAlias("dfc08fd7", "work"); err != nil {
		t.Errorf("a freed name: %v", err)
	}
}

func TestAliasRefusals(t *testing.T) {
	s := aliasStore(t, "cafe1234", "dfc08fd7")
	var pre *AliasPrefixError
	if _, err := s.SetAlias("dfc08fd7", "cafe"); !errors.As(err, &pre) || pre.ID != "cafe1234" {
		t.Errorf("the start of an id: %v", err)
	}
	for _, bad := range []string{"inbox", "", "Bad", "1x", "a b", "../x"} {
		if _, err := s.SetAlias("dfc08fd7", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := s.SetAlias(Inbox, "work"); err == nil {
		t.Error("the inbox has no alias")
	}
	if _, err := s.ClearAlias(Inbox); err == nil {
		t.Error("the inbox has no alias to clear")
	}
	for _, n := range []string{"off", "add"} { // words of the command line are fine
		if _, err := s.SetAlias("dfc08fd7", n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
}

func TestAliasRace(t *testing.T) {
	ids := []string{"s0000001", "s0000002", "s0000003", "s0000004", "s0000005", "s0000006"}
	s := aliasStore(t, ids...)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := s.SetAlias(ID(id), "work"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	holders := 0
	for _, a := range s.Aliases() {
		if a == "work" {
			holders++
		}
	}
	if wins != 1 || holders != 1 {
		t.Errorf("%d winners, %d holders", wins, holders)
	}
	entries, _ := filepath.Glob(filepath.Join(s.SessionsDir(), "*", ".sitrep.*"))
	if len(entries) != 0 {
		t.Errorf("temporary files left: %v", entries)
	}
	fi, err := os.Stat(s.Path("s0000001", "alias"))
	if err == nil && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode())
	}
}

func TestResolveWithAliases(t *testing.T) {
	s := aliasStore(t, "c5d05701", "c5e00000", "dfc08fd7", "zeta0001")
	s.SetAlias("c5d05701", "sitrep")
	s.SetAlias("dfc08fd7", "mbe")
	for in, want := range map[string]ID{"sitrep": "c5d05701", "mbe": "dfc08fd7", "c5d": "c5d05701", "dfc0": "dfc08fd7", "zeta": "zeta0001", "inbox": Inbox} {
		if got, err := s.Resolve(in); err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"sit", "mb", "sitrepx", "nothing", "c5"} { // a prefix of an alias, no match, and two ids
		if got, err := s.Resolve(in); err == nil {
			t.Errorf("Resolve(%q) = %q", in, got)
		}
	}
	// an id wins over an alias: a hand-written file may hold a name that starts an id, and a later session may start with one
	os.WriteFile(s.Path("dfc08fd7", "alias"), []byte("zeta\n"), 0o600)
	if got, err := s.Resolve("zeta"); err != nil || got != "zeta0001" {
		t.Errorf("an id and an alias: %q, %v", got, err)
	}
	// two holders of one name
	os.WriteFile(s.Path("dfc08fd7", "alias"), []byte("sitrep\n"), 0o600)
	if _, err := s.Resolve("sitrep"); err == nil || !strings.Contains(err.Error(), "held by 2") {
		t.Errorf("two holders: %v", err)
	}
}

func TestHostileAliasFiles(t *testing.T) {
	s := aliasStore(t, "c5d05701")
	path := s.Path("c5d05701", "alias")
	write := func(b string) { os.Remove(path); os.WriteFile(path, []byte(b), 0o600) }
	for name, body := range map[string]string{
		"escape": "ab\x1b[31m\n", "c1": "a\u009bb\n", "bidi": "a‮b\n", "upper": "Work\n", "long": strings.Repeat("a", 65) + "\n",
		"megabyte": strings.Repeat("a", 1<<20), "two lines then junk": "\n", "empty": "", "spaces": " work\n", "inbox": "inbox\n",
	} {
		write(body)
		if got := s.Alias("c5d05701"); got != "" {
			t.Errorf("%s: read %q", name, got)
		}
	}
	write("work\nsecond line\n")
	if got := s.Alias("c5d05701"); got != "work" {
		t.Errorf("first line: %q", got)
	}
	write("work") // no newline
	if got := s.Alias("c5d05701"); got != "work" {
		t.Errorf("no newline: %q", got)
	}
	os.Remove(path)
	target := filepath.Join(t.TempDir(), "elsewhere")
	os.WriteFile(target, []byte("work\n"), 0o600)
	os.Symlink(target, path)
	if got := s.Alias("c5d05701"); got != "" {
		t.Errorf("a link: %q", got)
	}
	os.Remove(path)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.Alias("c5d05701"); got != "" { // must not block
		t.Errorf("a pipe: %q", got)
	}
}

func TestAnAliasFileAloneMakesNoSession(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	os.MkdirAll(s.SessionDir("ghost001"), 0o700)
	os.WriteFile(s.Path("ghost001", "alias"), []byte("work\n"), 0o600)
	if len(s.Sessions()) != 0 || len(s.Aliases()) != 0 {
		t.Errorf("sessions %v aliases %v", s.Sessions(), s.Aliases())
	}
	if _, err := s.Resolve("work"); err == nil {
		t.Error("resolved a folder with no notes")
	}
}

func TestAliasList(t *testing.T) {
	s := aliasStore(t, "c5d05701", "dfc08fd7")
	s.SetAlias("c5d05701", "zz")
	s.SetAlias("dfc08fd7", "aa")
	l := s.AliasList()
	if len(l) != 2 || l[0].Name != "aa" || l[1].Name != "zz" {
		t.Errorf("%v", l)
	}
}

func TestSetAliasFromTakesAName(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, id := range []string{"c5d05701", "dfc08fd7"} {
		if err := s.WriteFile(s.Path(ID(id), "notes"), []byte("Objective: x\n")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetAlias("c5d05701", "deck"); err != nil {
		t.Fatal(err)
	}
	no := func(ID) bool { return false }
	yes := func(ID) bool { return true }
	var taken *AliasTakenError
	if _, _, err := s.SetAliasFrom("dfc08fd7", "deck", nil); !errors.As(err, &taken) {
		t.Errorf("no way to take: %v", err)
	}
	if _, _, err := s.SetAliasFrom("dfc08fd7", "deck", no); !errors.As(err, &taken) || s.Alias("c5d05701") != "deck" {
		t.Errorf("the holder may not be taken from: %v", err)
	}
	changed, from, err := s.SetAliasFrom("dfc08fd7", "deck", yes)
	if err != nil || !changed || from != "c5d05701" || s.Alias("dfc08fd7") != "deck" || s.Alias("c5d05701") != "" {
		t.Errorf("take: %v %v %q, names %q and %q", changed, err, from, s.Alias("dfc08fd7"), s.Alias("c5d05701"))
	}
	if _, _, err := s.SetAliasFrom("c5d05701", "dfc0", yes); err == nil {
		t.Error("a name that is the start of an id is still refused")
	}
	if changed, from, err := s.SetAliasFrom("dfc08fd7", "free", yes); err != nil || !changed || from != "" {
		t.Errorf("a free name takes from nobody: %v %v %q", changed, err, from)
	}
}
