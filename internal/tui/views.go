package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The views are pure: a snapshot and some options go in, lines of text come out.

const (
	tabWatch = iota
	tabTop
	tabCount
)

var tabNames = [tabCount]string{"Watch", "Top"}

// The Top view can be sorted four ways. Activity is the order sitrep sends, newest first.
const (
	sortActivity = iota
	sortState
	sortDir
	sortQueue
	sortCount
)

var sortNames = [sortCount]string{"activity", "state", "directory", "queue"}

type viewOpts struct {
	Width, Height int
	ShowEnded     bool
	Sort          int
	Sel           string // the id of the selected session, "" for none
	Filter        string // only sessions that contain this text are listed
	Theme         Theme
}

// visibleIDs lists the ids in the order a tab draws them, so selection moves the way the eye does.
func visibleIDs(s Snapshot, tab int, showEnded bool, by int, filter string) []string {
	list, _ := shown(s, showEnded)
	list = filterSessions(list, filter)
	if tab == tabTop {
		list = sorted(list, by)
	}
	ids := make([]string, len(list))
	for i, x := range list {
		ids[i] = x.ID
	}
	return ids
}

// matches says whether a session contains text, ignoring case, in the places a person looks:
// its id, directory, state and the three lines of its status block.
func matches(x Session, text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return true
	}
	for _, f := range []string{x.ID, x.Alias, x.Dir, x.StateLabel, x.Objective, x.Progress, x.Now} {
		if strings.Contains(strings.ToLower(f), text) {
			return true
		}
	}
	return false
}

func filterSessions(list []Session, text string) []Session {
	if strings.TrimSpace(text) == "" {
		return list
	}
	var out []Session
	for _, x := range list {
		if matches(x, text) {
			out = append(out, x)
		}
	}
	return out
}

// gutter is the two columns before a row: a marker on the selected session. A narrow screen has none.
func gutter(id string, o viewOpts) string {
	switch {
	case o.Width < 20:
		return ""
	case id != "" && id == o.Sel:
		return "▸ "
	}
	return "  "
}

// indexOf finds the block of the selected session, or -1.
func indexOf(list []Session, id string) int {
	for i, x := range list {
		if id != "" && x.ID == id {
			return i
		}
	}
	return -1
}

// stateRank orders sessions for the state sort: the ones that need the user come first.
func stateRank(x Session) int {
	switch {
	case x.NeedsInput:
		return 0
	case x.Idle:
		return 3
	case x.State == "working":
		return 1
	case x.State == "waiting":
		return 2
	}
	return 4
}

// sorted returns a copy of list in the chosen order. Ties keep the order sitrep sent, then
// break by id, so rows do not shuffle between refreshes.
func sorted(list []Session, by int) []Session {
	out := append([]Session(nil), list...)
	less := func(a, b Session) int {
		switch by {
		case sortState:
			return stateRank(a) - stateRank(b)
		case sortDir:
			return strings.Compare(strings.ToLower(a.Dir), strings.ToLower(b.Dir))
		case sortQueue:
			return b.Queue.Count - a.Queue.Count
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := less(out[i], out[j]); c != 0 {
			return c < 0
		}
		return false
	})
	return out
}

func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// wrapText wraps to width and keeps at most maxLines, ending the last with an ellipsis.
func wrapText(text string, width, maxLines int) []string {
	width = max(width, 1)
	if text == "" {
		return []string{""}
	}
	lines := strings.Split(ansi.Wrap(text, width, ""), "\n")
	if len(lines) <= maxLines {
		return lines
	}
	rest := strings.Join(lines[maxLines-1:], " ")
	return append(lines[:maxLines-1:maxLines-1], ansi.Truncate(rest, width, "…"))
}

// shown returns the sessions to list, and how many finished ones are left out.
func shown(s Snapshot, showEnded bool) (list []Session, ended int) {
	for _, x := range s.Sessions {
		if !x.Running && !showEnded {
			ended++
			continue
		}
		list = append(list, x)
	}
	return list, ended + s.Hidden.Ended
}

