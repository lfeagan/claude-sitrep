// Command sitrep-tui is an optional full-screen view of sitrep: tabs for a Watch view and
// a Top view, and a status bar. It is read-only. It gets everything from `sitrep ls --json`,
// so the rules for what is running live in one place, the shell script.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

func main() {
	tabName := flag.String("tab", "watch", "the tab to start on: watch or top")
	interval := flag.Duration("interval", 3*time.Second, "how often to refresh")
	once := flag.Bool("once", false, "print one frame as plain text and exit")
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
	if _, err := tea.NewProgram(newModel(bin, *interval, tab)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sitrep-tui:", err)
		os.Exit(1)
	}
}
