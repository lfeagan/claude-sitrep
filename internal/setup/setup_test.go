package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

type env struct {
	t        *testing.T
	home     string
	binDir   string
	skills   string
	exe      string
	out, err bytes.Buffer
}

var now = time.Date(2026, 11, 20, 12, 30, 45, 0, time.UTC)

func newEnv(t *testing.T) *env {
	t.Helper()
	home := t.TempDir()
	e := &env{t: t, home: home, binDir: filepath.Join(home, "bin"), skills: filepath.Join(home, ".claude", "skills")}
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	e.exe = filepath.Join(t.TempDir(), "sitrep-built")
	os.WriteFile(e.exe, []byte("#!/bin/sh\necho the program\n"), 0o755)
	return e
}

func (e *env) settings() string { return filepath.Join(e.home, ".claude", "settings.json") }
func (e *env) nd() string       { return filepath.Join(e.home, ".claude", "sitrep") }

func (e *env) write(path, body string) {
	e.t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func (e *env) run(remove bool) int {
	e.out.Reset()
	e.err.Reset()
	return Setup{Home: e.home, BinDir: e.binDir, SkillsDir: e.skills, Executable: func() (string, error) { return e.exe, nil }, Now: func() time.Time { return now }, Out: &e.out, Err: &e.err}.Run(remove)
}

func (e *env) backups() []string {
	m, _ := filepath.Glob(e.settings() + ".bak.*")
	return m
}

func (e *env) exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestFreshSetup(t *testing.T) {
	e := newEnv(t)
	if code := e.run(false); code != 0 {
		t.Fatalf("exit %d: %s", code, e.err.String())
	}
	if got := e.out.String(); got != "No existing status line found; showing the sitrep block only.\nSet up. Restart Claude Code sessions to pick up the status line.\n" {
		t.Errorf("output %q", got)
	}
	doc := e.read(e.settings())
	if gjson.Get(doc, "statusLine.command").String() != Command || gjson.Get(doc, "statusLine.type").String() != "command" {
		t.Errorf("settings %q", doc)
	}
	if fi, _ := os.Stat(e.settings()); fi.Mode().Perm() != 0o600 {
		t.Errorf("a new settings file has mode %v", fi.Mode().Perm())
	}
	if len(e.backups()) != 0 {
		t.Error("a backup of a file that did not exist")
	}
	bin := filepath.Join(e.nd(), "bin", "sitrep")
	if fi, err := os.Stat(bin); err != nil || fi.Mode().Perm() != 0o700 || e.read(bin) != e.read(e.exe) {
		t.Errorf("the program was not installed: %v", err)
	}
	if fi, _ := os.Stat(filepath.Dir(bin)); fi.Mode().Perm() != 0o700 {
		t.Errorf("bin folder mode %v", fi.Mode().Perm())
	}
	if l, _ := os.Readlink(filepath.Join(e.binDir, "sitrep")); l != bin {
		t.Errorf("link -> %q", l)
	}
}

func TestOtherKeysAndFormattingStay(t *testing.T) {
	for name, in := range map[string]string{
		"pretty":       "{\n  \"model\": \"opus\",\n  \"env\": {\"A\": \"1\"},\n  \"permissions\": {\"allow\": [\"Bash(ls)\"]}\n}\n",
		"crlf":         "{\r\n  \"model\": \"opus\"\r\n}\r\n",
		"no newline":   `{"zeta":1,"alpha":2}`,
		"big integer":  `{"n":12345678901234567890123,"f":1.0,"e":1e3}`,
		"unicode":      "{\"name\":\"\u00fc\u4e16\u754c\"}\n",
		"empty object": "{}",
	} {
		e := newEnv(t)
		e.write(e.settings(), in)
		if code := e.run(false); code != 0 {
			t.Fatalf("%s: exit %d: %s", name, code, e.err.String())
		}
		got := e.read(e.settings())
		// everything before the added key is the input, byte for byte, up to its closing brace
		head := strings.TrimRight(in, "\r\n")
		head = strings.TrimSuffix(head, "}")
		if !strings.HasPrefix(got, head) {
			t.Errorf("%s: the original text did not stay as it was:\n in:  %q\n out: %q", name, in, got)
		}
		if gjson.Get(got, "statusLine.command").String() != Command {
			t.Errorf("%s: %q", name, got)
		}
		if bak := e.backups(); len(bak) != 1 || e.read(bak[0]) != in || !strings.HasSuffix(bak[0], ".bak.20261120T123045") {
			t.Errorf("%s: backups %v", name, bak)
		}
	}
}

func TestStatusLineShapes(t *testing.T) {
	for name, c := range map[string]struct {
		in      string
		ok      bool
		padding bool
	}{
		"absent":       {`{"a":1}`, true, false},
		"null":         {`{"a":1,"statusLine":null}`, true, false},
		"object":       {`{"statusLine":{"type":"command","command":"old","padding":2}}`, true, true},
		"object empty": {`{"statusLine":{}}`, true, false},
		"string":       {`{"statusLine":"x"}`, false, false},
		"array":        {`{"statusLine":[1]}`, false, false},
		"number":       {`{"statusLine":3}`, false, false},
	} {
		e := newEnv(t)
		e.write(e.settings(), c.in)
		code := e.run(false)
		if c.ok != (code == 0) {
			t.Errorf("%s: exit %d: %s", name, code, e.err.String())
			continue
		}
		if !c.ok {
			if e.read(e.settings()) != c.in || len(e.backups()) != 0 || e.exists(e.nd()) || !strings.Contains(e.err.String(), "is not an object; nothing was changed.") {
				t.Errorf("%s: something changed or no message: %q", name, e.err.String())
			}
			continue
		}
		if c.padding && gjson.Get(e.read(e.settings()), "statusLine.padding").Int() != 2 {
			t.Errorf("%s: padding lost: %s", name, e.read(e.settings()))
		}
	}
}

func TestInvalidJSONChangesNothing(t *testing.T) {
	for name, in := range map[string]string{
		"garbage": "not json", "truncated": `{"a":`, "trailing comma": `{"a":1,}`, "comment": "{ // c\n}", "array": `[1]`, "empty": "", "duplicate key": `{"a":1,"a":2}`, "duplicate status line key": `{"statusLine":{"command":"x","command":"y"}}`, "null": "null", "trailing": `{} x`,
	} {
		e := newEnv(t)
		e.write(e.settings(), in)
		if code := e.run(false); code != 1 {
			t.Errorf("%s: exit %d", name, code)
		}
		if e.read(e.settings()) != in || len(e.backups()) != 0 || e.exists(e.nd()) || e.exists(e.binDir) {
			t.Errorf("%s: something changed", name)
		}
		if !strings.Contains(e.err.String(), "nothing was changed.") {
			t.Errorf("%s: message %q", name, e.err.String())
		}
	}
}

func TestUpstreamIsWrappedAndSetupIsRepeatable(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"statusLine":{"type":"command","command":"bash ~/.claude/hud.sh --color 'x y'"}}`)
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if !strings.Contains(e.out.String(), "Wrapping the existing status line (it renders first): bash ~/.claude/hud.sh --color 'x y'\n") {
		t.Errorf("output %q", e.out.String())
	}
	if got := e.read(filepath.Join(e.nd(), "sitrep.conf")); got != "SITREP_HUD_CMD='bash ~/.claude/hud.sh --color '\\''x y'\\'''\n" {
		t.Errorf("conf %q", got)
	}
	before := e.read(e.settings())
	e.out.Reset()
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if !strings.Contains(e.out.String(), "Status line is already set up; keeping the saved upstream (if any).") || e.read(e.settings()) != before {
		t.Errorf("second run: %q", e.out.String())
	}
	if got := e.read(filepath.Join(e.nd(), "sitrep.conf")); !strings.Contains(got, "hud.sh") {
		t.Errorf("the saved upstream was lost: %q", got)
	}
}

func TestSetupWithNoUpstreamDropsAStaleOne(t *testing.T) {
	e := newEnv(t)
	e.write(filepath.Join(e.nd(), "sitrep.conf"), "SITREP_QUEUE_ITEMS=9\nSITREP_HUD_CMD='stale'\n")
	e.write(e.settings(), `{}`)
	e.run(false)
	if got := e.read(filepath.Join(e.nd(), "sitrep.conf")); got != "SITREP_QUEUE_ITEMS=9\n" {
		t.Errorf("conf %q", got)
	}
}

func TestHostileUpstreamIsShownSafely(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"statusLine":{"command":"echo \u001b[31mred\u202e"}}`)
	e.run(false)
	if strings.ContainsAny(e.out.String(), "\x1b\u202e") {
		t.Errorf("output %q", e.out.String())
	}
}