func queueWord(n int) string {
	switch n {
	case 0:
		return "empty"
	case 1:
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}

// field lays out "    Label:   text" with a hanging indent, in at most maxLines lines.
func field(label, text string, width, maxLines int) []string {
	prefix := "    " + padRight(label, 11)
	avail := max(width-ansi.StringWidth(prefix), 10)
	lines := wrapText(text, avail, maxLines)
	out := make([]string, len(lines))
	for i, l := range lines {
		if i == 0 {
			out[i] = prefix + l
		} else {
			out[i] = strings.Repeat(" ", ansi.StringWidth(prefix)) + l
		}
	}
	return out
}

// queueLines lists numbered items, the first under label, each wrapped to maxLines. Items
// sit under the label column, or at indent when there is no label.
func queueLines(label string, q Queue, width, maxLines, indent, limit int) []string {
	if limit >= 0 && len(q.Items) > limit { // sitrep sends every item; the setting says how many to list
		q.Items = q.Items[:limit]
	}
	var out []string
	pad := strings.Repeat(" ", indent)
	for i, it := range q.Items {
		num := fmt.Sprintf("%d. ", i+1)
		prefix := pad + num
		if i == 0 && label != "" {
			prefix = "    " + padRight(label, 11) + num
		}
		avail := max(width-ansi.StringWidth(prefix), 10)
		for j, l := range wrapText(it, avail, maxLines) {
			if j == 0 {
				out = append(out, prefix+l)
			} else {
				out = append(out, strings.Repeat(" ", ansi.StringWidth(prefix))+l)
			}
		}
	}
	if more := q.Count - len(q.Items); more > 0 {
		out = append(out, pad+fmt.Sprintf("+%d more", more))
	}
	return out
}

func paint(lines []string, st lipgloss.Style, width int) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		// clip first: a style with a width wraps a line that is too wide, it does not cut it
		out[i] = st.Width(width).Render(ansi.Truncate(l, width, "…"))
	}
	return out
}

// mark colours a session's lines: pink-red when it waits for the user, which beats the
// purple of recent activity.
func mark(lines []string, x Session, o viewOpts) []string {
	switch {
	case x.NeedsInput && x.Running:
		return paint(lines, o.Theme.Attn, o.Width)
	case x.Highlight && x.Running:
		return paint(lines, o.Theme.Hot, o.Width)
	}
	return lines
}

// stateText is the state as drawn: a ● leads the state of a session that needs the user,
// so it shows in plain text, in screenshots, and for readers who cannot tell the colours apart.
func stateText(x Session) string {
	switch {
	case x.NeedsInput && x.Running:
		return "● " + x.StateLabel
	case x.Snoozed && x.Running: // acknowledged: a hollow mark, no colour
		return "○ " + x.StateLabel
	}
	return x.StateLabel
}

// newlyNeedYou is true when some session needs the user now and did not in the last snapshot,
// that is, when something has just stopped and is waiting. A session that was already waiting
// does not count, so a refresh does not ring again.
func newlyNeedYou(prev, next Snapshot) bool {
	was := map[string]bool{}
	for _, x := range prev.Sessions {
		if x.Running && x.NeedsInput {
			was[x.ID] = true
		}
	}
	for _, x := range next.Sessions {
		if x.Running && x.NeedsInput && !was[x.ID] {
			return true
		}
	}
	return false
}

// needYou counts the running sessions that wait for the user.
func needYou(s Snapshot) int {
	n := 0
	for _, x := range s.Sessions {
		if x.Running && x.NeedsInput {
			n++
		}
	}
	return n
}

