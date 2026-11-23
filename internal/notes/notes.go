// Package notes is the per-session text file (Objective, Progress, Now) and the pin that keeps the summarizer from rewriting it.
package notes

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// ErrBlank says there is no text to pin.
var ErrBlank = errors.New("nothing to pin")

// Notes is the set of session files in a state folder.
type Notes struct{ Store state.Store }

// Pin sets the Objective and stops the summarizer from changing it. The other lines of the file stay as they are.
func (n Notes) Pin(id state.ID, text textsafe.Text) error {
	if strings.TrimSpace(text.String()) == "" {
		return ErrBlank
	}
	if err := n.Store.Ensure(); err != nil {
		return err
	}
	var keep []string
	if b, err := state.ReadFile(n.Store.Path(id, "notes"), 1<<20); err == nil {
		for _, l := range strings.SplitAfter(string(b), "\n") {
			if l != "" && !strings.HasPrefix(l, "Objective:") {
				keep = append(keep, l)
			}
		}
	}
	body := "Objective: " + text.String() + "\n" + strings.Join(keep, "")
	if err := n.Store.WriteFile(n.Store.Path(id, "notes"), []byte(body)); err != nil {
		return err
	}
	if err := n.Store.EnsureSession(id); err != nil {
		return err
	}
	f, err := state.OpenRegular(n.Store.Path(id, "pinned"), os.O_WRONLY|os.O_CREATE, state.FileMode)
	if err != nil {
		return err
	}
	return f.Close()
}

// Unpin lets the summarizer rewrite the Objective again.
func (n Notes) Unpin(id state.ID) error {
	err := os.Remove(n.Store.Path(id, "pinned"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Snooze mutes the needs-you state of a session until its state next changes: it stores when the state last changed, and the snapshot
// treats the mute as over once that time is no longer the one stored.
func (n Notes) Snooze(id state.ID, changed int64) error {
	if err := n.Store.Ensure(); err != nil {
		return err
	}
	return n.Store.WriteFile(n.Store.Path(id, "snooze"), []byte(strconv.FormatInt(changed, 10)+"\n"))
}

// Unsnooze ends the mute at once.
func (n Notes) Unsnooze(id state.ID) error {
	err := os.Remove(n.Store.Path(id, "snooze"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Get is the value of the first "<key>:" line of the session's notes, without the spaces after the colon; empty when there is none. The text
// is raw: it came from a file, so it goes through textsafe before it is shown or stored again.
func (n Notes) Get(id state.ID, key string) string {
	b, err := state.ReadFile(n.Store.Path(id, "notes"), 1<<20)
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.TrimLeft(rest, " ")
		}
	}
	return ""
}

// Write replaces the notes with the three lines the hook keeps.
func (n Notes) Write(id state.ID, objective, progress, now textsafe.Text) error {
	if err := n.Store.Ensure(); err != nil {
		return err
	}
	body := "Objective: " + objective.String() + "\nProgress:  " + progress.String() + "\nNow:       " + now.String() + "\n"
	return n.Store.WriteFile(n.Store.Path(id, "notes"), []byte(body))
}
