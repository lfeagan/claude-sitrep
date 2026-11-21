// Package state is where sitrep keeps its files: the folder, the session ids that name them, and the way they are written.
package state

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// Modes for everything sitrep writes (invariant 2): state holds prompt text, so it is owner-only. A Umask call does not reach in-process
// tests, so the modes are always explicit.
const (
	FileMode os.FileMode = 0o600
	DirMode  os.FileMode = 0o700
)

// Inbox is the queue of no session. It is reserved: no session id may be this.
const Inbox ID = "inbox"

// ValidID reports whether s can be part of a file name: letters, digits, underscore and hyphen, and nothing else (invariant 3).
func ValidID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// SessionID is ValidID for a real session: the inbox is not one.
func SessionID(s string) bool { return ValidID(s) && s != string(Inbox) }

// ID names a queue: a session id, or the inbox. It can only be made by ParseID or Resolve, so a path built from an ID cannot leave the
// state folder (invariant 3).
type ID string

// ParseID checks that s is a valid id (the inbox included).
func ParseID(s string) (ID, error) {
	if !ValidID(s) {
		return "", fmt.Errorf("bad id '%s'", textsafe.Clean(s, 200))
	}
	return ID(s), nil
}

// ConfigDir is Claude Code's own folder. Only the transcripts and the live-session registry honor CLAUDE_CONFIG_DIR; sitrep's own state
// never moves (see Open), and unifying the two would orphan existing state.
func ConfigDir(getenv func(string) string, home string) string {
	if d := getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(home, ".claude")
}

// Store is the state folder.
type Store struct{ Dir string }

// Open names the default state folder, ~/.claude/sitrep, whatever CLAUDE_CONFIG_DIR says. It creates nothing.
func Open(home string) Store { return Store{Dir: filepath.Join(home, ".claude", "sitrep")} }

// EnvDir is the variable that moves the state folder.
const EnvDir = "SITREP_STATE_DIR"

// Locate names the state folder: the path in SITREP_STATE_DIR when it is set, or else the default. Only the state moves. The program, which
// the hooks run from ~/.claude/sitrep/bin, stays where setup put it. A value that is not an absolute path is refused, and never read as
// relative to wherever the program happens to run, so that a hook cannot write state somewhere by accident. So is the home folder, Claude's
// own folder, the registry and the transcripts in it, and any folder above them: the state is made owner-only and fills the folder with its
// own files, so it needs one of its own.
func Locate(home string, getenv func(string) string) (Store, error) {
	d := ""
	if getenv != nil {
		d = getenv(EnvDir)
	}
	if d == "" {
		return Open(home), nil
	}
	if !filepath.IsAbs(d) || strings.ContainsRune(d, 0) || sharedFolder(filepath.Clean(d), home, ConfigDir(getenv, home)) {
		return Store{}, fmt.Errorf("%s must be an absolute path to a folder of its own, not '%s'", EnvDir, textsafe.Clean(d, 200))
	}
	return Store{Dir: filepath.Clean(d)}, nil
}

// sharedFolder says whether dir is, or holds, a folder that belongs to the user or to Claude Code.
func sharedFolder(dir, home, config string) bool {
	for _, p := range []string{home, config, filepath.Join(config, "sessions"), filepath.Join(config, "projects")} {
		if rel, err := filepath.Rel(dir, filepath.Clean(p)); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true // p is dir, or below it
		}
	}
	return false
}

// Ensure creates the folder if needed and makes it owner-only. A folder that exists, such as one chosen with SITREP_STATE_DIR, only loses
// its group and other bits: its mode is never widened (invariant 5).
func (s Store) Ensure() error {
	if err := os.MkdirAll(s.Dir, DirMode); err != nil {
		return err
	}
	fi, err := os.Lstat(s.Dir)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return err // a link is the user's: never chmod through it
	}
	if mode := fi.Mode().Perm(); mode&^DirMode != 0 {
		return os.Chmod(s.Dir, mode&DirMode)
	}
	return nil
}

// SessionsDir is the folder that holds one folder per session.
func (s Store) SessionsDir() string { return filepath.Join(s.Dir, "sessions") }

// SessionDir is the folder of one session. The inbox is no session, and lives in the root.
func (s Store) SessionDir(id ID) string {
	if id == Inbox {
		return s.Dir
	}
	return filepath.Join(s.SessionsDir(), string(id))
}

// Path is the file of a session: notes, queue, cwd, state, count, stamp, pending, lock, pinned or snooze. The inbox has only its queue, in
// the root as inbox.queue.
func (s Store) Path(id ID, name string) string {
	if id == Inbox {
		return filepath.Join(s.Dir, "inbox."+name)
	}
	return filepath.Join(s.SessionDir(id), name)
}

