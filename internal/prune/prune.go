// Package prune finds sessions that are over, marks them ended, and says which hold queue items. It never touches a session that is
// alive or too new to judge, and it never deletes a queue.
package prune

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// Action is one session that is over.
type Action struct {
	ID    state.ID
	Dir   textsafe.Text // the last part of its working directory, for the user
	Mark  bool          // it has not been marked ended yet
	Items int           // queue items it still holds
}

// Pruner works on a state folder and the live-session registry.
type Pruner struct {
	Store     state.Store
	ConfigDir string
	Reg       live.Registry
}

// NoRegistryError says the registry could not be read, so nothing can be judged.
type NoRegistryError struct{ Dir string }

func (e *NoRegistryError) Error() string {
	return fmt.Sprintf("cannot read the live-session registry in %s; nothing changed", textsafe.Clean(e.Dir, 300))
}

// Plan lists the sessions that are over, at time now (seconds since the epoch).
func (p Pruner) Plan(ctx context.Context, now int64) ([]Action, error) {
	set := p.Reg.Read(ctx)
	if !set.OK {
		return nil, &NoRegistryError{Dir: filepath.Join(p.ConfigDir, "sessions")}
	}
	entries, err := os.ReadDir(p.Store.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	q := queue.Queue{Store: p.Store}
	var out []Action
	for _, e := range entries {
		sid, ok := strings.CutSuffix(e.Name(), ".txt")
		if !ok || !state.SessionID(sid) {
			continue
		}
		if !state.IsRegular(filepath.Join(p.Store.Dir, e.Name())) {
			continue
		}
		changed, ok := live.Changed(p.Store, sid)
		word := live.Word(p.Store, sid)
		if !ok || !live.Dead(set, sid, word, changed, now) {
			continue // alive, or too new to judge
		}
		id := state.ID(sid)
		out = append(out, Action{ID: id, Dir: dirName(p.Store, id), Mark: word != "ended", Items: q.Count(id)})
	}
	return out, nil
}

// dirName is the last part of the session's working directory, or ?.
func dirName(s state.Store, id state.ID) textsafe.Text {
	b, _ := state.ReadFile(s.Path(id, "cwd"), 64<<10)
	cwd := textsafe.Clean(string(b), 200).String()
	if cwd == "" {
		cwd = "?"
	}
	return textsafe.Clean(filepath.Base(cwd), 0)
}

// MarkEnded records that a session is over.
func (p Pruner) MarkEnded(id state.ID) error {
	return p.Store.WriteFile(p.Store.Hidden(id, "state"), []byte("ended\n"))
}
