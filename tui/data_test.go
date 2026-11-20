package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func loadFixture(t *testing.T) Snapshot {
	t.Helper()
	f, err := os.Open("testdata/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDecodeFixture(t *testing.T) {
	s := loadFixture(t)
	if len(s.Sessions) != 4 || s.Registry != "ok" || s.Inbox.Count != 3 || s.Hidden.Ended != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if s.Sessions[2].QuietSecs != nil {
		t.Errorf("a null quiet_secs must stay nil")
	}
	if s.Sessions[1].QuietSecs == nil || *s.Sessions[1].QuietSecs != 1500 {
		t.Errorf("quiet_secs lost")
	}
}

func TestDecodeCleansText(t *testing.T) {
	s := loadFixture(t)
	for _, x := range s.Sessions {
		for _, str := range []string{x.Objective, x.Progress, x.Now, x.Dir, x.StateLabel} {
			if clean(str) != str {
				t.Errorf("text was not cleaned: %q", str)
			}
		}
	}
	if got := s.Sessions[2].Objective; strings.ContainsAny(got, "\x1b\a\t") || strings.ContainsRune(got, 0x202e) || strings.ContainsRune(got, 0x2028) {
		t.Errorf("hostile characters survived: %q", got)
	}
}

func TestDecodeVersion(t *testing.T) {
	for _, v := range []int{0, 2, 99} {
		_, err := decode(strings.NewReader(fmt.Sprintf(`{"version":%d,"sessions":[]}`, v)))
		if err == nil || !strings.Contains(err.Error(), "version") {
			t.Errorf("version %d should be refused, got %v", v, err)
		}
	}
}

func TestDecodeToleratesNewFields(t *testing.T) {
	s, err := decode(strings.NewReader(`{"version":1,"future":{"a":1},"sessions":[{"id":"x","running":true,"state":"waiting","extra":[1,2]}]}`))
	if err != nil || len(s.Sessions) != 1 {
		t.Fatalf("unknown fields must be ignored: %v", err)
	}
}

func TestDecodeSanityLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"version":1,"registry":"weird","sessions":[`)
	for i := 0; i < maxSessions+50; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"s%d","state":"bogus","age":-5,"queue":{"count":0,"items":["a"]}}`, i)
	}
	b.WriteString(`]}`)
	s, err := decode(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sessions) != maxSessions {
		t.Errorf("sessions not capped: %d", len(s.Sessions))
	}
	if s.Registry != "unavailable" || s.Sessions[0].State != "-" || s.Sessions[0].Age != 0 {
		t.Errorf("values not normalised: %+v", s.Sessions[0])
	}
	if s.Sessions[0].Queue.Count != 1 {
		t.Errorf("count must be at least the items listed")
	}
}

func TestDecodeBadInput(t *testing.T) {
	for _, in := range []string{"", "not json", `{"version":1,"sessions":[`, "[]"} {
		if _, err := decode(strings.NewReader(in)); err == nil {
			t.Errorf("%q should fail", in)
		}
	}
}

