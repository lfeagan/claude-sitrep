package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteThemeShots writes frames of the dashboard in every theme, with their colour codes, for a script that turns them into pictures
// (to look at a theme, or to show it to a reviewer). It does nothing unless SITREP_SHOTS_DIR names a folder, and
// SITREP_SHOTS_THEMES (a comma-separated list) picks the themes, and SITREP_SHOTS_COLORS is a SITREP_COLORS value applied on top, to try
// a palette without editing the theme.
func TestWriteThemeShots(t *testing.T) {
	dir := os.Getenv("SITREP_SHOTS_DIR")
	if dir == "" {
		t.Skip("SITREP_SHOTS_DIR is not set")
	}
	names := ThemeNames
	if v := os.Getenv("SITREP_SHOTS_THEMES"); v != "" {
		names = strings.Split(v, ",")
	}
	s := wallSnap(6)
	s.Sessions[2].AttnFaded = true // waited 40 minutes
	s.Sessions[5].Idle, s.Sessions[5].StateLabel, s.Sessions[5].State, s.Sessions[5].NeedsInput = true, "idle? 25m", "working", false
	for _, name := range names {
		for _, dark := range []bool{true, false} {
			th := themeFor(dark, Settings{Theme: name, Colors: os.Getenv("SITREP_SHOTS_COLORS")})
			table := render(s, tabTable, "error: a sample failure (showing data from 12s ago)", viewOpts{Width: 100, Height: 10, Error: true, Theme: th})
			wall := render(s, tabWall, "", viewOpts{Width: 100, Height: 8, Size: wallCompact, Theme: th})
			side := map[bool]string{true: "dark", false: "light"}[dark]
			body := table + "\n=====\n" + wall + "\n"
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s.ans", name, side)), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
