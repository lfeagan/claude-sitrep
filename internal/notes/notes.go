// Package notes is the per-session text file (Objective, Progress, Now) and the pin that keeps the summarizer from rewriting it.
package notes

import (
	"errors"
	"os"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// ErrBlank says there is no text to pin.
var ErrBlank = errors.New("nothing to pin")

// Notes is the set of session files in a state folder.
type Notes struct{ Store state.Store }

// Pin sets the Objective and stops the summarizer from changing it. The other lines of the file stay as they are.
func (n Notes) Pin(id string, text textsafe.Text) error {
	if strings.TrimSpace(string(text)) == "" {
		return ErrBlank
	}
	if err := n.Store.Ensure(); err != nil {
		return err
	}
	var keep []string
	if b, err := os.ReadFile(n.Store.Path(id, "txt")); err == nil {
		for _, l := range strings.SplitAfter(string(b), "\n") {
			if l != "" && !strings.HasPrefix(l, "Objective:") {
				keep = append(keep, l)
			}
		}
	}
	body := "Objective: " + string(text) + "\n" + strings.Join(keep, "")
	if err := n.Store.WriteFile(n.Store.Path(id, "txt"), []byte(body)); err != nil {
		return err
	}
	f, err := os.OpenFile(n.Store.Hidden(id, "pinned"), os.O_WRONLY|os.O_CREATE, state.FileMode)
	if err != nil {
		return err
	}
	return f.Close()
}

// Unpin lets the summarizer rewrite the Objective again.
func (n Notes) Unpin(id string) error {
	err := os.Remove(n.Store.Hidden(id, "pinned"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
