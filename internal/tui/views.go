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
	tabCards = iota
	tabTable
	tabWall
	tabCount
)

var tabNames = [tabCount]string{"Cards", "Table", "Wall"}

// The Table view can be sorted four ways. Activity is the order sitrep sends, newest first.
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
	Typing        bool        // a filter is being typed, so the footer lists the keys of that
	Error         bool        // the status text is an error: it is drawn in the error colour
	Demo          bool        // made-up sessions: the status bar says so
	Hits          *[]string   // when set, filled with the id of the session on each screen row ("" for none), for a mouse click
	Auto          bool        // no tile size is chosen yet: the Wall picks the one that fits
	Size          int         // the Wall's tile size (wallStandard, wallRoomy or wallCompact)
	Cells         *[]wallCell // when set, filled with the place of each tile of the Wall, for a mouse click
}

// visibleIDs lists the ids in the order a tab draws them, so selection moves the way the eye does.
func visibleIDs(s Snapshot, tab int, showEnded bool, by int, filter string) []string {
	list, _ := shown(s, showEnded)
	list = filterSessions(list, filter)
	switch tab {
	case tabTable:
		list = sorted(list, by)
	default: // Cards and the Wall: the sessions that need you first
		list = sorted(list, sortState)
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
		return "▶ "
	}
	return "  "
}