// script writes an executable shell script into dir and returns its path.
func script(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFetch(t *testing.T) {
	fixture, _ := filepath.Abs("testdata/snapshot.json")
	ctx := context.Background()

	ok := script(t, "sitrep", fmt.Sprintf(`[ "$1 $2 $3" = "ls --json -a" ] || { echo "bad args: $*" >&2; exit 9; }; cat %q`, fixture))
	if s, err := fetch(ctx, ok, 5*time.Second); err != nil || len(s.Sessions) != 4 {
		t.Fatalf("a good run failed: %v", err)
	}

	_, err := fetch(ctx, script(t, "sitrep", `echo "boom: it broke" >&2; exit 3`), 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "boom: it broke") {
		t.Errorf("a failing run should report stderr, got %v", err)
	}

	start := time.Now()
	_, err = fetch(ctx, script(t, "sitrep", `sleep 5`), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "took longer") || time.Since(start) > 3*time.Second {
		t.Errorf("a slow run should time out quickly, got %v after %s", err, time.Since(start))
	}

	if _, err = fetch(ctx, script(t, "sitrep", `echo "this is not json"`), 5*time.Second); err == nil {
		t.Errorf("junk output should fail")
	}
	if _, err = fetch(ctx, script(t, "sitrep", `exit 0`), 5*time.Second); err == nil {
		t.Errorf("empty output should fail")
	}

	_, err = fetch(ctx, script(t, "sitrep", `head -c 9000000 /dev/zero`), 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "too much") {
		t.Errorf("huge output should be refused, got %v", err)
	}

	_, err = fetch(ctx, script(t, "sitrep", fmt.Sprintf("printf 'evil \\033[31m\\a\\n' >&2; exit 4 # %s", "x")), 5*time.Second)
	if err == nil || strings.ContainsAny(err.Error(), "\x1b\a") {
		t.Errorf("an error message must be cleaned, got %q", err)
	}

	if _, err = fetch(ctx, filepath.Join(t.TempDir(), "missing"), time.Second); err == nil {
		t.Errorf("a missing binary should fail")
	}
}

func TestFindSitrep(t *testing.T) {
	good := script(t, "sitrep", "exit 0")
	t.Setenv("SITREP_BIN", good)
	if p, err := findSitrep(); err != nil || p != good {
		t.Errorf("SITREP_BIN should win: %q %v", p, err)
	}

	t.Setenv("SITREP_BIN", t.TempDir()) // a directory
	if _, err := findSitrep(); err == nil {
		t.Errorf("a directory is not an executable file")
	}

	plain := filepath.Join(t.TempDir(), "sitrep")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SITREP_BIN", plain)
	if _, err := findSitrep(); err == nil {
		t.Errorf("a file that is not executable must be refused")
	}

	t.Setenv("SITREP_BIN", "")
	dir := filepath.Dir(script(t, "sitrep", "exit 0"))
	t.Setenv("PATH", dir)
	if p, err := findSitrep(); err != nil || filepath.Base(p) != "sitrep" {
		t.Errorf("PATH lookup failed: %q %v", p, err)
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := findSitrep(); err == nil || !strings.Contains(err.Error(), "SITREP_BIN") {
		t.Errorf("a missing command should say how to fix it, got %v", err)
	}
}

func TestFetchNoisyStderrIsNotAnError(t *testing.T) {
	body := `head -c 20000 /dev/zero | tr '\\0' 'x' >&2; echo '{"version":1,"sessions":[]}'`
	s, err := fetch(context.Background(), script(t, "sitrep", body), 5*time.Second)
	if err != nil || s.Version != 1 {
		t.Fatalf("a noisy stderr must not fail a good run: %v", err)
	}
}

func TestFetchKillsGrandchildren(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	body := fmt.Sprintf(`trap '' TERM; sleep 30 & echo $! > %q; wait`, pidFile)
	if _, err := fetch(context.Background(), script(t, "sitrep", body), 400*time.Millisecond); err == nil {
		t.Fatal("a hung run must time out")
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &pid)
	alive := func() bool { return syscall.Kill(pid, 0) == nil }
	for i := 0; i < 50 && alive(); i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if alive() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("a grandchild outlived the timed-out call")
	}
}

func TestDecodeClampsCountsAndQueues(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"version":1,"hidden":{"ended":-4,"stranded_items":-1},"sessions":[{"id":"a","queue":{"count":150,"items":[`)
	for i := 0; i < 150; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"x"`)
	}
	b.WriteString(`]}}]}`)
	s, err := decode(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.Sessions[0].Queue.Items); got != maxItems {
		t.Errorf("items not capped: %d", got)
	}
	if s.Sessions[0].Queue.Count != 150 {
		t.Errorf("the true count must survive the cap: %d", s.Sessions[0].Queue.Count)
	}
	if s.Hidden.Ended != 0 || s.Hidden.StrandedItems != 0 {
		t.Errorf("negative counts must be clamped: %+v", s.Hidden)
	}
}
