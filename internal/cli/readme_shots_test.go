package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
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
	write("wall", "dark", "# sitrep dash --demo\n"+frame("wall", 100, 12, "", true, "", true))
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
	write("cvd", "dark", "# cvd-safe\n"+frame("table", 84, 9, "cvd-safe", true, sel, false))
}
