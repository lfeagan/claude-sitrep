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

// Open names the state folder, always ~/.claude/sitrep, whatever CLAUDE_CONFIG_DIR says. It creates nothing.
func Open(home string) Store { return Store{Dir: filepath.Join(home, ".claude", "sitrep")} }

// Ensure creates the folder if needed and makes it owner-only.
func (s Store) Ensure() error {
	if err := os.MkdirAll(s.Dir, DirMode); err != nil {
		return err
	}
	if fi, err := os.Lstat(s.Dir); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		return err // a link is the user's: never chmod through it
	}
	return os.Chmod(s.Dir, DirMode)
}

// Path is the file for a session id and an extension without the dot: <id>.txt, <id>.queue, <id>.cwd.
func (s Store) Path(id ID, ext string) string { return filepath.Join(s.Dir, string(id)+"."+ext) }

// Hidden is the dot file kept beside them: .<id>.state, .<id>.count, and so on.
func (s Store) Hidden(id ID, ext string) string { return filepath.Join(s.Dir, "."+string(id)+"."+ext) }

// WriteFile replaces path with data in one step: a reader sees the old file or the new one, never a half. The temporary file has a
// unique name in the same folder and is removed if anything fails.
func (s Store) WriteFile(path string, data []byte) error {
	if filepath.Dir(path) != s.Dir {
		return fmt.Errorf("%s is not in the state folder", path)
	}
	f, err := os.CreateTemp(s.Dir, ".sitrep.*")
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

// Resolve turns a unique prefix of a session id into the full id. "inbox" matches exactly. The error text is safe to print.
//
// It is stricter than the bash it replaces: only files named <valid id>.txt count, so inbox.txt and a.b.txt never match by prefix.
func (s Store) Resolve(prefix string) (ID, error) {
	id, err := ParseID(prefix)
	if err != nil {
		return "", err
	}
	if id == Inbox {
		return Inbox, nil
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var m []ID
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".txt")
		if !ok || !SessionID(id) || !strings.HasPrefix(id, prefix) {
			continue
		}
		if fi, err := os.Stat(filepath.Join(s.Dir, e.Name())); err != nil || !fi.Mode().IsRegular() {
			continue
		}
		m = append(m, ID(id))
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