// watchBody is today's multi-line view: a block per session.
func watchBody(s Snapshot, o viewOpts) []string {
	list, ended := shown(s, o.ShowEnded)
	list = filterSessions(list, o.Filter)
	var head []string
	if s.Inbox.Count > 0 {
		head = append(head, "Inbox: "+queueWord(s.Inbox.Count))
		head = append(head, queueLines("", s.Inbox, o.Width, 2, 4, s.Settings.QueueItems)...)
	}
	dw, sw, iw := 9, 5, idWidth(s)
	for _, x := range list {
		dw, sw = max(dw, ansi.StringWidth(x.Dir)), max(sw, ansi.StringWidth(stateText(x)))
	}
	if len(list) == 0 && strings.TrimSpace(o.Filter) != "" {
		head = append(head, "no session matches the filter")
	}
	var blocks [][]string
	for _, x := range list {
		row := gutter(x.ID, o) + fmt.Sprintf("%s  %s  %s  %s", idCell(x.Label(), iw), padRight(x.Dir, dw), padRight(stateText(x), sw), queueWord(x.Queue.Count))
		block := []string{ansi.Truncate(row, o.Width, "…")}
		block = append(block, field("Objective:", x.Objective, o.Width, 3)...)
		block = append(block, field("Progress:", x.Progress, o.Width, 3)...)
		block = append(block, field("Now:", x.Now, o.Width, 3)...)
		block = append(block, queueLines("Queue:", x.Queue, o.Width, 2, 4+11, s.Settings.QueueItems)...)
		blocks = append(blocks, mark(block, x, o))
	}
	return fitBlocks(head, blocks, indexOf(list, o.Sel), ended, s.Hidden.StrandedItems, o)
}

// fitBlocks fits whole sessions to the height and adds the footers. When they do not all
// fit it scrolls by block, so that the selected one (sel, or -1) stays on the screen, and
// says how many are above and below.
func fitBlocks(head []string, blocks [][]string, sel, ended, stranded int, o viewOpts) []string {
	foot := 0
	if ended > 0 {
		foot++
	}
	if stranded > 0 {
		foot++
	}
	total := len(head)
	for _, b := range blocks {
		total += len(b)
	}
	limit := 0 // 0 means everything fits
	if o.Height > 0 && total+foot > o.Height {
		limit = o.Height - 1 - foot // leave a line for "+N more"
	}
	first := 0
	if limit > 0 && sel > 0 { // scroll, so that the whole selected block is on screen
		avail := limit - len(head)
		for first < sel {
			h := 0
			for _, b := range blocks[first : sel+1] {
				h += len(b)
			}
			above := 0
			if first > 0 {
				above = 1
			}
			if h+above <= avail {
				break
			}
			first++
		}
	}
	out := append([]string{}, head...)
	if first > 0 {
		out = append(out, fmt.Sprintf("+%d above", first))
	}
	count := first
	for _, b := range blocks[first:] {
		if limit > 0 && len(out)+len(b) > limit {
			if count == first { // never show nothing: the first block shown is clipped to the room there is
				out = append(out, b[:min(len(b), max(limit-len(out), 1))]...)
				count++
			}
			break
		}
		out = append(out, b...)
		count++
	}
	if count < len(blocks) {
		out = append(out, fmt.Sprintf("+%d more", len(blocks)-count))
	}
	if ended > 0 {
		out = append(out, fmt.Sprintf("+%d ended (a)", ended))
	}
	if stranded > 0 {
		out = append(out, fmt.Sprintf("%d items stranded in ended sessions (sitrep prune)", stranded))
	}
	if o.Height > 0 && len(out) > o.Height {
		out = out[:o.Height]
	}
	return out
}

// counts tallies the sessions for the summary line.
func counts(s Snapshot) (running, working, waiting, idle, ended int) {
	for _, x := range s.Sessions {
		if !x.Running {
			ended++
			continue
		}
		running++
		switch {
		case x.Idle:
			idle++
		case x.State == "working":
			working++
		case x.State == "waiting":
			waiting++
		}
	}
	return running, working, waiting, idle, ended + s.Hidden.Ended
}