func TestSymlinkedSettingsStaysALink(t *testing.T) {
	e := newEnv(t)
	real := filepath.Join(t.TempDir(), "dotfiles", "settings.json")
	e.write(real, `{"model":"opus"}`)
	os.Chmod(real, 0o644)
	if err := os.Symlink(real, e.settings()); err != nil {
		t.Fatal(err)
	}
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if fi, _ := os.Lstat(e.settings()); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("settings.json is no longer a link")
	}
	if gjson.Get(e.read(real), "statusLine.command").String() != Command || gjson.Get(e.read(real), "model").String() != "opus" {
		t.Errorf("the linked file: %q", e.read(real))
	}
	if fi, _ := os.Stat(real); fi.Mode().Perm() != 0o644 {
		t.Errorf("the mode of the real file changed to %v", fi.Mode().Perm())
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(real), ".settings.*")); len(m) != 0 {
		t.Errorf("temporary files left: %v", m)
	}
}

func TestBackupIsOwnerOnly(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"env":{"TOKEN":"secret"}}`)
	os.Chmod(e.settings(), 0o644)
	e.run(false)
	bak := e.backups()
	if len(bak) != 1 {
		t.Fatalf("backups %v", bak)
	}
	if fi, _ := os.Stat(bak[0]); fi.Mode().Perm() != 0o600 {
		t.Errorf("backup mode %v", fi.Mode().Perm())
	}
}

func TestCheckoutInstallsStopSetup(t *testing.T) {
	for name, c := range map[string]struct {
		settings string
		prepare  func(e *env)
		want     string
	}{
		"hook":                   {`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"bash /x/claude-sitrep/hooks/sitrep-hook.sh"}]}]}}`, nil, "a hook in settings.json: bash /x/claude-sitrep/hooks/sitrep-hook.sh"},
		"old hook":               {`{"hooks":{"SessionStart":[{"hooks":[{"command":"bash /x/hooks/notes-hook.sh"}]}]}}`, nil, "a hook in settings.json"},
		"status line":            {`{"statusLine":{"command":"bash /x/claude-sitrep/statusline/statusline.sh # claude-sitrep"}}`, nil, "the status line: bash /x/claude-sitrep/statusline/statusline.sh # claude-sitrep"},
		"old plugin status line": {`{"statusLine":{"command":"bash \"$HOME/.claude/sitrep/statusline.sh\" # claude-sitrep"}}`, nil, "the status line of the 0.3.x plugin setup"},
		"untagged status line":   {`{"statusLine":{"command":"bash /x/claude-sitrep/statusline/statusline.sh"}}`, nil, "the status line:"},
		"skill link": {`{}`, func(e *env) {
			os.MkdirAll(e.skills, 0o755)
			os.MkdirAll("/tmp/does-not-exist-claude-sitrep/skills", 0o755)
			os.Symlink("/nowhere/claude-sitrep/skills/sitrep-queue", filepath.Join(e.skills, "sitrep-queue"))
		}, "the skill link"},
		"bin link": {`{}`, func(e *env) {
			os.MkdirAll(e.binDir, 0o755)
			os.Symlink("/nowhere/claude-sitrep/bin/sitrep", filepath.Join(e.binDir, "q"))
		}, "the link"},
	} {
		e := newEnv(t)
		e.write(e.settings(), c.settings)
		if c.prepare != nil {
			c.prepare(e)
		}
		if code := e.run(false); code != 1 {
			t.Errorf("%s: exit %d", name, code)
		}
		if !strings.Contains(e.err.String(), c.want) || !strings.Contains(e.err.String(), "Nothing was changed.") {
			t.Errorf("%s: message %q", name, e.err.String())
		}
		if e.read(e.settings()) != c.settings || len(e.backups()) != 0 || e.exists(e.nd()) || e.out.Len() != 0 {
			t.Errorf("%s: something changed", name)
		}
	}
}

func TestSeveralCheckoutLeftoversAreAllNamed(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"hooks":{"Stop":[{"hooks":[{"command":"bash /x/hooks/sitrep-hook.sh"}]}]},"statusLine":{"command":"x # claude-sitrep"}}`)
	e.run(false)
	if strings.Count(e.err.String(), "\n  - ") != 2 {
		t.Errorf("%q", e.err.String())
	}
}

