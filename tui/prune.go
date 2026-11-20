package main

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// The dashboard never changes state itself. Prune runs a fixed `sitrep prune` command, after
// a preview that the user confirms.

const (
	previewTimeout = 10 * time.Second
	pruneTimeout   = 15 * time.Second
	maxPruneLines  = 40
	maxPruneBytes  = 8 << 10
)

// pruneArgs are the only prune commands the dashboard runs.
var (
	pruneArgsPreview = []string{"prune", "--dry-run"}
	pruneArgsMove    = []string{"prune", "-y"}
	pruneArgsKeep    = []string{"prune", "--keep"}
)

// Phases of the prune screen.
const (
	pruneNone    = iota
	pruneLoading // the preview is running
	pruneConfirm // the preview is shown, waiting for y, n or esc
	pruneBusy    // the real prune is running
)

// pruneMsg is the answer to a prune command.
type pruneMsg struct {
	real bool // false for the preview
	text string
	err  error
}

// pruneLines cleans the output of a prune command and caps how much of it is kept.
func pruneLines(out string) []string {
	if len(out) > maxPruneBytes {
		out = out[:maxPruneBytes]
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = clean(l); l != "" {
			lines = append(lines, l)
		}
		if len(lines) == maxPruneLines {
			break
		}
	}
	return lines
}

// nothingToPrune is true when a preview holds no "would ..." line: no session to mark, and no
// queue to ask about.
func nothingToPrune(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "would ") && !strings.HasPrefix(l, "would mark 0 ended") {
			return false
		}
	}
	return true
}

// runPrune runs one fixed prune command with its own timeout.
func runPrune(bin string, args []string, timeout time.Duration, real bool, run runner) func() pruneMsg {
	return func() pruneMsg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		out, err := run(ctx, bin, args...)
		return pruneMsg{real: real, text: out, err: err}
	}
}

// pruneScreen is the preview and the question. A running prune shows no keys, because none work.
func pruneScreen(lines []string, phase, width, height int, th Theme) string {
	body := []string{"Prune the sessions that are gone", ""}
	switch phase {
	case pruneLoading:
		body = append(body, "looking…")
	case pruneBusy:
		body = append(body, "working…")
	default:
		for _, l := range lines {
			body = append(body, "  "+l)
		}
		body = append(body, "",
			"  y   mark them ended, and move their queue items to the inbox",
			"  n   mark them ended, and leave the items where they are",
			"  esc cancel")
	}
	for i, l := range body {
		body[i] = ansi.Truncate(l, max(width, 1), "…")
	}
	if height > 0 {
		body = body[:min(len(body), max(height-1, 0))]
		for len(body) < height-1 {
			body = append(body, "")
		}
	}
	hint := " y yes  n keep items  esc cancel "
	if phase != pruneConfirm {
		hint = " prune "
	}
	bar := th.Status.Width(max(width, 1)).Render(ansi.Truncate(hint, max(width, 1), ""))
	if height == 1 {
		return bar
	}
	return strings.Join(append(body, bar), "\n")
}