func summary(s Snapshot) string {
	r, w, a, i, e := counts(s)
	out := fmt.Sprintf("%d running: %d working, %d waiting, %d idle?, %d ended", r, w, a, i, e)
	if n := needYou(s); n > 0 {
		out = fmt.Sprintf("● %d need you   ", n) + out
	}
	return out
}

// shortSummary is the status bar's version: short enough to leave room for the key hints.
func shortSummary(s Snapshot) string {
	r, _, _, _, e := counts(s)
	out := fmt.Sprintf("%d running, %d ended", r, e)
	if n := needYou(s); n > 0 {
		out = fmt.Sprintf("● %d need you   ", n) + out
	}
	return out
}

// topBody is the one-line-per-session view.
func topBody(s Snapshot, o viewOpts) []string {
	list, ended := shown(s, o.ShowEnded)
	list = sorted(filterSessions(list, o.Filter), o.Sort)
	head := []string{summary(s)}
	if s.Inbox.Count > 0 {
		head[0] += "   inbox " + queueWord(s.Inbox.Count)
	}

	dw, sw := 9, 5
	for _, x := range list {
		dw, sw = max(dw, ansi.StringWidth(x.Dir)), max(sw, ansi.StringWidth(stateText(x)))
	}
	iw := idWidth(s)
	dw = min(dw, max((o.Width-16-iw)/3, 6)) // a narrow screen gets a narrow directory column
	head = append(head, o.Theme.Label.Render(gutter("", o)+fmt.Sprintf("%s  %s  %s  %-5s  %s", idCell("ID", iw), padRight("DIRECTORY", dw), padRight("STATE", sw), "QUEUE", "NOW   sort: "+sortNames[o.Sort])))
	if len(list) == 0 && strings.TrimSpace(o.Filter) != "" {
		head = append(head, "no session matches the filter")
	}
	var blocks [][]string
	for _, x := range list {
		text := x.Now
		if text == "" {
			text = x.Objective
		}
		prefix := gutter(x.ID, o) + fmt.Sprintf("%s  %s  %s  %-5d  ", idCell(x.Label(), iw), padRight(ansi.Truncate(x.Dir, dw, "…"), dw), padRight(stateText(x), sw), x.Queue.Count)
		line := prefix + ansi.Truncate(text, max(o.Width-ansi.StringWidth(prefix), 1), "…")
		blocks = append(blocks, mark([]string{line}, x, o))
	}
	return fitBlocks(head, blocks, indexOf(list, o.Sel), ended, s.Hidden.StrandedItems, o)
}

func tabBar(active, width int, th Theme) []string {
	var parts []string
	for i, n := range tabNames {
		switch {
		case th.Plain && i == active:
			parts = append(parts, "["+n+"] ")
		case th.Plain:
			parts = append(parts, " "+n+"  ")
		case i == active:
			parts = append(parts, th.TabActive.Render(n))
		default:
			parts = append(parts, th.Tab.Render(n))
		}
	}
	return []string{ansi.Truncate(strings.Join(parts, ""), max(width, 1), ""), th.Rule.Render(strings.Repeat("─", max(width, 1)))}
}

// statusBar shows the counts on the left and the keys on the right. A failed refresh
// replaces the counts with the error, so the last good data stays on screen.
func statusBar(s Snapshot, width int, errText string, th Theme) string {
	left := " " + shortSummary(s)
	if errText != "" {
		left = " " + errText
	}
	right := "q quit  tab  a ended  s sort  ? help "
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		right, gap = "", width-ansi.StringWidth(left)
	}
	line := ansi.Truncate(left+strings.Repeat(" ", max(gap, 0))+right, width, "")
	return th.Status.Width(width).Render(line)
}