func TestOtherToolsLookAlikesAreNotCheckoutInstalls(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"hooks":{"Stop":[{"hooks":[{"command":"bash /x/other/hooks/other-hook.sh"},{"type":"prompt"}]},{"hooks":[]}]},"statusLine":{"command":"my-hud"}}`)
	os.MkdirAll(e.binDir, 0o755)
	os.Symlink("/opt/other/bin/sitrep", filepath.Join(e.binDir, "other")) // another tool called sitrep: its folder does not look like ours
	if code := e.run(false); code != 0 {
		t.Fatalf("exit %d: %s", code, e.err.String())
	}
}

func TestBinFolderThatIsAFolderIsRefused(t *testing.T) {
	e := newEnv(t)
	os.MkdirAll(filepath.Join(e.nd(), "bin", "sitrep"), 0o700)
	e.write(e.settings(), `{}`)
	if code := e.run(false); code != 1 || e.read(e.settings()) != `{}` {
		t.Errorf("exit %d", code)
	}
}

func TestBinDirLinkIsLeftAloneWhenNotOurs(t *testing.T) {
	e := newEnv(t)
	os.MkdirAll(e.binDir, 0o755)
	e.write(filepath.Join(e.binDir, "sitrep"), "someone else's")
	e.run(false)
	if !strings.Contains(e.err.String(), "exists and is not ours; left alone. Run ") || e.read(filepath.Join(e.binDir, "sitrep")) != "someone else's" {
		t.Errorf("%q", e.err.String())
	}
	// ours: silent
	e2 := newEnv(t)
	e2.run(false)
	e2.err.Reset()
	e2.run(false)
	if e2.err.Len() != 0 {
		t.Errorf("a second run was noisy: %q", e2.err.String())
	}
}

func TestSetupRunningFromTheStablePathDoesNotCopyItself(t *testing.T) {
	e := newEnv(t)
	bin := filepath.Join(e.nd(), "bin", "sitrep")
	e.write(bin, "the built program")
	os.Chmod(bin, 0o700)
	e.exe = bin
	before, _ := os.Stat(bin)
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	after, _ := os.Stat(bin)
	if !os.SameFile(before, after) {
		t.Error("the program was replaced by a copy of itself")
	}
}

func TestAFailedSettingsWriteChangesNothingElse(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	e := newEnv(t)
	e.write(e.settings(), `{"a":1}`)
	os.Chmod(filepath.Dir(e.settings()), 0o500)
	defer os.Chmod(filepath.Dir(e.settings()), 0o755)
	if code := e.run(false); code != 1 {
		t.Errorf("exit %d", code)
	}
	os.Chmod(filepath.Dir(e.settings()), 0o755)
	if e.read(e.settings()) != `{"a":1}` || e.exists(e.nd()) || e.exists(e.binDir) {
		t.Error("a failed write left changes behind")
	}
}

func TestRemoveRestoresTheUpstream(t *testing.T) {
	e := newEnv(t)
	orig := "{\n  \"model\": \"opus\",\n  \"statusLine\": {\"type\": \"command\", \"command\": \"bash hud.sh\", \"padding\": 1}\n}\n"
	e.write(e.settings(), orig)
	e.write(filepath.Join(e.nd(), "mydata.bak"), "keep me\n")
	e.run(false)
	e.write(filepath.Join(e.nd(), "sitrep.conf"), e.read(filepath.Join(e.nd(), "sitrep.conf"))+"SITREP_QUEUE_ITEMS=4\n")
	if code := e.run(true); code != 0 {
		t.Fatal(e.err.String())
	}
	if got := e.out.String(); got != "Status line restored.\nRemoved. State in ~/.claude/sitrep is left in place. Run /plugin uninstall sitrep to remove the plugin itself.\n" {
		t.Errorf("output %q", got)
	}
	restored := e.read(e.settings())
	if gjson.Get(restored, "statusLine.command").String() != "bash hud.sh" || gjson.Get(restored, "statusLine.padding").Int() != 1 || gjson.Get(restored, "model").String() != "opus" {
		t.Errorf("settings %q", restored)
	}
	if got := e.read(filepath.Join(e.nd(), "sitrep.conf")); got != "SITREP_QUEUE_ITEMS=4\n" {
		t.Errorf("conf %q", got)
	}
	if e.exists(filepath.Join(e.nd(), "bin")) || e.exists(filepath.Join(e.binDir, "sitrep")) {
		t.Error("the program or its link is still there")
	}
	if e.read(filepath.Join(e.nd(), "mydata.bak")) != "keep me\n" {
		t.Error("state was touched")
	}
}

func TestRemoveWithNoUpstreamDeletesTheStatusLine(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"model":"opus"}`)
	e.run(false)
	e.run(true)
	if got := e.read(e.settings()); got != `{"model":"opus"}` {
		t.Errorf("round trip: %q", got)
	}
	if e.exists(filepath.Join(e.nd(), "sitrep.conf")) {
		t.Error("an empty conf was left")
	}
}

