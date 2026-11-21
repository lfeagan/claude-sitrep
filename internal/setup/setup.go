// Package setup puts sitrep's block on Claude Code's status line, and takes it off again. The plugin's hooks.json brings the hooks, so this
// writes none. It changes nothing until the settings edit has been checked and has succeeded, and it says what it found.
package setup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/migrate"
	"github.com/lfeagan/claude-sitrep/internal/settings"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// Command is the status line command sitrep writes. $HOME is expanded by the shell when Claude Code runs it, so the command works
// whatever the user's home is, and the tag at the end marks it as ours.
const Command = `"$HOME/.claude/sitrep/bin/sitrep" statusline ` + settings.Tag

// Setup is one run of setup or of its removal.
type Setup struct {
	Home       string
	BinDir     string // where the sitrep link goes; ~/bin when empty
	SkillsDir  string // where a checkout install linked the skill; ~/.claude/skills when empty
	Executable func() (string, error)
	Now        func() time.Time
	Out, Err   io.Writer
}

func (s Setup) settingsPath() string { return filepath.Join(s.Home, ".claude", "settings.json") }
func (s Setup) store() state.Store   { return state.Open(s.Home) }
func (s Setup) binPath() string      { return filepath.Join(s.store().Dir, "bin", "sitrep") }
func (s Setup) conf() conf.Conf      { return conf.Open(s.store().Dir) }
func (s Setup) binDir() string {
	if s.BinDir != "" {
		return s.BinDir
	}
	return filepath.Join(s.Home, "bin")
}
func (s Setup) skillsDir() string {
	if s.SkillsDir != "" {
		return s.SkillsDir
	}
	return filepath.Join(s.Home, ".claude", "skills")
}

// show makes text from settings or a path safe to print.
func show(s string) string { return textsafe.Strip(s).String() }

func (s Setup) say(format string, a ...any)  { fmt.Fprintf(s.Out, format+"\n", a...) }
func (s Setup) warn(format string, a ...any) { fmt.Fprintf(s.Err, format+"\n", a...) }

// Run sets up, or with remove undoes it, and returns the exit status.
func (s Setup) Run(remove bool) int {
	path := s.settingsPath()
	raw, exists, err := settings.Read(path)
	if err != nil {
		s.warn("cannot read %s: %v; nothing was changed.", show(path), err)
		return 1
	}
	bom, body := settings.SplitBOM(raw)
	doc := settings.Doc(body)
	if exists {
		if doc, err = settings.Parse(body); err != nil {
			s.warn("%s %v; nothing was changed.", show(path), err)
			return 1
		}
	}
	f := file{path: path, raw: raw, bom: bom, exists: exists}
	if remove {
		return s.remove(doc, f)
	}
	return s.install(doc, f)
}

// file is the settings file as it was read.
type file struct {
	path   string
	raw    []byte // everything, including a byte order mark
	bom    string
	exists bool
}

// original is what Write compares with: nil when there was no file.
func (f file) original() []byte {
	if !f.exists {
		return nil
	}
	return f.raw
}

func (f file) write(d settings.Doc) error {
	return settings.Write(f.path, settings.Doc(f.bom+string(d)), f.original())
}

