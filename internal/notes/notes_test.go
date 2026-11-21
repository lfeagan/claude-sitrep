package notes

import (
	"os"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

func TestPinAndUnpin(t *testing.T) {
	s := state.Open(t.TempDir())
	n := Notes{Store: s}
	if err := n.Pin("s1", textsafe.Clean("   ", 0)); err != ErrBlank {
		t.Errorf("blank pin: %v", err)
	}
	if err := n.Pin("s1", textsafe.Clean("ship it", 0)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Path("s1", "txt")); string(b) != "Objective: ship it\n" {
		t.Errorf("new file %q", b)
	}
	if err := os.WriteFile(s.Path("s1", "txt"), []byte("Objective: old\nProgress:  p\nNow:       n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := n.Pin("s1", textsafe.Clean("new goal", 0)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Path("s1", "txt")); string(b) != "Objective: new goal\nProgress:  p\nNow:       n\n" {
		t.Errorf("file %q", b)
	}
	if fi, err := os.Stat(s.Hidden("s1", "pinned")); err != nil || fi.Mode().Perm() != state.FileMode {
		t.Errorf("pinned file: %v", err)
	}
	if err := n.Unpin("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Hidden("s1", "pinned")); !os.IsNotExist(err) {
		t.Errorf("still pinned: %v", err)
	}
	if err := n.Unpin("s1"); err != nil {
		t.Errorf("unpin twice: %v", err)
	}
}