func TestRemoveLeavesAStatusLineThatIsNotOurs(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"statusLine":{"command":"someone else's"}}`)
	e.write(filepath.Join(e.nd(), "sitrep.conf"), "SITREP_HUD_CMD='stale'\n")
	if code := e.run(true); code != 0 {
		t.Fatal(e.err.String())
	}
	if !strings.Contains(e.out.String(), "The status line is not sitrep's; left alone.") || gjson.Get(e.read(e.settings()), "statusLine.command").String() != "someone else's" {
		t.Errorf("%q", e.out.String())
	}
}

func TestRemoveWithNoSettingsFile(t *testing.T) {
	e := newEnv(t)
	if code := e.run(true); code != 0 || e.exists(e.settings()) {
		t.Errorf("exit %d", code)
	}
}

func TestRemoveOnlyTouchesLinksToOurBinary(t *testing.T) {
	e := newEnv(t)
	e.run(false)
	os.Symlink("/elsewhere/sitrep", filepath.Join(e.binDir, "other"))
	e.write(filepath.Join(e.binDir, "plain"), "x")
	os.Symlink(filepath.Join(e.nd(), "bin", "sitrep"), filepath.Join(e.binDir, "q")) // an alias that is ours
	e.run(true)
	if !e.exists(filepath.Join(e.binDir, "other")) || !e.exists(filepath.Join(e.binDir, "plain")) {
		t.Error("a file that is not ours was removed")
	}
	if e.exists(filepath.Join(e.binDir, "q")) || e.exists(filepath.Join(e.binDir, "sitrep")) {
		t.Error("our links are still there")
	}
}

func TestRemoveDoesNotFollowOrDeleteOtherThingsInBin(t *testing.T) {
	e := newEnv(t)
	e.run(false)
	e.write(filepath.Join(e.nd(), "bin", "other-tool"), "not ours")
	e.run(true)
	if !e.exists(filepath.Join(e.nd(), "bin", "other-tool")) {
		t.Error("another file in bin was removed")
	}
}

func TestRoundTripIsByteForByteWithAnUpstream(t *testing.T) {
	e := newEnv(t)
	orig := "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"bash hud.sh\"\n  },\n  \"z\": 1\n}\n"
	e.write(e.settings(), orig)
	e.run(false)
	e.run(true)
	got := e.read(e.settings())
	if gjson.Get(got, "statusLine.command").String() != "bash hud.sh" || gjson.Get(got, "z").Int() != 1 {
		t.Errorf("%q", got)
	}
}

func TestAByteOrderMarkIsKept(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), "\xef\xbb\xbf{\"model\":\"opus\"}\n")
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	got := e.read(e.settings())
	if !strings.HasPrefix(got, "\xef\xbb\xbf{\"model\":\"opus\"") || !strings.Contains(got, "statusLine") {
		t.Errorf("%q", got)
	}
	if bak := e.backups(); len(bak) != 1 || e.read(bak[0]) != "\xef\xbb\xbf{\"model\":\"opus\"}\n" {
		t.Error("the backup is not the original bytes")
	}
}

func TestTwoRunsInOneSecondKeepTheFirstBackup(t *testing.T) {
	e := newEnv(t)
	orig := `{"env":{"A":"1"}}`
	e.write(e.settings(), orig)
	e.run(false)
	e.run(true) // the same second: the clock is fixed
	e.run(false)
	var pristine bool
	for _, b := range e.backups() {
		if e.read(b) == orig {
			pristine = true
		}
	}
	if len(e.backups()) != 3 || !pristine {
		t.Errorf("backups %v", e.backups())
	}
}

func TestStatusLineCommandThatIsNotAStringIsRefused(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"statusLine":{"command":{"x":1}}}`)
	if code := e.run(false); code != 1 || !strings.Contains(e.err.String(), "is not an object; nothing was changed.") {
		t.Errorf("exit %d: %q", code, e.err.String())
	}
}