func (s Setup) install(doc settings.Doc, f file) int {
	path := f.path
	if found := s.checkoutInstall(doc); len(found) > 0 {
		s.warn("Found an earlier install of sitrep (from a checkout, or the 0.3.x plugin setup) that would run beside this one:")
		for _, line := range found {
			s.warn("  - %s", line.text)
		}
		s.warn("Nothing was changed. Take it out first, then run this setup again:")
		for _, line := range found {
			if line.hint != "" {
				s.warn("  %s", line.hint)
			}
		}
		for _, r := range uniqueRoots(found) {
			if _, err := os.Stat(filepath.Join(r, "uninstall.sh")); err == nil {
				s.warn("  bash %s", show(shellWord(filepath.Join(r, "uninstall.sh"))))
			} else {
				s.warn("  %s is gone: remove its entries from ~/.claude/settings.json by hand (a backup is made by every run of this setup)", show(r))
			}
		}
		return 1
	}
	bin := s.binPath()
	for _, d := range []string{filepath.Dir(bin)} {
		if fi, err := os.Lstat(d); err == nil && !fi.IsDir() {
			s.warn("%s exists and is not a folder; leaving it alone.", show(d))
			return 1
		}
	}
	if fi, err := os.Lstat(bin); err == nil && fi.IsDir() {
		s.warn("%s is a folder; leaving it alone.", show(bin))
		return 1
	}
	if !f.exists {
		doc = settings.New()
	}
	old, ok := doc.StatusLine()
	if !ok {
		s.warn("The statusLine in %s is not an object; nothing was changed.", show(path))
		return 1
	}
	newDoc, err := doc.SetStatusLine(Command)
	if err == nil {
		err = settings.Verify(doc, newDoc, Command)
	}
	if err != nil {
		s.warn("cannot edit %s: %v; nothing was changed.", show(path), err)
		return 1
	}
	exe, err := s.executable()
	if err != nil {
		s.warn("cannot find this program: %v; nothing was changed.", err)
		return 1
	}
	var backup string
	unchanged := f.exists && string(f.raw) == f.bom+string(newDoc)
	if f.exists && !unchanged {
		if backup, err = settings.Backup(path, f.raw, s.Now()); err != nil {
			s.warn("cannot back up %s: %v; nothing was changed.", show(path), err)
			return 1
		}
	}
	if !f.exists {
		if err := os.MkdirAll(filepath.Dir(path), state.DirMode); err != nil {
			s.warn("cannot create %s: %v; nothing was changed.", show(filepath.Dir(path)), err)
			return 1
		}
	}
	if !unchanged {
		if err := f.write(newDoc); err != nil {
			s.warn("cannot write %s: %v; nothing was changed.", show(path), err)
			return 1
		}
	}
	// From here the settings are written. If the program cannot be put where the status line runs it, the settings are put back.
	st := s.store()
	if err := st.Ensure(); err == nil {
		err = s.installBinary(exe)
		if err != nil {
			s.rollback(f, newDoc)
			s.warn("Could not install the program at %s: %v. The settings were put back.", show(bin), err)
			return 1
		}
	} else {
		s.rollback(f, newDoc)
		s.warn("cannot create %s: %v. The settings were put back.", show(st.Dir), err)
		return 1
	}
	s.migrate()
	c := s.conf()
	switch old {
	case "":
		if err := c.DropHUD(); err != nil {
			s.warn("Could not update %s.", show(c.File))
		}
		s.say("No existing status line found; showing the sitrep block only.")
	case Command:
		s.say("Status line is already set up; keeping the saved upstream (if any).")
	default:
		if err := c.PutHUD(old); err != nil {
			extra := ""
			if backup != "" {
				extra = " The original is in " + show(backup) + "."
			}
			s.warn("Could not save the upstream status line to %s.%s", show(c.File), extra)
		}
		s.say("Wrapping the existing status line (it renders first): %s", show(old))
	}
	if err := c.PutExamples(); err != nil {
		s.warn("Could not add the example settings to %s: %v", show(c.File), err)
	}
	s.link()
	s.removeBuildCopy(exe)
	s.say("Set up. Restart Claude Code sessions to pick up the status line.")
	return 0
}

// migrate moves state of the old flat layout into a folder per session, now that the program that reads the new layout is in place. A hook
// that runs between the two writes the new layout, and the move merges and never overwrites, so nothing is lost.
func (s Setup) migrate() {
	r := migrate.Run(s.store(), false)
	if r.Moved+r.Merged+r.Deleted+r.Left > 0 {
		s.say("Moved the state of the old layout into ~/.claude/sitrep/sessions: %d moved, %d merged, %d left, %d removed.", r.Moved, r.Merged, r.Left, r.Deleted)
	}
	if r.Errors > 0 {
		for _, l := range r.Lines {
			if strings.HasPrefix(l, "error: ") {
				s.warn("%s", l)
			}
		}
		s.warn("Some state was not moved: run `sitrep migrate` again.")
	}
}

