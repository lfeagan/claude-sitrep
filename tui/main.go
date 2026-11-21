// Command sitrep-tui is an optional full-screen view of sitrep: tabs for a Watch view and
// a Top view, and a status bar. It gets everything from `sitrep ls --json`, so the rules for
// what is running live in one place, the shell script. It changes nothing itself: the keys
// that change state run a fixed `sitrep` command, after a confirm, and --read-only turns them off.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// envOn says whether a switch in the environment is on: set, and not 0, false or no.
func envOn(name string) bool {
	switch strings.ToLower(os.Getenv(name)) {
	case "", "0", "false", "no":
		return false
	}
	return true
}

// envOff says whether a switch in the environment is explicitly off: 0, false or no.
func envOff(name string) bool {
	switch strings.ToLower(os.Getenv(name)) {
	case "0", "false", "no":
		return true
	}
	return false
}

func main() {
	tabName := flag.String("tab", "watch", "the tab to start on: watch or top")
	interval := flag.Duration("interval", 3*time.Second, "how often to refresh")
	once := flag.Bool("once", false, "print one frame as plain text and exit")
	readOnly := flag.Bool("read-only", envOn("SITREP_TUI_READONLY"), "turn off every key that changes state, such as p (or set SITREP_TUI_READONLY)")
	noBell := flag.Bool("no-bell", envOff("SITREP_TUI_BELL"), "do not ring the terminal bell when a session starts needing you (or set SITREP_TUI_BELL=0)")
	quitOnJump := flag.Bool("quit-on-jump", false, "leave the dashboard after a good jump (for a tmux popup)")
	width := flag.Int("width", 100, "the frame width for --once")
	flag.Parse()

	tab := tabWatch
	switch *tabName {
	case "watch":
	case "top":
		tab = tabTop
	default:
		fmt.Fprintf(os.Stderr, "sitrep-tui: unknown tab %q (watch or top)\n", *tabName)
		os.Exit(2)
	}
	if *interval < time.Second {
		*interval = time.Second
	}
	bin, err := findSitrep()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sitrep-tui:", err)
		os.Exit(1)
	}
	if *once {
		snap, err := fetch(nilContext(), bin, fetchTimeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sitrep-tui:", err)
			os.Exit(1)
		}
		fmt.Println(render(snap, tab, "", viewOpts{Width: *width, Height: 0, ShowEnded: false, Theme: plainTheme()}))
		return
	}
	m := newModel(bin, *interval, tab)
	m.quitJump, m.readOnly, m.bell = *quitOnJump, *readOnly, !*noBell
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sitrep-tui:", err)
		os.Exit(1)
	}
}
