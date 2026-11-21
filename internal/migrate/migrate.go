// Package migrate moves the state of the old flat layout (0.4 and before: <id>.txt, .<id>.state and so on, all in the state folder) into
// one folder per session. `sitrep setup` runs it after it has installed the program, and the user can run it with `sitrep migrate`. It is
// never called from a hook, and it is safe to run again and to stop at any point.
package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// Result says what a run did, one line per file, and how many.
type Result struct {
	Lines   []string
	Moved   int
	Merged  int
	Left    int
	Deleted int
	Errors  int
}

type item struct {
	name string
	l    state.Legacy
}

// Pending counts the files of the old layout that are still in the root of the state folder, for a note that tells the user to migrate.
func Pending(s state.Store) int {
	n := 0
	for _, it := range found(s) {
		if !it.l.Obsolete {
			n++
		}
	}
	return n
}

// found lists the regular files in the root whose names are those of the old layout: bin, sitrep.conf, inbox.queue, the folder's own lock,
// links, folders, pipes and names it does not know are not among them.
func found(s state.Store) []item {
	entries, _ := os.ReadDir(s.Dir)
	var items []item
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if l, ok := state.ParseLegacy(e.Name()); ok {
			items = append(items, item{e.Name(), l})
		}
	}
	return items
}

// Run moves every file of the old layout it finds. A file is moved by a hard link into the session's folder and removal of the old name, so
// nothing is ever overwritten (a rename would replace a file a new hook wrote a moment ago), the modification time that dates a state is
// kept, and a run that is stopped halfway is finished by the next one. A queue that exists in both places is merged, the old items first.
// Any other file whose destination exists stays where it is and is reported. The notes of a session move last, because a session exists
// only when its notes do: one that is moved in part is not listed in part. With dry nothing changes.
func Run(s state.Store, dry bool) Result {
	var r Result
	if fi, err := os.Lstat(s.SessionsDir()); err == nil && !fi.IsDir() {
		r.fail("sessions is not a folder (a link or a file): nothing was moved")
		return r
	}
	if _, err := os.Stat(s.Dir); err != nil {
		if !os.IsNotExist(err) {
			r.fail("cannot read the state folder: %v", err)
		}
		return r
	}
	items := found(s)
	sort.SliceStable(items, func(i, j int) bool { return items[i].l.File != "notes" && items[j].l.File == "notes" })
	if !dry && len(items) > 0 {
		unlock, err := state.Lock(filepath.Join(s.Dir, ".queue.lock")) // the lock queue changes take, so none is half done under us
		if err != nil {
			r.fail("cannot take the queue lock: %v", err)
			return r
		}
		defer unlock()
	}
	for _, it := range items {
		src := filepath.Join(s.Dir, it.name)
		if it.l.Obsolete {
			r.remove(src, it.name, dry)
			continue
		}
		r.move(s, src, it, dry)
	}
	return r
}

// remove deletes a file that nothing reads, by its own name.
func (r *Result) remove(src, name string, dry bool) {
	if !dry {
		if err := os.Remove(src); err != nil {
			r.fail("%s: %v", name, err)
			return
		}
	}
	r.line(verb(dry, "deleted", "would delete")+" %s (nothing reads it)", name)
	r.Deleted++
}

func verb(dry bool, done, would string) string {
	if dry {
		return would
	}
	return done
}

func (r *Result) move(s state.Store, src string, it item, dry bool) {
	dest := s.Path(it.l.ID, it.l.File)
	rel := filepath.Join("sessions", string(it.l.ID), it.l.File)
	f, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		r.fail("%s: %v", it.name, err)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		r.fail("%s: not a regular file", it.name)
		return
	}
	if d, err := os.Lstat(dest); err == nil {
		switch {
		case os.SameFile(fi, d): // an earlier run linked it and stopped before it removed the old name
			if !dry {
				if err := os.Remove(src); err != nil {
					r.fail("%s: %v", it.name, err)
					return
				}
			}
			r.line(verb(dry, "moved", "would move")+" %s -> %s", it.name, rel)
			r.Moved++
		case it.l.File == "queue":
			r.merge(s, src, dest, it.name, rel, dry)
		default:
			r.line("left %s: %s exists", it.name, rel)
			r.Left++
		}
		return
	}
	if dry {
		r.line("would move %s -> %s", it.name, rel)
		r.Moved++
		return
	}
	if fi.Mode().Perm()&0o077 != 0 {
		_ = f.Chmod(fi.Mode().Perm() &^ 0o077) // state is owner-only; the descriptor is of the file itself, never of a link
	}
	if err := s.EnsureSession(it.l.ID); err != nil {
		r.fail("%s: %v", it.name, err)
		return
	}
	if err := os.Link(src, dest); err != nil {
		if errors.Is(err, os.ErrExist) { // a hook wrote it between our look and our link: next time it is a conflict, or a merge
			r.line("left %s: %s exists", it.name, rel)
			r.Left++
			return
		}
		r.fail("%s: %v (it was not copied, and stays where it is)", it.name, err)
		return
	}
	if d, err := os.Lstat(dest); err != nil || !os.SameFile(fi, d) { // on some systems link follows a link: it was swapped for one
		_ = os.Remove(dest)
		r.fail("%s: changed while it was moved", it.name)
		return
	}
	if err := os.Remove(src); err != nil {
		r.fail("%s: %v", it.name, err)
		return
	}
	r.line("moved %s -> %s", it.name, rel)
	r.Moved++
}

// merge puts the items of an old queue in front of those of the queue that is already in place, and removes the old file.
func (r *Result) merge(s state.Store, src, dest, name, rel string, dry bool) {
	if dry {
		r.line("would merge %s into %s", name, rel)
		r.Merged++
		return
	}
	old, err1 := state.ReadFile(src, 8<<20)
	cur, err2 := state.ReadFile(dest, 8<<20)
	if err1 != nil || err2 != nil {
		r.fail("%s: cannot read a queue to merge", name)
		return
	}
	if len(old) > 0 && old[len(old)-1] != '\n' {
		old = append(old, '\n')
	}
	if err := s.WriteFile(dest, append(old, cur...)); err != nil {
		r.fail("%s: %v", name, err)
		return
	}
	if err := os.Remove(src); err != nil {
		r.fail("%s: %v", name, err)
		return
	}
	r.line("merged %s into %s", name, rel)
	r.Merged++
}

func (r *Result) line(format string, a ...any) {
	r.Lines = append(r.Lines, textsafe.Strip(fmt.Sprintf(format, a...)).String())
}

func (r *Result) fail(format string, a ...any) {
	r.line("error: "+format, a...)
	r.Errors++
}