// BuildName is the file the setup skill builds the program into, beside the stable path. Setup copies it into place and removes it.
const BuildName = "sitrep.new"

// removeBuildCopy removes the program setup ran from when that was the skill's build copy, which has served its purpose. It names exactly
// that one file in the state folder and nothing else.
func (s Setup) removeBuildCopy(exe string) {
	named := filepath.Join(s.store().Dir, BuildName)
	if a, err := os.Stat(exe); err == nil {
		if b, err := os.Lstat(named); err == nil && b.Mode().IsRegular() && os.SameFile(a, b) {
			_ = os.Remove(named)
		}
	}
}

// rollback puts the settings back as they were read, when nothing else has written them since.
func (s Setup) rollback(f file, written settings.Doc) {
	_ = settings.Write(f.path, settings.Doc(f.raw), []byte(f.bom+string(written)))
	if !f.exists {
		_ = os.Remove(f.path)
	}
}

func (s Setup) executable() (string, error) {
	exe, err := s.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	in, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	return exe, in.Close()
}

// installBinary makes the program available at the stable path, which the status line and the hooks name. When setup already runs from there
// there is nothing to do; otherwise it copies itself in.
func (s Setup) installBinary(exe string) error {
	dst := s.binPath()
	if a, aerr := os.Stat(exe); aerr == nil {
		if b, berr := os.Stat(dst); berr == nil && os.SameFile(a, b) {
			return nil
		}
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, state.DirMode); err != nil {
		return err
	}
	if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink == 0 && fi.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(dir, fi.Mode().Perm()&^0o077) // only ever tighten, and never through a link
	}
	in, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, ".sitrep.*")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o700); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// link makes BIN_DIR/sitrep point at the program, unless something else is there.
func (s Setup) link() {
	dir, bin := s.binDir(), s.binPath()
	l := filepath.Join(dir, "sitrep")
	if _, err := os.Lstat(l); err == nil {
		if t, _ := os.Readlink(l); t != bin {
			s.warn("Note: %s exists and is not ours; left alone. Run %s directly.", show(l), show(bin))
		}
		return
	}
	if err := os.MkdirAll(dir, state.DirMode); err == nil {
		_ = os.Symlink(bin, l)
	}
}

func (s Setup) remove(doc settings.Doc, f file) int {
	if f.exists {
		backup, err := settings.Backup(f.path, f.raw, s.Now())
		if err != nil {
			s.warn("cannot back up %s: %v; nothing was changed.", show(f.path), err)
			return 1
		}
		_ = backup
		if cur, _ := doc.StatusLine(); cur == Command {
			up := s.conf().FromFile("SITREP_HUD_CMD") // what setup saved; the environment has no say here
			out, err := doc.RestoreStatusLine(up)
			if err == nil {
				err = settings.Verify(doc, out, up)
			}
			if err != nil {
				s.warn("cannot edit %s: %v; nothing was changed.", show(f.path), err)
				return 1
			}
			if err := f.write(out); err != nil {
				s.warn("cannot write %s: %v; nothing was changed.", show(f.path), err)
				return 1
			}
			if err := s.conf().DropHUD(); err != nil {
				s.warn("Could not update %s.", show(s.conf().File))
			}
			s.say("Status line restored.")
		} else {
			s.say("The status line is not sitrep's; left alone.")
		}
	}
	if err := s.conf().DropExamples(); err != nil { // the block is ours, whatever the status line is
		s.warn("Could not update %s.", show(s.conf().File))
	}
	bin := s.binPath()
	entries, _ := os.ReadDir(s.binDir())
	for _, e := range entries {
		l := filepath.Join(s.binDir(), e.Name())
		if t, err := os.Readlink(l); err == nil && t == bin {
			_ = os.Remove(l)
		}
	}
	if fi, err := os.Lstat(bin); err == nil && fi.Mode().IsRegular() {
		_ = os.Remove(bin)
		_ = os.Remove(filepath.Dir(bin)) // only when it is empty
	}
	s.say("Removed. State in ~/.claude/sitrep is left in place. Run /plugin uninstall sitrep to remove the plugin itself.")
	return 0
}

