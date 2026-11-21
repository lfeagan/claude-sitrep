package cli

import (
	"bytes"
	"strings"
	"testing"
)

func run(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	e := Env{Home: home, Getenv: func(string) string { return "" }, Stdout: &out, Stderr: &errb}
	code := Run(args, e)
	return code, out.String(), errb.String()
}

func TestRunInProcess(t *testing.T) {
	home := t.TempDir()
	if code, out, _ := run(t, home, "add", "inbox", "first"); code != 0 || out != "queued on inbox (now 1)\n" {
		t.Errorf("add: %d %q", code, out)
	}
	if code, out, _ := run(t, home, "rm", "inbox"); code != 0 || out != "first\n" {
		t.Errorf("rm: %d %q", code, out)
	}
	if code, _, errs := run(t, home, "rm", "inbox"); code != 1 || errs != "sitrep: queue empty\n" {
		t.Errorf("rm empty: %d %q", code, errs)
	}
}

func TestErrorTextIsSafe(t *testing.T) {
	_, _, errs := run(t, t.TempDir(), "no\x1b[31m\u202ething")
	if strings.ContainsAny(errs, "\x1b\u202e") || errs != "sitrep: unknown command 'no[31mthing'; run 'sitrep help'\n" {
		t.Errorf("stderr %q", errs)
	}
	_, _, errs = run(t, t.TempDir(), "add", "x\x1b[0m", "text")
	if strings.Contains(errs, "\x1b") {
		t.Errorf("stderr %q", errs)
	}
}

func TestItemNo(t *testing.T) {
	for in, want := range map[string]int{"1": 1, "42": 42} {
		if got, err := itemNo(in); err != nil || got != want {
			t.Errorf("itemNo(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "0", "01", "-1", "1x", "x", " 1"} {
		if _, err := itemNo(in); err == nil {
			t.Errorf("itemNo(%q) accepted", in)
		}
	}
	if n, err := itemNo("99999999999999999999999"); err != nil || n < 1<<30 {
		t.Errorf("a huge number is a number past every item: %d, %v", n, err)
	}
}
