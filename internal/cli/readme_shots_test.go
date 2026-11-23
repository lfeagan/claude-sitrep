package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/statusline"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/tui"
)

// TestWriteReadmeShots writes the frames of the pictures in the README, from the made-up demo sessions, for docs/theme-shots/readme_shots.py.
// It does nothing unless SITREP_README_DIR names a folder; `make screenshots` runs it.
func TestWriteReadmeShots(t *testing.T) {
	dir := os.Getenv("SITREP_README_DIR")
	if dir == "" {
		t.Skip("SITREP_README_DIR is not set")
	}
	snap := demoSnapshot(1_800_000_000)
	sel := string(snap.Sessions[0].ID)
	write := func(name, side string, frames ...string) {
		t.Helper()
		body := strings.Join(frames, "\n=====\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s.ans", name, side)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	frame := func(tab string, w, h int, theme string, dark bool, sel string, demo bool) string {
		t.Helper()
		out, err := tui.ShotFrame(snap, tab, w, h, theme, dark, sel, demo)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// the hero shows four sessions, so that each card has room for its Objective, Progress, Now and queue
	full := snap
	snap.Sessions = []snapshot.Session{full.Sessions[0], full.Sessions[1], full.Sessions[2], full.Sessions[6]}
	write("hero-cards", "dark", "# sitrep dash --demo\n"+frame("cards", 100, 30, "", true, sel, true))
	snap = full
	write("table", "dark", "# sitrep dash --demo\n"+frame("table", 100, 14, "", true, sel, true))
	write("wall", "dark", "# sitrep dash --demo\n"+frame("wall", 66, 12, "", true, "", true))
	// a theme is shown on four sessions, one of each look: needs you, recent activity, plain and snoozed
	snap.Sessions = []snapshot.Session{full.Sessions[0], full.Sessions[2], full.Sessions[3], full.Sessions[6]}
	theme := func(side string, dark bool, names ...string) {
		var frames []string
		for _, n := range names {
			frames = append(frames, "# "+n+"\n"+frame("table", 84, 9, n, dark, sel, false))
		}
		write("themes", side, frames...)
	}
	theme("dark", true, "eighties", "homebrew", "nord", "tokyo", "volcano", "gameboy")
	theme("light", false, "paper", "novel", "bossa-nova", "arctic")
	// the status line under a prompt, drawn by the program itself from the notes and queue of a demo session (c9d4)
	write("statusline", "dark", "# the status line under each prompt\n"+statusLineFrame(t, demoSessions[2]))
	write("cvd", "dark", "# cvd-safe\n"+frame("table", 84, 9, "cvd-safe", true, sel, false))
}

// statusLineFrame writes the notes and queues of one demo session in a throwaway state folder and returns what `sitrep statusline` draws for it.
func statusLineFrame(t *testing.T, d demoSession) string {
	t.Helper()
	st := state.Open(t.TempDir())
	if err := st.Ensure(); err != nil {
		t.Fatal(err)
	}
	id := demoID(d.id)
	if err := st.EnsureSession(id); err != nil {
		t.Fatal(err)
	}
	notes := "Objective: " + d.objective + "\nProgress:  " + d.progress + "\nNow:       " + d.now + "\n"
	if err := st.WriteFile(st.Path(id, "notes"), []byte(notes)); err != nil {
		t.Fatal(err)
	}
	q := queue.Queue{Store: st}
	for _, it := range d.queue {
		if _, err := q.Add(id, textsafe.Clean(it, 0)); err != nil {
			t.Fatal(err)
		}
	}
	for _, it := range []string{"look at the flaky test in CI", "ask about the release date"} {
		if _, err := q.Add(state.Inbox, textsafe.Clean(it, 0)); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	line := statusline.Line{Store: st, Conf: conf.Conf{File: filepath.Join(st.Dir, "sitrep.conf"), Getenv: func(string) string { return "" }}, Run: &runner.Fake{}}
	line.Render(context.Background(), strings.NewReader(`{"session_id":"`+d.id+`"}`), &out)
	return strings.TrimRight(out.String(), "\n")
}