// leftover is something an install from a checkout left, and the folder of that checkout when it can be told.
type leftover struct {
	text string
	root string
	hint string // what to do, when there is no checkout to name
}

// legacyPluginCommand is the status line that setup of the 0.3.x plugin wrote.
const legacyPluginCommand = `bash "$HOME/.claude/sitrep/statusline.sh" ` + settings.Tag

func uniqueRoots(found []leftover) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range found {
		if l.root != "" && !seen[l.root] {
			seen[l.root] = true
			out = append(out, l.root)
		}
	}
	sort.Strings(out)
	return out
}

// checkoutInstall lists what an install from a checkout of the repository left behind: its hooks would run beside the plugin's. A link
// counts as ours when its target still has our layout, or is gone and was named for us, because another tool may also be called sitrep.
func (s Setup) checkoutInstall(doc settings.Doc) []leftover {
	var found []leftover
	for _, c := range doc.HookCommands() {
		if strings.Contains(c, "hooks/sitrep-hook.sh") || strings.Contains(c, "hooks/notes-hook.sh") {
			root := strings.TrimPrefix(c, "bash ")
			root, _, _ = strings.Cut(root, "/hooks/")
			found = append(found, leftover{text: "a hook in settings.json: " + show(c), root: unescape(root)})
		}
	}
	if sl, _ := doc.StatusLine(); sl == legacyPluginCommand {
		found = append(found, leftover{text: "the status line of the 0.3.x plugin setup: " + show(sl),
			hint: "restore the status line you had before sitrep (the command saved in ~/.claude/sitrep/sitrep.conf as SITREP_HUD_CMD), or delete the statusLine key"})
	} else if sl != "" && sl != Command && (settings.Ours(sl) || strings.Contains(sl, "claude-sitrep/statusline/statusline.sh")) {
		root := strings.TrimPrefix(sl, "bash ")
		root, _, _ = strings.Cut(root, "/statusline/statusline.sh")
		found = append(found, leftover{text: "the status line: " + show(sl), root: unescape(root)})
	}
	sk := filepath.Join(s.skillsDir(), "sitrep-queue")
	if t, err := os.Readlink(sk); err == nil && strings.HasSuffix(t, "/skills/sitrep-queue") {
		if _, serr := os.Stat(filepath.Join(t, "SKILL.md")); serr == nil || strings.Contains(t, "claude-sitrep") {
			found = append(found, leftover{text: fmt.Sprintf("the skill link %s -> %s", show(sk), show(t)), root: strings.TrimSuffix(t, "/skills/sitrep-queue")})
		}
	}
	entries, _ := os.ReadDir(s.binDir())
	for _, e := range entries {
		l := filepath.Join(s.binDir(), e.Name())
		t, err := os.Readlink(l)
		if err != nil || !strings.HasSuffix(t, "/bin/sitrep") || t == s.binPath() {
			continue
		}
		root := strings.TrimSuffix(t, "/bin/sitrep")
		if _, serr := os.Stat(filepath.Join(root, "hooks", "sitrep-hook.sh")); serr == nil || strings.Contains(t, "claude-sitrep") {
			found = append(found, leftover{text: fmt.Sprintf("the link %s -> %s", show(l), show(t)), root: root})
		}
	}
	return found
}

// unescape drops the backslashes a shell-quoted path has.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// shellWord quotes a path for a command line, only when it needs it.
func shellWord(p string) string {
	for _, r := range p {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_/.:@%+=,-", r)) {
			return conf.Quote(p)
		}
	}
	return p
}