// helpScreen is the key list: the keys, what the tabs show, and what the colours mean.
func helpScreen(width, height int, readOnly bool, th Theme) string {
	lines := []string{
		"Keys",
		"",
		"  up, down, j, k, home, end      move the selection (the \u25b8 marker)",
		"  enter                          jump to its tmux pane or cmux surface",
		"  y, Y                           copy the id, or its claude --resume command",
		"  d, space                       the selected session in full (esc closes)",
		"  tab, shift+tab, left, right    switch tab (also 1 and 2)",
		"  a                              show or hide ended sessions",
		"  s                              cycle the Top sort: activity, state, dir, queue",
		"  /                              filter the sessions; enter keeps it, esc clears it",
		"  i, e                           the inbox, or the selected session's queue",
		"  a x K J T                      add, remove (asks), up, down, top",
		"  t, P, U                        take an inbox item; pin or unpin the Objective",
		"  z                              snooze it until it changes; z again ends it",
		"  p                              prune the sessions that are gone (asks first)",
		"  r                              refresh now",
		"  ?                              this list",
		"  q, ctrl+c                      quit",
		"",
		"Tabs",
		"",
		"  Watch   every session as a block: Objective, Progress, Now and the queue",
		"  Top     a summary line, then one line per session",
		"",
		"Colours",
		"",
		"  ● pink-red   Claude has stopped and the next move is yours, until you reply",
		"  ○            snoozed: waiting, but you said it can wait",
		"  purple       the session changed in the last minute",
		"",
		"Any key closes this list.",
	}
	if readOnly { // the keys that change state are off, so they are not listed
		kept := lines[:0]
		for _, l := range lines {
			if !strings.HasPrefix(l, "  p ") && !strings.HasPrefix(l, "  z ") && !strings.HasPrefix(l, "  t, ") && !strings.HasPrefix(l, "  a x ") {
				kept = append(kept, l)
			}
		}
		lines = append(kept[:len(kept):len(kept)], "", "This dashboard is read-only.")
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, max(width, 1), "…")
	}
	if height > 0 {
		lines = lines[:min(len(lines), max(height-1, 0))]
		for len(lines) < height-1 {
			lines = append(lines, "")
		}
	}
	bar := th.Status.Width(max(width, 1)).Render(ansi.Truncate(" keys", max(width, 1), ""))
	if height == 1 {
		return bar
	}
	return strings.Join(append(lines, bar), "\n")
}

// render lays out the whole screen: tabs on top, the body, the status bar at the bottom.
func render(s Snapshot, tab int, errText string, o viewOpts) string {
	bar := tabBar(tab, o.Width, o.Theme)
	bodyOpts := o
	if o.Height > 0 { // a height of 0 means no limit, as for --once
		bodyOpts.Height = max(o.Height-len(bar)-1, 1)
	}
	var body []string
	if tab == tabTop {
		body = topBody(s, bodyOpts)
	} else {
		body = watchBody(s, bodyOpts)
	}
	for i, l := range body { // a last guard: nothing may be wider than the screen
		body[i] = ansi.Truncate(l, max(o.Width, 1), "…")
	}
	for len(body) < bodyOpts.Height { // pad, so the status bar sits on the last row
		body = append(body, "")
	}
	lines := append(append(bar, body...), statusBar(s, o.Width, errText, o.Theme))
	if o.Height > 0 && len(lines) > o.Height { // a tiny screen: keep the status bar, drop from the top
		lines = append(lines[:o.Height-1], lines[len(lines)-1])
	}
	return strings.Join(lines, "\n")
}

// maxLabelWidth is the widest the first column grows for an alias; a longer alias is cut with an ellipsis.
const maxLabelWidth = 16

// idWidth is the width of the first column: four, or the longest label of the whole snapshot (so that a filter does not move it) up to
// maxLabelWidth.
func idWidth(s Snapshot) int {
	w := 4
	for _, x := range s.Sessions {
		w = max(w, min(ansi.StringWidth(x.Label()), maxLabelWidth))
	}
	return w
}

// idCell is a label cut to the column and padded to its width.
func idCell(label string, w int) string { return padRight(ansi.Truncate(label, w, "…"), w) }