func TestAProgramThatCannotBeInstalledPutsTheSettingsBack(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	e := newEnv(t)
	orig := `{"model":"opus"}`
	e.write(e.settings(), orig)
	os.MkdirAll(filepath.Join(e.nd(), "bin"), 0o700)
	os.Chmod(filepath.Join(e.nd(), "bin"), 0o500) // the program cannot be copied in
	defer os.Chmod(filepath.Join(e.nd(), "bin"), 0o700)
	if code := e.run(false); code != 1 || !strings.Contains(e.err.String(), "The settings were put back.") {
		t.Errorf("exit %d: %q", code, e.err.String())
	}
	os.Chmod(filepath.Join(e.nd(), "bin"), 0o700)
	if e.read(e.settings()) != orig {
		t.Errorf("the settings were left changed: %q", e.read(e.settings()))
	}
}

func TestABinThatIsAFileIsRefused(t *testing.T) {
	e := newEnv(t)
	e.write(filepath.Join(e.nd(), "bin"), "a file")
	e.write(e.settings(), `{}`)
	if code := e.run(false); code != 1 || e.read(e.settings()) != `{}` || len(e.backups()) != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestADanglingSettingsLinkIsCreatedWhereItPoints(t *testing.T) {
	e := newEnv(t)
	real := filepath.Join(t.TempDir(), "dotfiles", "settings.json")
	os.MkdirAll(filepath.Dir(real), 0o755)
	os.Symlink(real, e.settings())
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if fi, _ := os.Lstat(e.settings()); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if !strings.Contains(e.read(real), "statusLine") {
		t.Errorf("%q", e.read(real))
	}
}

func TestMissingClaudeFolderIsCreatedOwnerOnly(t *testing.T) {
	e := newEnv(t)
	os.RemoveAll(filepath.Join(e.home, ".claude"))
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if fi, err := os.Stat(filepath.Join(e.home, ".claude")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("%v", err)
	}
}

func TestTheGuardPointsToUninstallWhenTheCheckoutIsStillThere(t *testing.T) {
	e := newEnv(t)
	root := filepath.Join(t.TempDir(), "claude-sitrep")
	os.MkdirAll(filepath.Join(root, "hooks"), 0o755)
	os.WriteFile(filepath.Join(root, "uninstall.sh"), []byte("#!/bin/bash\n"), 0o755)
	e.write(e.settings(), `{"hooks":{"Stop":[{"hooks":[{"command":"bash `+root+`/hooks/sitrep-hook.sh"}]}]}}`)
	e.run(false)
	if !strings.Contains(e.err.String(), "  bash "+root+"/uninstall.sh") {
		t.Errorf("%q", e.err.String())
	}
	e2 := newEnv(t)
	e2.write(e2.settings(), `{"hooks":{"Stop":[{"hooks":[{"command":"bash /gone/claude-sitrep/hooks/sitrep-hook.sh"}]}]}}`)
	e2.run(false)
	if !strings.Contains(e2.err.String(), "/gone/claude-sitrep is gone: remove its entries from ~/.claude/settings.json by hand") {
		t.Errorf("%q", e2.err.String())
	}
}

func TestTheGuardToleratesOddHookShapes(t *testing.T) {
	for name, in := range map[string]string{
		"hooks a number":  `{"hooks":5}`,
		"events not list": `{"hooks":{"Stop":{"a":1},"SessionStart":"x"}}`,
		"no command":      `{"hooks":{"Stop":[{"hooks":[{"type":"prompt"},{"command":7},{"command":null}]}]}}`,
		"group a string":  `{"hooks":{"Stop":["x",null,[]]}}`,
	} {
		e := newEnv(t)
		e.write(e.settings(), in)
		if code := e.run(false); code != 0 {
			t.Errorf("%s: exit %d: %s", name, code, e.err.String())
		}
	}
}

func TestAConfThatCannotBeWrittenDoesNotStopSetup(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"statusLine":{"type":"command","command":"my hud"}}`)
	os.MkdirAll(filepath.Join(e.nd(), "sitrep.conf"), 0o700) // a folder where the file should be
	if code := e.run(false); code != 0 {
		t.Fatalf("exit %d: %s", code, e.err.String())
	}
	if !strings.Contains(e.err.String(), "Could not save the upstream status line to ") || !strings.Contains(e.err.String(), "The original is in ") {
		t.Errorf("%q", e.err.String())
	}
	if !strings.Contains(e.read(e.settings()), Command[:20]) || !e.exists(filepath.Join(e.binDir, "sitrep")) {
		t.Error("setup did not finish")
	}
}

func TestSetupAndRemoveReadTheConfFileOnly(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"statusLine":{"command":"saved upstream"}}`)
	e.run(false)
	t.Setenv("SITREP_HUD_CMD", "from the shell")
	e.run(true)
	if got := gjson.Get(e.read(e.settings()), "statusLine.command").String(); got != "saved upstream" {
		t.Errorf("restored %q", got)
	}
}