// selectBlock gives the selected session's lines a solid two-column bar in the gutter, over the whole block, with an arrow on the first
// line. The bar uses the style of the status bar's keys (white on the accent colour) so it shows against the pink and grey row colours;
// without colour the arrow is followed by a vertical line.
func selectBlock(lines []string, x Session, o viewOpts) []string {
	if o.Width < 20 || x.ID == "" || x.ID != o.Sel {
		return lines
	}
	cont := "  "
	if o.Theme.Plain {
		cont = "│ "
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		glyph := cont
		if i == 0 {
			glyph = "▶ "
		}
		out[i] = o.Theme.StatusKey.Render(glyph) + ansi.TruncateLeft(l, 2, "")
	}
	return out
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

// noMatch is the line for a filter that lists nothing; it says so when finished sessions, hidden now, do match.
func noMatch(s Snapshot, o viewOpts) string {
	msg := "no session matches the filter"
	if o.ShowEnded {
		return msg
	}
	var gone []Session
	for _, x := range s.Sessions {
		if !x.Running {
			gone = append(gone, x)
		}
	}
	if n := len(filterSessions(gone, o.Filter)); n > 0 {
		msg += fmt.Sprintf(" (%d ended hidden, press a)", n)
	}
	return msg
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
// grey of recent activity. A session that has waited long (AttnFaded) is not coloured for it.
func mark(lines []string, x Session, o viewOpts) []string {
	switch {
	case x.NeedsInput && x.Running && !x.AttnFaded:
		return paint(lines, o.Theme.Attn, o.Width)
	case x.Highlight && x.Running:
		return paint(lines, o.Theme.Hot, o.Width)
	case x.Snoozed && x.Running && len(lines) > 0: // only the mark and the state word, in the colour the theme gives them
		out := append([]string(nil), lines...)
		st := stateText(x)
		out[0] = strings.Replace(out[0], st, o.Theme.Snooze.Render(st), 1)
		return out
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

// cardRoom is how many lines a field of a card, and an item of a queue, may take.
type cardRoom struct {
	field, item int
	compact     bool // only the header and the Now line of a session, and the inbox on one line
}

// cardRooms goes from the roomiest to the tightest: the Cards tab uses the first that shows every session whole, so a screen with
// few sessions gives each its full text, and one with many cuts each to the short form.
var cardRooms = []cardRoom{{8, 8, false}, {5, 4, false}, {3, 2, false}}

// cardsCompactFrom is how many sessions it takes for the Cards tab to give up the Objective and the Progress of each when even the short form
// does not fit: with fewer, scrolling through whole cards is fine.
const cardsCompactFrom = 5

// cardsBody is today's multi-line view: a block per session.
func cardsBody(s Snapshot, o viewOpts) []string {
	if o.Height > 0 {
		for _, room := range cardRooms[:len(cardRooms)-1] {
			head, blocks, ids, foot := cardBlocks(s, o, room)
			total := len(head) + foot
			for _, b := range blocks {
				total += len(b)
			}
			if total <= o.Height {
				return layBlocks(s, o, head, blocks, ids)
			}
		}
	}
	head, blocks, ids, foot := cardBlocks(s, o, cardRooms[len(cardRooms)-1])
	if o.Height > 0 && len(ids) >= cardsCompactFrom {
		total := len(head) + foot
		for _, b := range blocks {
			total += len(b)
		}
		if total > o.Height {
			head, blocks, ids, _ = cardBlocks(s, o, cardRoom{field: 1, item: 1, compact: true})
		}
	}
	return layBlocks(s, o, head, blocks, ids)
}

// layBlocks fits the blocks of the cards to the height.
func layBlocks(s Snapshot, o viewOpts, head []string, blocks [][]string, ids []string) []string {
	list, ended := shown(s, o.ShowEnded)
	list = sorted(filterSessions(list, o.Filter), sortState)
	return fitBlocks(head, blocks, ids, indexOf(list, o.Sel), ended, s.Hidden.StrandedItems, o)
}

// cardBlocks lays out the header and a block per session within room, and counts the footer lines that follow.
func cardBlocks(s Snapshot, o viewOpts, room cardRoom) (head []string, blocks [][]string, ids []string, foot int) {
	list, ended := shown(s, o.ShowEnded)
	list = sorted(filterSessions(list, o.Filter), sortState) // the Wall's order: the sessions that need you first
	if s.Inbox.Count > 0 {
		head = append(head, "Inbox: "+queueWord(s.Inbox.Count))
		if room.compact {
			head[len(head)-1] += " (i)"
		} else {
			head = append(head, queueLines("", s.Inbox, o.Width, room.item, 4, s.Settings.QueueItems)...)
		}
	}
	dw, sw, iw := 9, 5, idWidth(s)
	for _, x := range list {
		dw, sw = max(dw, ansi.StringWidth(x.Dir)), max(sw, ansi.StringWidth(stateText(x)))
	}
	if len(list) == 0 && strings.TrimSpace(o.Filter) != "" {
		head = append(head, noMatch(s, o))
	}
	for _, x := range list {
		ids = append(ids, x.ID)
		row := gutter(x.ID, o) + fmt.Sprintf("%s  %s  %s  %s", idCell(x.Label(), iw), padRight(x.Dir, dw), padRight(stateText(x), sw), queueWord(x.Queue.Count))
		block := []string{ansi.Truncate(row, o.Width, "…")}
		if room.compact {
			now, label := x.Now, "Now:"
			if now == "" {
				now, label = x.Objective, "Objective:"
			}
			block = append(block, field(label, now, o.Width, 1)...)
		} else {
			block = append(block, field("Objective:", x.Objective, o.Width, room.field)...)
			block = append(block, field("Progress:", x.Progress, o.Width, room.field)...)
			block = append(block, field("Now:", x.Now, o.Width, room.field)...)
			block = append(block, queueLines("Queue:", x.Queue, o.Width, room.item, 4+11, s.Settings.QueueItems)...)
		}
		blocks = append(blocks, selectBlock(mark(block, x, o), x, o))
	}
	if ended > 0 {
		foot++
	}
	if s.Hidden.StrandedItems > 0 {
		foot++
	}
	return head, blocks, ids, foot
}

// fitBlocks fits whole sessions to the height and adds the footers. When they do not all
// fit it scrolls by block, so that the selected one (sel, or -1) stays on the screen, and
// says how many are above and below.
func fitBlocks(head []string, blocks [][]string, ids []string, sel, ended, stranded int, o viewOpts) []string {
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
	hits := make([]string, len(out)) // the session of each line of out, "" for a line that is none
	add := func(lines []string, id string) {
		out = append(out, lines...)
		for range lines {
			hits = append(hits, id)
		}
	}
	if first > 0 {
		add([]string{fmt.Sprintf("+%d above", first)}, "")
	}
	count := first
	for i, b := range blocks[first:] {
		if limit > 0 && len(out)+len(b) > limit {
			if count == first { // never show nothing: the first block shown is clipped to the room there is
				add(b[:min(len(b), max(limit-len(out), 1))], ids[first+i])
				count++
			}
			break
		}
		add(b, ids[first+i])
		count++
	}
	if count < len(blocks) {
		add([]string{fmt.Sprintf("+%d more", len(blocks)-count)}, "")
	}
	if ended > 0 {
		add([]string{fmt.Sprintf("+%d ended (a)", ended)}, "")
	}
	if stranded > 0 {
		add([]string{fmt.Sprintf("%d items stranded in ended sessions (sitrep prune)", stranded)}, "")
	}
	if o.Height > 0 && len(out) > o.Height {
		out = out[:o.Height]
		hits = hits[:o.Height]
	}
	if o.Hits != nil {
		*o.Hits = hits
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
		case x.Blocked != "":
			waiting++ // it needs the user, whatever the state word says
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
	every := list // column widths follow every listed session, so the layout does not change with the filter
	list = sorted(filterSessions(list, o.Filter), o.Sort)
	head := []string{summary(s)}
	if s.Inbox.Count > 0 {
		head[0] += "   inbox " + queueWord(s.Inbox.Count)
	}

	dw, sw := 9, 5
	for _, x := range every {
		dw, sw = max(dw, ansi.StringWidth(x.Dir)), max(sw, ansi.StringWidth(stateText(x)))
	}
	iw := idWidth(s)
	full := min(dw, max((o.Width-16-iw)/3, 6)) // a narrow screen gets a narrow directory column
	// the CTX column goes in when the text of NOW keeps at least minNow columns; it narrows the directory column a little for the room
	dw = full
	showCtx := false
	if withCtx := min(dw, max((o.Width-22-iw)/3, 6)); o.Width-(ansi.StringWidth(gutter("", o))+iw+withCtx+sw+13+ctxWidth+2) >= minNow {
		showCtx, dw = true, withCtx
	}
	ctxHead := ""
	if showCtx {
		ctxHead = padRight("CTX", ctxWidth) + "  "
	}
	head = append(head, o.Theme.Label.Render(gutter("", o)+fmt.Sprintf("%s  %s  %s  %-5s  %s%s", idCell("ID", iw), padRight("DIRECTORY", dw), padRight("STATE", sw), "QUEUE", ctxHead, "NOW   sort: "+sortNames[o.Sort])))
	if len(list) == 0 && strings.TrimSpace(o.Filter) != "" {
		head = append(head, noMatch(s, o))
	}
	var blocks [][]string
	var ids []string
	for _, x := range list {
		ids = append(ids, x.ID)
		text := x.Now
		if text == "" {
			text = x.Objective
		}
		ctx := ""
		if showCtx {
			ctx = ctxCell(x, o) + "  "
		}
		prefix := gutter(x.ID, o) + fmt.Sprintf("%s  %s  %s  %-5d  %s", idCell(x.Label(), iw), padRight(ansi.Truncate(x.Dir, dw, "…"), dw), padRight(stateText(x), sw), x.Queue.Count, ctx)
		line := prefix + ansi.Truncate(text, max(o.Width-ansi.StringWidth(prefix), 1), "…")
		blocks = append(blocks, selectBlock(mark([]string{line}, x, o), x, o))
	}
	return fitBlocks(head, blocks, ids, indexOf(list, o.Sel), ended, s.Hidden.StrandedItems, o)
}

// tabParts is the text of each tab's name, as the tab bar draws it.
func tabParts(active int, th Theme) []string {
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
	return parts
}

func tabBar(active, width int, th Theme) []string {
	return []string{ansi.Truncate(strings.Join(tabParts(active, th), ""), max(width, 1), ""), th.Rule.Render(strings.Repeat("─", max(width, 1)))}
}

// tabAt is the tab whose name is under column x of the tab bar, or -1: the names, with the space around them that the bar draws, so that a
// click just beside a name is still a click on it.
func tabAt(x int, th Theme) int {
	col := 0
	for i, p := range tabParts(0, th) {
		col += ansi.StringWidth(p)
		if x >= 0 && x < col {
			return i
		}
	}
	return -1
}

// statusBar shows the counts on the left and the keys on the right. A failed refresh
// replaces the counts with the error, so the last good data stays on screen.
func statusBar(s Snapshot, width int, errText string, isErr bool, tab int, typing, filtered, demo bool, th Theme) string {
	left := " " + shortSummary(s)
	if demo {
		left = " DEMO: made-up sessions   " + shortSummary(s)
	}
	if errText != "" {
		left = " " + errText
	}
	hints := []string{"q quit", "enter jump", "tab switch", "/ filter", "n name", "1-9 take", "a ended", "? help"}
	switch tab {
	case tabTable: // only the Table view has a sort
		hints = []string{"q quit", "enter jump", "s sort", "tab switch", "/ filter", "n name", "1-9 take", "a ended", "? help"}
	case tabWall:
		hints = []string{"q quit", "enter jump", "tab switch", "hjkl move", "s size", "/ filter", "n name", "1-9 take", "a ended", "? help"}
	}
	if filtered && !typing { // a filter is on: say how to end it, ahead of the keys a narrow screen drops
		hints = append([]string{hints[0], "esc clear filter"}, hints[1:]...)
	}
	if typing { // the other keys are text now
		hints = []string{"enter keep", "up/down move", "ctrl+u clear", "esc cancel"}
	}
	right := fitHints(hints, width-ansi.StringWidth(left)-1)
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		right, gap = "", width-ansi.StringWidth(left)
	}
	line := ansi.Truncate(left+strings.Repeat(" ", max(gap, 0))+right, width, "")
	if isErr && errText != "" { // the error is a block of its own colour at the left of the bar
		lw := min(ansi.StringWidth(left)+1, width)
		return th.Err.Render(ansi.Truncate(line, lw, "")) + th.Status.Width(width-lw).Render(ansi.TruncateLeft(line, lw, ""))
	}
	return th.Status.Width(width).Render(line)
}

// fitHints joins the key hints of a footer, most important first, with the last one (the help) always kept: the ones that do not fit in
// width are dropped from the end, so a narrow screen loses the least useful hint and never the way to the help. It returns "" when not even
// the last one fits.
func fitHints(hints []string, width int) string {
	last := hints[len(hints)-1]
	for n := len(hints) - 1; n >= 0; n-- {
		line := " " + strings.Join(append(append([]string{}, hints[:n]...), last), "  ") + " "
		if ansi.StringWidth(line) <= width {
			return line
		}
	}
	return ""
}

// helpWrap wraps a help line that is wider than the screen at the column where its description starts, so a narrow screen shows all of
// it. A line with no description column, or a screen too narrow to hold one, is left for the caller to cut.
func helpWrap(lines []string, width int) []string {
	var out []string
	for _, l := range lines {
		if ansi.StringWidth(l) <= width {
			out = append(out, l)
			continue
		}
		r := []rune(l)
		j := 2
		for j+1 < len(r) && !(r[j] == ' ' && r[j+1] == ' ') {
			j++
		}
		k := j
		for k < len(r) && r[k] == ' ' {
			k++
		}
		if j+1 >= len(r) || k >= len(r) || width-k < 12 {
			out = append(out, l)
			continue
		}
		wrapped := strings.Split(ansi.Wrap(string(r[k:]), width-k, ""), "\n")
		out = append(out, string(r[:k])+wrapped[0])
		for _, w := range wrapped[1:] {
			out = append(out, strings.Repeat(" ", k)+w)
		}
	}
	return out
}

// helpStep is how far a key scrolls the key list, in lines.
func helpStep(key string, height int) (int, bool) {
	page := max(height-2, 1)
	switch key {
	case "j", "down":
		return 1, true
	case "k", "up":
		return -1, true
	case "pgdown", "space", "ctrl+f", "ctrl+d":
		return page, true
	case "pgup", "ctrl+b", "ctrl+u":
		return -page, true
	case "g", "home":
		return -1 << 20, true
	case "G", "end":
		return 1 << 20, true
	}
	return 0, false
}

// helpLines is the key list: the keys, what the tabs show, and what the colours mean, wrapped and cut to the width.
func helpLines(width int, readOnly bool) []string {
	lines := []string{
		"Keys",
		"",
		"  j, k, gg, G, ctrl+d, ctrl+u    move the selection (the \u25b6 arrow and bar)",
		"  arrows, home, end, pgup, pgdn  the same, as in vi and top",
		"  enter                          jump to its tmux pane or cmux surface",
		"  y, Y                           copy the id, or its claude --resume command",
		"  v, space                       the selected session in full (q closes; e, i: queues)",
		"  tab, shift+tab, h, l           switch tab (left and right too)",
		"  h, l, j, k, arrows             on the Wall: the tile left, right, above, below",
		"                                 (left of the first and right of the last: the tab)",
		"  1-9                            take that inbox item into the selected session",
		"  a                              show or hide ended sessions",
		"  s                              cycle the Table sort, or the Wall's tile size",
		"  /                              filter the sessions; enter keeps it, esc clears it",
		"  ctrl+u, ctrl+w                 in a line you type: clear it, or its last word",
		"  i, e                           the inbox, or the session's queue; in a queue, the other one",
		"  o, enter, dd, K J T            add, edit, remove (asks), move up, down, top",
		"  t, P, U, m                     take an inbox item; pin, unpin; m sends it on",
		"  n                              name the selected session (an alias); empty clears it",
		"  z                              snooze it until it changes; z again ends it",
		"  C                              choose the colours: a preview, a picker, and a write to sitrep.conf",
		"  p                              prune the sessions that are gone (asks first)",
		"  r                              refresh now",
		"  ?                              this list, from any screen",
		"  q                              back; quit on the tabs (ctrl+c: anywhere)",
		"",
		"Tabs",
		"",
		"  Cards   every session as a block: Objective, Progress, Now and the queue",
		"  Table   a summary line, then one line per session (CTX: context left)",
		"  Wall    the sessions as tiles of one size, many to a screen, those that need you first",
		"",
		"Colours (default theme)",
		"",
		"  ● pink-red   Claude has stopped and the next move is yours, until you reply",
		"               the colour fades after 10 minutes (SITREP_ATTN_FADE); the ● stays",
		"  ○            snoozed: waiting, but you said it can wait",
		"  grey         the session changed in the last minute",
		"  idle?        working, but quiet for 10 minutes: it may have been interrupted",
	}
	if readOnly { // the keys that change state are off, so they are not listed
		kept := lines[:0]
		for _, l := range lines {
			if !strings.HasPrefix(l, "  p ") && !strings.HasPrefix(l, "  z ") && !strings.HasPrefix(l, "  t, P, U") && !strings.HasPrefix(l, "  o, enter, dd") && !strings.HasPrefix(l, "  n ") && !strings.HasPrefix(l, "  1-9") {
				kept = append(kept, l)
			}
		}
		lines = append(kept[:len(kept):len(kept)], "", "This dashboard is read-only.")
	}
	lines = helpWrap(lines, width)
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, max(width, 1), "…")
	}
	return lines
}

// helpScreen shows the key list from line scroll on, in the room the screen has.
func helpScreen(width, height int, readOnly bool, scroll int, th Theme) string {
	lines := helpLines(width, readOnly)
	hint := " keys   any key closes this list"
	if height > 0 {
		room := max(height-1, 0)
		if len(lines) > room {
			scroll = max(0, min(scroll, len(lines)-room))
			hint = fmt.Sprintf(" keys   j/k scroll (%d-%d of %d)   any other key closes", scroll+1, scroll+room, len(lines))
		} else {
			scroll = 0
		}
		lines = lines[scroll:min(len(lines), scroll+room)]
		for len(lines) < room {
			lines = append(lines, "")
		}
	}
	bar := th.Status.Width(max(width, 1)).Render(ansi.Truncate(hint, max(width, 1), ""))
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
	var hits []string
	if o.Hits != nil {
		bodyOpts.Hits = &hits
	}
	var cells []wallCell
	switch tab {
	case tabTable:
		body = topBody(s, bodyOpts)
	case tabWall:
		if o.Cells != nil {
			bodyOpts.Cells = &cells
		}
		body = wallBody(s, bodyOpts)
	default:
		body = cardsBody(s, bodyOpts)
	}
	if o.Cells != nil {
		*o.Cells = (*o.Cells)[:0]
		for _, c := range cells {
			c.Y0, c.Y1 = c.Y0+len(bar), c.Y1+len(bar)
			*o.Cells = append(*o.Cells, c)
		}
	}
	for i, l := range body { // a last guard: nothing may be wider than the screen
		body[i] = ansi.Truncate(l, max(o.Width, 1), "…")
	}
	for len(body) < bodyOpts.Height { // pad, so the status bar sits on the last row
		body = append(body, "")
	}
	lines := append(append(bar, body...), statusBar(s, o.Width, errText, o.Error, tab, o.Typing, strings.TrimSpace(o.Filter) != "", o.Demo, o.Theme))
	if o.Hits != nil {
		h := append(make([]string, len(bar)), hits...)
		for len(h) < len(bar)+len(body) { // the body was padded
			h = append(h, "")
		}
		*o.Hits = append(h, "") // the status bar
	}
	if o.Height > 0 && len(lines) > o.Height { // a tiny screen: keep the status bar, drop from the top
		lines = append(lines[:o.Height-1], lines[len(lines)-1])
		if o.Hits != nil {
			*o.Hits = append((*o.Hits)[:o.Height-1], "")
		}
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

const (
	ctxWidth = 4  // the CTX column: 100%, !9%, or -
	minNow   = 20 // the room the text of NOW keeps before CTX is dropped
)

// ctxCell is the share of the context window left, rounded down, in ctxWidth columns: "62%", "!9%" at 10% or less (so the plain text carries
// the warning, as the ● does for a session that needs you), and "-" where nothing is known. At 20% or less it is bold, but not on a row
// that is already coloured: a colour reset inside such a row would end its colour.
func ctxCell(x Session, o viewOpts) string {
	if x.Context == nil || !x.Running {
		return padRight("-", ctxWidth)
	}
	left := min(max(x.Context.Left(), 0), 100)
	text := fmt.Sprintf("%d%%", left)
	if left <= 10 {
		text = "!" + text
	}
	cell := padRight(text, ctxWidth)
	if left <= 20 && !(x.Running && ((x.NeedsInput && !x.AttnFaded) || x.Highlight)) {
		cell = o.Theme.Warn.Render(cell)
	}
	return cell
}
