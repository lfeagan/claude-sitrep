package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const detailLabel = 10 // the width of the label column

// detailLines lays out one session in full: nothing is cut, long text is wrapped to width,
// and the whole queue is listed.
func detailLines(x Session, width int) []string {
	avail := max(width-detailLabel-2, 10)
	var out []string
	row := func(label, text string) {
		for i, l := range wrapText(text, avail, 1<<20) {
			prefix := strings.Repeat(" ", detailLabel+2)
			if i == 0 {
				prefix = "  " + padRight(label, detailLabel)
			}
			out = append(out, prefix+l)
		}
	}
	state := x.StateLabel
	if x.NeedsInput && x.Running {
		state = "● " + state + "  (waiting for you)"
	}
	proc := "not known here"
	if x.PID != nil {
		proc = fmt.Sprintf("pid %d", *x.PID)
	}
	row("Session", x.ID)
	row("Directory", x.Cwd)
	row("State", state)
	row("Process", proc)
	if x.QuietSecs != nil {
		row("Quiet", fmt.Sprintf("%s", agoText(*x.QuietSecs)))
	}
	if x.Pinned {
		row("Pinned", "yes: the Objective is kept as set")
	}
	out = append(out, "")
	row("Objective", x.Objective)
	row("Progress", x.Progress)
	row("Now", x.Now)
	out = append(out, "")
	switch {
	case x.Queue.Count == 0:
		row("Queue", "empty")
	case len(x.Queue.Items) < x.Queue.Count:
		row("Queue", fmt.Sprintf("%d items, the first %d shown", x.Queue.Count, len(x.Queue.Items)))
	default:
		row("Queue", queueWord(x.Queue.Count))
	}
	for i, it := range x.Queue.Items {
		num := fmt.Sprintf("%d. ", i+1)
		w := max(width-detailLabel-2-ansi.StringWidth(num), 10)
		for j, l := range wrapText(it, w, 1<<20) {
			prefix := strings.Repeat(" ", detailLabel+2+ansi.StringWidth(num))
			if j == 0 {
				prefix = strings.Repeat(" ", detailLabel+2) + num
			}
			out = append(out, prefix+l)
		}
	}
	return out
}

func agoText(secs int64) string {
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm", secs/60)
	}
	return fmt.Sprintf("%dh", secs/3600)
}

// detailBody is how many lines of the detail fit on a screen of this height.
func detailBody(height int) int {
	if height <= 0 {
		return 1 << 20
	}
	return max(height-2, 1) // a title line, and the status bar
}

// detailMax is the largest scroll position for a session at this size.
func detailMax(x Session, width, height int) int {
	return max(len(detailLines(x, width))-detailBody(height), 0)
}

// detailScreen draws the title, the visible part of the detail, and a bar with the keys.
func detailScreen(x Session, width, height, scroll int, th Theme) string {
	lines := detailLines(x, width)
	body := detailBody(height)
	scroll = min(max(scroll, 0), max(len(lines)-body, 0))
	end := min(scroll+body, len(lines))
	view := append([]string{}, lines[scroll:end]...)
	title := th.Label.Render(ansi.Truncate(fmt.Sprintf(" %s  %s", x.Short, x.Dir), max(width, 1), "…"))
	hint := " esc back  up/down session  pgup/pgdn scroll "
	if len(lines) > body {
		hint = fmt.Sprintf(" lines %d-%d of %d  esc back  up/down  space/b scroll ", scroll+1, end, len(lines))
	}
	for i, l := range view {
		view[i] = ansi.Truncate(l, max(width, 1), "…")
	}
	for height > 0 && len(view) < body {
		view = append(view, "")
	}
	bar := th.Status.Width(max(width, 1)).Render(ansi.Truncate(hint, max(width, 1), ""))
	if height == 1 {
		return bar
	}
	return strings.Join(append(append([]string{title}, view...), bar), "\n")
}