func TestRemoveReadsWhatBashWrote(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{}`)
	e.run(false)
	e.write(filepath.Join(e.nd(), "sitrep.conf"), "SITREP_HUD_CMD=bash\\ /x/hud.sh\\ --a\\ \\'b\\'\n")
	e.run(true)
	if got := gjson.Get(e.read(e.settings()), "statusLine.command").String(); got != "bash /x/hud.sh --a 'b'" {
		t.Errorf("restored %q", got)
	}
}

func TestUpstreamWithAmpersandsAndUnicodeRoundTrips(t *testing.T) {
	e := newEnv(t)
	up := `cd "$HOME" && ./hud.sh --name '\u00fc\u4e16' <in >out 2>&1; echo \\ done`
	b, _ := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": up}})
	e.write(e.settings(), string(b))
	e.run(false)
	e.run(true)
	if got := gjson.Get(e.read(e.settings()), "statusLine.command").String(); got != up {
		t.Errorf("restored %q, want %q", got, up)
	}
}

func TestTheSkillsBuildCopyIsCopiedInAndRemoved(t *testing.T) {
	e := newEnv(t)
	build := filepath.Join(e.nd(), BuildName)
	e.write(build, "#!/bin/sh\necho built\n")
	os.Chmod(build, 0o755)
	e.exe = build
	e.write(e.settings(), `{}`)
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if e.exists(build) {
		t.Error("the build copy was left behind")
	}
	if e.read(filepath.Join(e.nd(), "bin", "sitrep")) != "#!/bin/sh\necho built\n" {
		t.Error("the program was not installed from the build copy")
	}
	// any other program is never removed
	e2 := newEnv(t)
	e2.run(false)
	if !e2.exists(e2.exe) {
		t.Error("setup removed the program it ran from")
	}
}

func TestSetupThatChangesNothingMakesNoBackup(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{"a":1}`)
	e.run(false)
	before, backups := e.read(e.settings()), len(e.backups())
	e.run(false)
	if e.read(e.settings()) != before || len(e.backups()) != backups {
		t.Errorf("a second run changed the file or made %d more backups", len(e.backups())-backups)
	}
}