// EnsureSession makes the folder of a session, owner-only, and the folder of sessions above it. Whatever writes first makes it.
func (s Store) EnsureSession(id ID) error {
	if id == Inbox {
		return s.Ensure()
	}
	if err := os.MkdirAll(s.SessionsDir(), DirMode); err != nil {
		return err
	}
	if err := os.Mkdir(s.SessionDir(id), DirMode); err != nil && !os.IsExist(err) {
		return err
	}
	// a link, or something that is no folder, is never written through
	for _, d := range []string{s.SessionsDir(), s.SessionDir(id)} {
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
			return fmt.Errorf("%s is not a folder", filepath.Base(d))
		}
	}
	return nil
}

// Sessions lists the sessions: the folders of the sessions folder that have a valid id and hold a regular file notes. A link is not followed,
// and a folder without notes is not a session.
func (s Store) Sessions() []ID {
	entries, _ := os.ReadDir(s.SessionsDir())
	var out []ID
	for _, e := range entries {
		if !e.Type().IsDir() || !SessionID(e.Name()) {
			continue
		}
		id := ID(e.Name())
		if IsRegular(s.Path(id, "notes")) {
			out = append(out, id)
		}
	}
	return out
}

// WriteFile replaces path with data in one step: a reader sees the old file or the new one, never a half. The path must be in the state folder
// or in the folder of a session (made if need be). The temporary file has a unique name in the same folder and is removed if anything fails.
func (s Store) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	switch {
	case dir == s.Dir:
	case filepath.Dir(dir) == s.SessionsDir() && SessionID(filepath.Base(dir)):
		if err := s.EnsureSession(ID(filepath.Base(dir))); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s is not in the state folder", path)
	}
	f, err := os.CreateTemp(dir, ".sitrep.*")
	if err != nil {
		return err
	}
	name := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(name)
		return err
	}
	if err := f.Chmod(FileMode); err != nil {
		return fail(err)
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Resolve turns a unique prefix of a session id, or an alias, into the full id. "inbox" matches exactly. The error text is safe to print.
//
// It is stricter than the bash it replaces: only session folders with a valid id count.
func (s Store) Resolve(prefix string) (ID, error) {
	id, err := ParseID(prefix)
	if err != nil {
		return "", err
	}
	if id == Inbox {
		return Inbox, nil
	}
	var m []ID
	for _, c := range s.Sessions() {
		if strings.HasPrefix(string(c), prefix) {
			m = append(m, c)
		}
	}
	if len(m) == 0 && ValidAlias(prefix) { // an id beats an alias, so only a word that starts no id is looked up as one
		for _, c := range s.Sessions() {
			if s.Alias(c) == prefix {
				m = append(m, c)
			}
		}
		if len(m) > 1 {
			return "", fmt.Errorf("alias '%s' is held by %d sessions; run 'sitrep ls'", prefix, len(m))
		}
	}
	if len(m) != 1 {
		return "", fmt.Errorf("id '%s' matches %d; run 'sitrep ls'", prefix, len(m))
	}
	return m[0], nil
}

// ReadFile reads at most max bytes of a regular file, and nothing else: a pipe, a device or a directory is an error, and opening one never
// blocks (invariant 4: a hook must not hang on a hostile file). A link to a regular file is followed, as the bash tests did.
func ReadFile(path string, max int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	return io.ReadAll(io.LimitReader(f, max))
}

// FirstLine is the first line of a small regular file, without leading or trailing spaces and tabs, as the shell's read -r gives it. It is
// empty when the file cannot be read.
func FirstLine(path string) string {
	b, err := ReadFile(path, 4096)
	if err != nil {
		return ""
	}
	l, _, _ := strings.Cut(string(b), "\n")
	return strings.Trim(l, " \t")
}

// IsRegular says whether path is a regular file (a link to one counts).
func IsRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// ErrBusy says a lock could not be taken in time.
var ErrBusy = errors.New("busy: another sitrep is changing the same files")

// Lock takes an exclusive lock on a file (made if needed), waiting a moment for another holder, and gives up rather than hang a prompt. The
// returned function releases it. Locks are advisory: every writer of what the lock guards takes it.
func Lock(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, FileMode)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 100; i++ {
		if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.Close()
	return nil, ErrBusy
}

// TryLock is Lock without the wait: one attempt, and ok is false when another holder has the lock. A hook uses it for work it can skip.
func TryLock(path string) (unlock func(), ok bool) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, FileMode)
	if err != nil {
		return nil, false
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, false
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, true
}
