package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const takeTimeout = 5 * time.Second

// takeMsg is the answer to `sitrep move inbox`.
type takeMsg struct {
	text string
	err  error
}

// inboxIndex is the selected item, kept inside the list as the inbox grows and shrinks.
func (m model) inboxIndex() int {
	return min(max(m.inboxSel, 0), max(len(m.snap.Inbox.Items)-1, 0))
}

// selectInbox moves the selection to item i.
func (m *model) selectInbox(i int) {
	m.inboxSel = min(max(i, 0), max(len(m.snap.Inbox.Items)-1, 0))
	m.inboxText = ""
	if len(m.snap.Inbox.Items) > 0 {
		m.inboxText = m.snap.Inbox.Items[m.inboxSel]
	}
}

// inboxRefresh follows the selected item to its new position after a refresh, and drops a
// question that no longer describes the inbox or the target.
func (m *model) inboxRefresh() {
	items := m.snap.Inbox.Items
	if m.inboxText != "" {
		for i, it := range items {
			if it == m.inboxText {
				m.inboxSel = i
				break
			}
		}
	}
	m.selectInbox(m.inboxSel)
	if !m.asking {
		return
	}
	switch x := m.inboxTarget(); {
	case x == nil || !x.Running:
		m.asking = false
		m.say("take: the session is gone")
	case m.askN < 1 || m.askN > len(items) || items[m.askN-1] != m.askItem:
		m.asking = false
		m.say("take: the inbox changed")
	}
}

// inboxTarget is the session `t` hands items to: the one that was selected when the view opened.
func (m model) inboxTarget() *Session {
	for i := range m.snap.Sessions {
		if m.snap.Sessions[i].ID == m.inboxTo {
			return &m.snap.Sessions[i]
		}
	}
	return nil
}

// inboxOpen shows the inbox. Looking is always allowed; only `t` changes anything.
func (m model) inboxOpen() (tea.Model, tea.Cmd) {
	m.inbox, m.inboxTo, m.asking = true, m.selID, false
	m.selectInbox(0)
	return m, nil
}

// inboxKey handles keys while the inbox is open. While the question is open only y, n and
// cancelling do anything, so a held key cannot start a second move.
func (m model) inboxKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.asking {
		switch key {
		case "y":
			return m.takeRun()
		case "n", "esc", "q":
			m.asking = false
		}
		return m, nil
	}
	last, i := max(len(m.snap.Inbox.Items)-1, 0), m.inboxIndex()
	switch key {
	case "q":
		return m, tea.Quit
	case "esc", "i", "backspace":
		m.inbox = false
	case "up", "k":
		m.selectInbox(i - 1)
	case "down", "j":
		m.selectInbox(i + 1)
	case "home":
		m.selectInbox(0)
	case "end":
		m.selectInbox(last)
	case "t":
		return m.takeStart()
	}
	return m, nil
}

// takeStart checks that an item can be handed to the target, then asks.
func (m model) takeStart() (tea.Model, tea.Cmd) {
	items := m.snap.Inbox.Items
	x := m.inboxTarget()
	switch {
	case m.acting:
		m.say("take: busy")
	case m.readOnly:
		m.say("take: the dashboard is read-only")
	case len(items) == 0:
		m.say("take: the inbox is empty")
	case x == nil:
		m.say("take: there is no session to take it into")
	case !x.Running:
		m.say("take: " + x.Short + " has ended")
	case !idRe.MatchString(x.ID):
		m.say("take: this session's id looks unusual, so it is left alone")
	default:
		m.asking, m.askN, m.askItem = true, m.inboxIndex()+1, items[m.inboxIndex()]
	}
	return m, nil
}

// takeRun moves the item that was asked about. The text goes with it, so sitrep refuses when
// the inbox changed since it was shown, and never moves a different item.
func (m model) takeRun() (tea.Model, tea.Cmd) {
	m.asking = false
	x := m.inboxTarget()
	if x == nil || !x.Running || !idRe.MatchString(x.ID) {
		m.say("take: the session is gone")
		return m, nil
	}
	m.acting = true
	args := []string{"move", "inbox", x.ID, fmt.Sprint(m.askN), "--item", m.askItem}
	run, bin := m.run, m.bin
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), takeTimeout)
		defer cancel()
		out, err := run(ctx, bin, args...)
		return takeMsg{text: out, err: err}
	}
}

// inboxScreen draws the title, the items from the one that keeps the selection in view, and a
// bar with the keys, or the question, or the last thing an action said.
func inboxScreen(items []string, total, sel int, to *Session, asking bool, askN int, status string, readOnly bool, width, height int, th Theme) string {
	width = max(width, 1)
	body := detailBody(height)
	const gutterW = 2
	digits := len(fmt.Sprint(len(items)))
	var lines []string
	start, end := 0, 0
	for i, it := range items {
		num := fmt.Sprintf("%*d. ", digits, i+1)
		avail := max(width-gutterW-ansi.StringWidth(num), 10)
		if i == sel {
			start = len(lines)
		}
		for j, l := range wrapText(it, avail, 1<<20) {
			mark, label := "  ", strings.Repeat(" ", ansi.StringWidth(num))
			if j == 0 {
				label = num
				if i == sel {
					mark = "▸ "
				}
			}
			lines = append(lines, mark+label+l)
		}
		if i == sel {
			end = len(lines)
		}
	}
	if len(items) == 0 {
		lines = []string{"  the inbox is empty"}
	}
	top := min(max(end-body, 0), start)
	view := append([]string{}, lines[top:min(top+body, len(lines))]...)
	for i, l := range view {
		view[i] = ansi.Truncate(l, width, "…")
	}
	for height > 0 && len(view) < body {
		view = append(view, "")
	}
	target := "no session"
	if to != nil {
		target = to.Short + " " + to.Dir
	}
	hint := " esc back  up/down item  t take into " + target + " "
	switch {
	case readOnly:
		hint = " esc back  up/down item  (read-only) "
	case asking:
		hint = fmt.Sprintf(" y/n  move item %d to %s? ", askN, target)
	case status != "":
		hint = " " + status + " "
	}
	count := queueWord(len(items))
	if total > len(items) {
		count = fmt.Sprintf("the first %d of %d", len(items), total)
	}
	title := th.Label.Render(ansi.Truncate(" Inbox ("+count+")", width, "…"))
	bar := th.Status.Width(width).Render(ansi.Truncate(hint, width, ""))
	switch height {
	case 1:
		return bar
	case 2:
		return strings.Join(append(view, bar), "\n")
	}
	return strings.Join(append(append([]string{title}, view...), bar), "\n")
}
