// Package statetest helps tests write session files by the names of the old flat layout (<id>.txt, .<id>.state), which many of them are
// written in, into the session folders of the layout in use.
package statetest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

// Path is where a file of the old layout goes now. A name that is not a session's file (inbox.queue, a bad name) stays in the root.
func Path(s state.Store, name string) string {
	if l, ok := state.ParseLegacy(name); ok && !l.Obsolete {
		return s.Path(l.ID, l.File)
	}
	return filepath.Join(s.Dir, name)
}

// Put writes a file named as in the old layout, makes its folder, and sets its modification time when age is not zero (age ago, from now).
func Put(t testing.TB, s state.Store, name, body string, age time.Duration) string {
	t.Helper()
	p := Path(s, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if age != 0 {
		at := time.Now().Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// PutAt is Put with a fixed modification time.
func PutAt(t testing.TB, s state.Store, name, body string, at time.Time) string {
	t.Helper()
	p := Put(t, s, name, body, 0)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
	return p
}