func TestSetupMovesStateOfTheOldLayout(t *testing.T) {
	e := newEnv(t)
	e.write(e.settings(), `{}`)
	e.write(filepath.Join(e.nd(), "13159d08.txt"), "Objective: x\n")
	e.write(filepath.Join(e.nd(), "13159d08.queue"), "item\n")
	e.write(filepath.Join(e.nd(), ".13159d08.state"), "waiting\n")
	e.write(filepath.Join(e.nd(), ".13159d08.prompt"), "old prompt")
	e.write(filepath.Join(e.nd(), "inbox.queue"), "i\n")
	if code := e.run(false); code != 0 {
		t.Fatal(e.err.String())
	}
	if !strings.Contains(e.out.String(), "Moved the state of the old layout into ~/.claude/sitrep/sessions: 3 moved, 0 merged, 0 left, 1 removed.") {
		t.Errorf("output %q", e.out.String())
	}
	if e.read(filepath.Join(e.nd(), "sessions", "13159d08", "queue")) != "item\n" || e.read(filepath.Join(e.nd(), "inbox.queue")) != "i\n" {
		t.Error("the state was not moved, or the inbox was")
	}
	// nothing to move: nothing said
	e2 := newEnv(t)
	e2.run(false)
	if strings.Contains(e2.out.String(), "Moved the state") {
		t.Errorf("%q", e2.out.String())
	}
}
