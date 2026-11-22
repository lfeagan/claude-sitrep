package conf

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/settings"
)

func p(s string) *string { return &s }

func TestEditRules(t *testing.T) {
	blockText := "\n" + examplesBegin + "\n# x\n" + examplesEnd + "\n"
	cases := []struct {
		name, body string
		ch         []Change
		want       string
		dropped    int
	}{
		{"adds to an empty file", "", []Change{{"SITREP_THEME", p("rainbow")}}, "SITREP_THEME=rainbow\n", 0},
		{"adds at the end", "A=1\nB=2\n", []Change{{"SITREP_THEME", p("pastel")}}, "A=1\nB=2\nSITREP_THEME=pastel\n", 0},
		{"adds after a last line with no ending", "A=1", []Change{{"SITREP_THEME", p("pastel")}}, "A=1\nSITREP_THEME=pastel\n", 0},
		{"adds above the examples block", "A=1\n" + blockText, []Change{{"SITREP_COLORS", p("attn=#ff0000")}}, "A=1\nSITREP_COLORS='attn=#ff0000'\n" + blockText, 0},
		{"replaces in place", "A=1\nSITREP_THEME=old\nB=2\n", []Change{{"SITREP_THEME", p("mono")}}, "A=1\nSITREP_THEME=mono\nB=2\n", 0},
		{"removes", "A=1\nSITREP_THEME=old\nB=2\n", []Change{{"SITREP_THEME", nil}}, "A=1\nB=2\n", 0},
		{"drops later lines of the same name", "SITREP_THEME=a\nX=1\nSITREP_THEME=b\n", []Change{{"SITREP_THEME", p("mono")}}, "SITREP_THEME=mono\nX=1\n", 1},
		{"leaves a comment, an export and an indented line", "# SITREP_THEME=a\nexport SITREP_THEME=b\n SITREP_THEME=c\n", []Change{{"SITREP_THEME", p("mono")}}, "# SITREP_THEME=a\nexport SITREP_THEME=b\n SITREP_THEME=c\nSITREP_THEME=mono\n", 0},
		{"keeps CRLF", "A=1\r\nSITREP_THEME=a\r\n", []Change{{"SITREP_THEME", p("mono")}, {"SITREP_COLORS", p("dim=7")}}, "A=1\r\nSITREP_THEME=mono\r\nSITREP_COLORS='dim=7'\r\n", 0},
		{"both at once", "", []Change{{"SITREP_THEME", p("rainbow")}, {"SITREP_COLORS", p("hot=236")}}, "SITREP_THEME=rainbow\nSITREP_COLORS='hot=236'\n", 0},
	}
	for _, c := range cases {
		got, dropped, err := Edit(c.body, c.ch)
		if err != nil || got != c.want || dropped != c.dropped {
			t.Errorf("%s:\n got %q (dropped %d, %v)\nwant %q (dropped %d)", c.name, got, dropped, err, c.want, c.dropped)
		}
	}
}

func TestEditRefusesWhatTheDashboardWouldIgnore(t *testing.T) {
	for _, ch := range []Change{
		{"SITREP_QUEUE_ITEMS", p("9")},
		{"SITREP_THEME", p("Bad Name")},
		{"SITREP_THEME", p("a\nB=1")},
		{"SITREP_COLORS", p("accent=red;rm")},
		{"SITREP_COLORS", p(strings.Repeat("a", MaxColors+1))},
		{"SITREP_COLORS", p("a=1\n")},
	} {
		if out, _, err := Edit("A=1\n", []Change{ch}); err == nil {
			t.Errorf("%v was accepted: %q", ch, out)
		}
	}
	// what the file would hold reads back as what was meant
	out, _, _ := Edit("", []Change{{"SITREP_COLORS", p("accent=#ff8800, attn=1/2")}})
	dir := t.TempDir()
	c := Open(dir)
	os.WriteFile(c.File, []byte(out), 0o600)
	if got := (Conf{File: c.File, Getenv: func(string) string { return "" }}).Colors(); got != "accent=#ff8800, attn=1/2" {
		t.Errorf("read back %q", got)
	}
}

func TestWriteBodyFollowsALinkAndKeepsItsMode(t *testing.T) {
	home := t.TempDir()
	dots := t.TempDir()
	target := filepath.Join(dots, "sitrep.conf")
	os.WriteFile(target, []byte("A=1\n"), 0o640)
	c := Open(home)
	if err := os.Symlink(target, c.File); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteBody("A=1\nSITREP_THEME=mono\n", "A=1\n"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(c.File); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a file: %v %v", fi, err)
	}
	got, _ := os.ReadFile(target)
	fi, _ := os.Stat(target)
	if string(got) != "A=1\nSITREP_THEME=mono\n" || fi.Mode().Perm() != 0o640 {
		t.Errorf("target %q mode %v", got, fi.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dots); len(entries) != 1 {
		t.Errorf("no temporary file is left behind: %v", entries)
	}
}

func TestWriteBodyStopsWhenTheFileChanged(t *testing.T) {
	c := Open(t.TempDir())
	os.WriteFile(c.File, []byte("A=1\n"), 0o600)
	os.WriteFile(c.File, []byte("A=1\nEDITED=1\n"), 0o600) // the user's editor, after we read
	err := c.WriteBody("A=1\nSITREP_THEME=mono\n", "A=1\n")
	if !errors.Is(err, settings.ErrChanged) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(c.File); string(got) != "A=1\nEDITED=1\n" {
		t.Errorf("the user's edit must stay: %q", got)
	}
}

func TestWriteBodyCreatesAMissingFileOwnerOnly(t *testing.T) {
	c := Open(t.TempDir())
	if err := c.WriteBody("SITREP_THEME=mono\n", ""); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(c.File)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("%v %v", fi, err)
	}
}

func TestBackupIsOwnerOnlyAndNeverOverwrites(t *testing.T) {
	c := Open(t.TempDir())
	at := time.Unix(1_800_000_000, 0)
	a, err := c.Backup("one", at)
	b, err2 := c.Backup("two", at)
	if err != nil || err2 != nil || a == b {
		t.Fatalf("%v %v %q %q", err, err2, a, b)
	}
	got, _ := os.ReadFile(a)
	fi, _ := os.Stat(a)
	if string(got) != "one" || fi.Mode().Perm() != 0o600 || filepath.Dir(a) != filepath.Dir(c.File) {
		t.Errorf("first backup %q %v", got, fi.Mode().Perm())
	}
}

func TestSetupWritersKeepALinkedConf(t *testing.T) {
	home := t.TempDir()
	dots := t.TempDir()
	target := filepath.Join(dots, "conf")
	os.WriteFile(target, []byte("MINE=1\n"), 0o600)
	c := Open(home)
	os.Symlink(target, c.File)
	if err := c.PutHUD("echo hi"); err != nil {
		t.Fatal(err)
	}
	if err := c.PutExamples(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(c.File); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("setup replaced the link with a file")
	}
	if got, _ := os.ReadFile(target); !strings.Contains(string(got), "MINE=1") || !strings.Contains(string(got), "SITREP_HUD_CMD=") {
		t.Errorf("the target holds %q", got)
	}
}

func TestOtherLinesIgnoresTheEndingAWriteAdds(t *testing.T) {
	out, _, _ := Edit("A=1", []Change{{"SITREP_THEME", p("mono")}})
	if OtherLines("A=1") != OtherLines(out) {
		t.Errorf("%q vs %q", OtherLines("A=1"), OtherLines(out))
	}
	if OtherLines("A=1\nB=2\n") == OtherLines("A=1\nB=3\n") {
		t.Error("a changed line is a difference")
	}
}
