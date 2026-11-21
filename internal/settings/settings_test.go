package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for in, want := range map[string]error{
		`{}`: nil, `{"a":1}`: nil, "{\r\n}": nil,
		``: ErrNotJSON, `{`: ErrNotJSON, `{"a":1,}`: ErrNotJSON, "{ // c\n}": ErrNotJSON, "\xef\xbb\xbf{}": ErrNotJSON, `{} x`: ErrNotJSON,
		`[]`: ErrNotObject, `null`: ErrNotObject, `5`: ErrNotObject, `"x"`: ErrNotObject,
		`{"a":1,"a":2}`: ErrDuplicate, `{"statusLine":{"command":"x","command":"y"}}`: ErrDuplicate, `{"statusLine":1,"statusLine":2}`: ErrDuplicate,
	} {
		if _, err := Parse([]byte(in)); !errors.Is(err, want) {
			t.Errorf("Parse(%q) = %v, want %v", in, err, want)
		}
	}
	if _, err := Parse([]byte(`{"hooks":{"a":1,"a":2}}`)); err != nil {
		t.Errorf("a duplicate deeper than statusLine is not our business: %v", err)
	}
}

func TestSplitBOM(t *testing.T) {
	if bom, rest := SplitBOM([]byte("\xef\xbb\xbf{}")); bom != BOM || string(rest) != "{}" {
		t.Errorf("%q %q", bom, rest)
	}
	if bom, rest := SplitBOM([]byte("{}")); bom != "" || string(rest) != "{}" {
		t.Errorf("%q %q", bom, rest)
	}
}

func TestStatusLineShapes(t *testing.T) {
	for in, c := range map[string]struct {
		cmd string
		ok  bool
	}{
		`{}`: {"", true}, `{"statusLine":null}`: {"", true}, `{"statusLine":{}}`: {"", true}, `{"statusLine":{"command":"x"}}`: {"x", true},
		`{"statusLine":"s"}`: {"", false}, `{"statusLine":[1]}`: {"", false}, `{"statusLine":false}`: {"", false}, `{"statusLine":{"command":5}}`: {"", false}, `{"statusLine":{"command":{}}}`: {"", false},
	} {
		cmd, ok := Doc(in).StatusLine()
		if cmd != c.cmd || ok != c.ok {
			t.Errorf("%s: %q, %v", in, cmd, ok)
		}
	}
}

func TestSetAndRestore(t *testing.T) {
	d := Doc("{\n  \"b\": 2,\n  \"a\": {\"x\": [1, 2]}\n}\n")
	set, err := d.SetStatusLine(`"$HOME/x" y # t`)
	if err != nil || Verify(d, set, `"$HOME/x" y # t`) != nil {
		t.Fatalf("%v", err)
	}
	if !strings.HasPrefix(string(set), "{\n  \"b\": 2,\n  \"a\": {\"x\": [1, 2]}") {
		t.Errorf("the original text moved: %q", set)
	}
	back, err := set.RestoreStatusLine("")
	if err != nil || strings.Contains(string(back), "statusLine") {
		t.Errorf("%q %v", back, err)
	}
	if Verify(set, back, "") != nil {
		t.Error("a correct removal did not verify")
	}
}

func TestVerifyRefusesABadEdit(t *testing.T) {
	before := Doc(`{"a":1,"b":{"c":[1,2]}}`)
	for name, after := range map[string]string{
		"invalid":        `{"a":1,`,
		"changed key":    `{"a":2,"b":{"c":[1,2]},"statusLine":{"command":"X"}}`,
		"changed nested": `{"a":1,"b":{"c":[1,3]},"statusLine":{"command":"X"}}`,
		"lost key":       `{"a":1,"statusLine":{"command":"X"}}`,
		"added key":      `{"a":1,"b":{"c":[1,2]},"z":0,"statusLine":{"command":"X"}}`,
		"wrong command":  `{"a":1,"b":{"c":[1,2]},"statusLine":{"command":"Y"}}`,
		"no command":     `{"a":1,"b":{"c":[1,2]}}`,
	} {
		if Verify(before, Doc(after), "X") == nil {
			t.Errorf("%s: verified", name)
		}
	}
	if Verify(before, Doc(`{"a":1,"b":{"c":[1, 2]},"statusLine":{"command":"X"}}`), "X") != nil {
		t.Error("whitespace inside a value is not a change")
	}
	if Verify(before, Doc(`{"a":1,"b":{"c":[1,2]},"statusLine":{"command":"X"}}`), "") == nil {
		t.Error("a status line that should be gone was accepted")
	}
}

func TestHookCommands(t *testing.T) {
	d := Doc(`{"hooks":{"Stop":[{"hooks":[{"command":"a"},{"command":5},{"type":"x"}]},"s",{"hooks":"no"}],"E":{"x":1},"F":"s"}}`)
	if got := d.HookCommands(); len(got) != 1 || got[0] != "a" {
		t.Errorf("%v", got)
	}
	if got := Doc(`{"hooks":5}`).HookCommands(); len(got) != 0 {
		t.Errorf("%v", got)
	}
}

func TestBackupNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var names []string
	for _, body := range []string{"one", "two", "three"} {
		n, err := Backup(p, []byte(body), at)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if names[0] != p+".bak.20260102T030405" || names[1] != names[0]+".1" || names[2] != names[0]+".2" {
		t.Errorf("%v", names)
	}
	for i, body := range []string{"one", "two", "three"} {
		if b, _ := os.ReadFile(names[i]); string(b) != body {
			t.Errorf("%s holds %q", names[i], b)
		}
		if fi, _ := os.Stat(names[i]); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", fi.Mode().Perm())
		}
	}
}

func TestWriteRefusesWhenTheFileChanged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(p, []byte(`{"a":1}`), 0o644)
	if err := Write(p, Doc(`{"a":2}`), []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"a":2}` {
		t.Errorf("%q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	// someone else wrote it in between
	if err := Write(p, Doc(`{"a":3}`), []byte(`{"a":1}`)); !errors.Is(err, ErrChanged) {
		t.Errorf("%v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"a":2}` {
		t.Errorf("a refused write changed the file: %q", b)
	}
	// it was created while a new file was being made
	q := filepath.Join(t.TempDir(), "new.json")
	os.WriteFile(q, []byte("theirs"), 0o600)
	if err := Write(q, Doc("{}"), nil); !errors.Is(err, ErrChanged) {
		t.Errorf("%v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".settings.*")); len(m) != 0 {
		t.Errorf("temporary files left: %v", m)
	}
}

func TestWriteNewFileIsOwnerOnlyAndFollowsLinks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "new.json")
	if err := Write(p, Doc("{}"), nil); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	// a chain of links, the last one dangling
	target := filepath.Join(dir, "real", "s.json")
	os.MkdirAll(filepath.Dir(target), 0o755)
	l1, l2 := filepath.Join(dir, "l1"), filepath.Join(dir, "l2")
	os.Symlink(target, l1)
	os.Symlink("l1", l2) // relative
	if err := Write(l2, Doc("{\"x\":1}"), nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "{\"x\":1}" {
		t.Errorf("%q", b)
	}
	for _, l := range []string{l1, l2} {
		if fi, _ := os.Lstat(l); fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is no longer a link", l)
		}
	}
	// a loop of links is an error, not a hang
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.Symlink(b, a)
	os.Symlink(a, b)
	if err := Write(a, Doc("{}"), nil); err == nil {
		t.Error("a loop of links was written")
	}
}
