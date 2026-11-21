package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The queue view shows one queue: the inbox, or the queue of the selected session.
const (
	qInbox = iota
	qSession
)

const (
	queueTimeout = 5 * time.Second
	maxPrompt    = 200 // what `sitrep add` keeps
)

// queueMsg is the answer to a sitrep command that changes a queue.
type queueMsg struct {
	verb string
	text string
	err  error
}

// askState is the question about one change. It holds what it is about as it was when it was
// asked, so `y` moves that item and no other, whatever a refresh has done since.
type askState struct {
	verb   string // "take" or "remove"
	qid    string // the queue the item is in: inbox, or a session id
	n      int    // its position,
	item   string // and its text
	target string // for take: the session that gets it
}

// promptState is a line being typed for `a` (add) or `P` (pin).
type promptState struct {
	verb string
	qid  string
	text string
}

// addText adds typed or pasted text to cur: line breaks and tabs become spaces, control and
// format characters go, and the result is cut at limit characters.
func addText(cur, text string, limit int) string {
	text = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(text)
	text = strings.TrimSuffix(clean(text+"|"), "|") // the mark keeps clean from trimming a typed space
	r := []rune(cur + text)
	if len(r) > limit {
		r = r[:limit]
	}
	return string(r)
}

func (m model) session(id string) *Session {
	for i := range m.snap.Sessions {
		if m.snap.Sessions[i].ID == id {
			return &m.snap.Sessions[i]
		}
	}
	return nil
}

// queueInfo is the queue on screen: its items, how many it holds, its id and its session.
func (m model) queueInfo() (items []string, total int, qid string, x *Session) {
	if m.qKind == qInbox {
		return m.snap.Inbox.Items, m.snap.Inbox.Count, "inbox", nil
	}
	if x = m.session(m.qTo); x == nil {
		return nil, 0, m.qTo, nil
	}
	return x.Queue.Items, x.Queue.Count, x.ID, x
}

func (m model) queueIndex() int {
	items, _, _, _ := m.queueInfo()
	return min(max(m.qSel, 0), max(len(items)-1, 0))
}

// selectQueue selects item i and remembers its text, so the selection can follow it.
func (m *model) selectQueue(i int) {
	items, _, _, _ := m.queueInfo()
	m.qSel, m.qText = min(max(i, 0), max(len(items)-1, 0)), ""
	if len(items) > 0 {
		m.qText = items[m.qSel]
	}
}

// queueRefresh follows the selected item after a refresh: to the same position if it is still
// there, else to the nearest item with the same text. It drops a question that no longer fits.
func (m *model) queueRefresh() {
	items, _, _, _ := m.queueInfo()
	if m.qText != "" && !(m.qSel < len(items) && items[m.qSel] == m.qText) {
		best := -1
		for i, it := range items {
			if it == m.qText && (best < 0 || abs(i-m.qSel) < abs(best-m.qSel)) {
				best = i
			}
		}
		if best >= 0 {
			m.qSel = best
		}
	}
	m.selectQueue(m.qSel)
	if m.ask == nil {
		return
	}
	a := *m.ask
	qi, ok := m.itemsOf(a.qid)
	switch {
	case !ok || (a.target != "" && !m.running(a.target)):
		m.ask = nil
		m.say(a.verb + ": the session is gone")
	case a.n < 1 || a.n > len(qi) || qi[a.n-1] != a.item:
		m.ask = nil
		m.say(a.verb + ": the queue changed")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// itemsOf is the items of a queue by id, and whether the queue can be edited.
func (m model) itemsOf(qid string) ([]string, bool) {
	if qid == "inbox" {
		return m.snap.Inbox.Items, true
	}
	x := m.session(qid)
	if x == nil || !x.Running {
		return nil, false
	}
	return x.Queue.Items, true
}

func (m model) running(id string) bool {
	x := m.session(id)
	return x != nil && x.Running
}

// queueOpen shows a queue. Looking is always allowed; only the edit keys change anything.
func (m model) queueOpen(kind int) (tea.Model, tea.Cmd) {
	if kind == qSession && m.selected() == nil {
		m.say("queue: no session is selected")
		return m, nil
	}
	m.queue, m.qKind, m.qTo, m.ask, m.prompt = true, kind, m.selID, nil, nil
	m.selectQueue(0)
	return m, nil
}

// editCheck says why a change to queue qid, and to session target, cannot be made, or "".
func (m model) editCheck(qid, target string) string {
	switch {
	case m.acting:
		return "busy"
	case m.readOnly:
		return "the dashboard is read-only"
	}
	for _, id := range []string{qid, target} {
		switch x := m.session(id); {
		case id == "inbox" || id == "":
		case x == nil:
			return "the session is gone"
		case !x.Running:
			return x.Short + " has ended"
		case !idRe.MatchString(x.ID):
			return "this session's id looks unusual, so it is left alone"
		}
	}
	return ""
}

// queueKey handles keys while a queue is open. While a question is open only y, n and
// cancelling do anything, so a held key cannot start a second change.
func (m model) queueKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.ask != nil {
		switch key {
		case "y":
			return m.askRun()
		case "n", "esc", "q":
			m.ask = nil
		}
		return m, nil
	}
	last, i := max(len(m.queueItems())-1, 0), m.queueIndex()
	switch key {
	case "q":
		return m, tea.Quit
	case "esc", "backspace":
		m.queue = false
	case "i":
		m.queue = m.qKind != qInbox
	case "e":
		m.queue = m.qKind != qSession
	case "up", "k":
		m.selectQueue(i - 1)
	case "down", "j":
		m.selectQueue(i + 1)
	case "home":
		m.selectQueue(0)
	case "end":
		m.selectQueue(last)
	case "t":
		if m.qKind == qInbox {
			return m.takeStart()
		}
	case "a":
		return m.promptOpen("add")
	case "P":
		if m.qKind == qSession {
			return m.promptOpen("pin")
		}
	case "U":
		if m.qKind == qSession {
			return m.unpinRun()
		}
	case "x":
		return m.removeStart()
	case "K", "J", "T":
		return m.moveStart(key)
	}
	return m, nil
}

func (m model) queueItems() []string {
	items, _, _, _ := m.queueInfo()
	return items
}

func (m model) refuse(verb, why string) (tea.Model, tea.Cmd) {
	m.say(verb + ": " + why)
	return m, nil
}

func (m model) takeStart() (tea.Model, tea.Cmd) {
	items := m.queueItems()
	if m.qTo == "" {
		return m.refuse("take", "there is no session to take it into")
	}
	if why := m.editCheck("inbox", m.qTo); why != "" {
		return m.refuse("take", why)
	}
	if len(items) == 0 {
		return m.refuse("take", "the inbox is empty")
	}
	i := m.queueIndex()
	m.ask = &askState{verb: "take", qid: "inbox", n: i + 1, item: items[i], target: m.qTo}
	return m, nil
}

func (m model) removeStart() (tea.Model, tea.Cmd) {
	items, _, qid, _ := m.queueInfo()
	if why := m.editCheck(qid, ""); why != "" {
		return m.refuse("remove", why)
	}
	if len(items) == 0 {
		return m.refuse("remove", "the queue is empty")
	}
	i := m.queueIndex()
	m.ask = &askState{verb: "remove", qid: qid, n: i + 1, item: items[i]}
	return m, nil
}

// askRun runs the change the question was about.
func (m model) askRun() (tea.Model, tea.Cmd) {
	a := *m.ask
	m.ask = nil
	if why := m.editCheck(a.qid, a.target); why != "" {
		return m.refuse(a.verb, why)
	}
	n := fmt.Sprint(a.n)
	if a.verb == "take" {
		return m.queueRun("take", -1, "move", "inbox", a.target, n, "--item", a.item)
	}
	return m.queueRun("remove", a.n-1, "rm", a.qid, n, "--item", a.item)
}

// moveStart moves the selected item up, down or to the front. A move that cannot happen says
// so and runs nothing.
func (m model) moveStart(key string) (tea.Model, tea.Cmd) {
	items, total, qid, _ := m.queueInfo()
	if why := m.editCheck(qid, ""); why != "" {
		return m.refuse("move", why)
	}
	i := m.queueIndex()
	switch {
	case len(items) == 0:
		return m.refuse("move", "the queue is empty")
	case key != "J" && i == 0:
		return m.refuse("move", "already first")
	case key == "J" && i == len(items)-1 && total > len(items):
		return m.refuse("move", "the next place is past the items shown")
	case key == "J" && i == len(items)-1:
		return m.refuse("move", "already last")
	}
	n, item := fmt.Sprint(i+1), items[i]
	switch key {
	case "K":
		return m.queueRun("move", i-1, "reorder", qid, n, fmt.Sprint(i), "--item", item)
	case "J":
		return m.queueRun("move", i+1, "reorder", qid, n, fmt.Sprint(i+2), "--item", item)
	}
	return m.queueRun("move", 0, "top", qid, n, "--item", item)
}

func (m model) unpinRun() (tea.Model, tea.Cmd) {
	_, _, qid, _ := m.queueInfo()
	if why := m.editCheck(qid, ""); why != "" {
		return m.refuse("unpin", why)
	}
	return m.queueRun("unpin", -1, "unpin", qid)
}

// promptOpen starts typing the text for an add or a pin.
func (m model) promptOpen(verb string) (tea.Model, tea.Cmd) {
	_, _, qid, _ := m.queueInfo()
	if why := m.editCheck(qid, ""); why != "" {
		return m.refuse(verb, why)
	}
	m.msg = "" // an older message would hide the prompt
	m.prompt = &promptState{verb: verb, qid: qid}
	return m, nil
}

// promptKey handles keys while text is being typed. Printable keys are text, even the ones that
// are commands otherwise.
func (m model) promptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := *m.prompt
	switch key := msg.String(); key {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.prompt = nil
		return m, nil
	case "enter":
		return m.promptSubmit()
	case "backspace":
		if r := []rune(p.text); len(r) > 0 {
			p.text = string(r[:len(r)-1])
		}
	case "space":
		p.text = addText(p.text, " ", maxPrompt)
	default:
		if msg.Text != "" && !strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") {
			p.text = addText(p.text, msg.Text, maxPrompt)
		}
	}
	m.prompt = &p
	return m, nil
}

func (m model) promptSubmit() (tea.Model, tea.Cmd) {
	p := *m.prompt
	m.prompt = nil
	text := strings.TrimSpace(p.text)
	switch {
	case text == "":
		return m.refuse(p.verb, "nothing to "+p.verb)
	}
	if why := m.editCheck(p.qid, ""); why != "" {
		return m.refuse(p.verb, why)
	}
	return m.queueRun(p.verb, -1, p.verb, p.qid, text)
}

// queueRun runs one fixed sitrep command. Its answer is a queueMsg. pend is the item to select
// when it worked, or -1.
func (m model) queueRun(verb string, pend int, args ...string) (tea.Model, tea.Cmd) {
	m.acting, m.pendSel = true, pend
	run, bin := m.run, m.bin
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), queueTimeout)
		defer cancel()
		out, err := run(ctx, bin, args...)
		return queueMsg{verb: verb, text: out, err: err}
	}
}

// queueResult takes the answer. Whatever happened, the next snapshot must start after it, so an
// older one that is still on its way is dropped.
func (m model) queueResult(msg queueMsg) (tea.Model, tea.Cmd) {
	m.acting = false
	if msg.err != nil {
		m.say(msg.verb + ": " + clean(msg.err.Error()))
	} else {
		if lines := pruneLines(msg.text); len(lines) > 0 {
			m.say(lines[len(lines)-1])
		}
		if m.pendSel >= 0 {
			m.qSel, m.qText = m.pendSel, ""
		}
	}
	if m.fetching {
		m.refetch = true
		return m, nil
	}
	m.fetching, m.started = true, m.started+1
	return m, m.fetchCmd()
}

// queueView is what the queue screen draws.
type queueView struct {
	Name     string
	Items    []string
	Total    int
	Sel      int
	Inbox    bool
	Target   string // where `t` takes an item
	Ask      string // the question, when one is open
	Prompt   string // the label of the line being typed, when one is open
	Typed    string
	Status   string
	ReadOnly bool
}

func (m model) queueView() queueView {
	items, total, _, x := m.queueInfo()
	v := queueView{Items: items, Total: total, Sel: m.queueIndex(), Inbox: m.qKind == qInbox, Status: m.statusText(), ReadOnly: m.readOnly}
	switch {
	case v.Inbox:
		v.Name = "Inbox"
		if t := m.session(m.qTo); t != nil {
			v.Target = t.Short + " " + t.Dir
		}
	case x != nil:
		v.Name = "Queue " + x.Short + " " + x.Dir
	default:
		v.Name = "Queue (the session is gone)"
	}
	if m.ask != nil {
		v.Ask = fmt.Sprintf("remove item %d?", m.ask.n)
		if m.ask.verb == "take" {
			to := m.ask.target
			if t := m.session(to); t != nil {
				to = t.Short + " " + t.Dir
			}
			v.Ask = fmt.Sprintf("move item %d to %s?", m.ask.n, to)
		}
	}
	if m.prompt != nil {
		v.Prompt, v.Typed = m.prompt.verb, m.prompt.text
	}
	return v
}

// queueScreen draws the title, the items from the one that keeps the selection in view, and a
// bar: the line being typed, else the question, else the last message, else the keys.
func queueScreen(v queueView, width, height int, th Theme) string {
	width = max(width, 1)
	body := detailBody(height)
	const gutterW = 2
	digits := len(fmt.Sprint(len(v.Items)))
	var lines []string
	start, end := 0, 0
	for i, it := range v.Items {
		num := fmt.Sprintf("%*d. ", digits, i+1)
		avail := max(width-gutterW-ansi.StringWidth(num), 10)
		if i == v.Sel {
			start = len(lines)
		}
		for j, l := range wrapText(it, avail, 1<<20) {
			mark, label := "  ", strings.Repeat(" ", ansi.StringWidth(num))
			if j == 0 {
				label = num
				if i == v.Sel {
					mark = "▸ "
				}
			}
			lines = append(lines, mark+label+l)
		}
		if i == v.Sel {
			end = len(lines)
		}
	}
	if len(v.Items) == 0 {
		lines = []string{"  the " + strings.ToLower(strings.Fields(v.Name)[0]) + " is empty"}
	}
	top := min(max(end-body, 0), start)
	view := append([]string{}, lines[top:min(top+body, len(lines))]...)
	for i, l := range view {
		view[i] = ansi.Truncate(l, width, "…")
	}
	for height > 0 && len(view) < body {
		view = append(view, "")
	}
	hint := " esc back  up/down  a add  x remove  K J T move  "
	switch {
	case v.ReadOnly:
		hint = " esc back  up/down  (read-only) "
	case v.Inbox && v.Target != "":
		hint += "t take into " + v.Target + " "
	case v.Inbox:
		hint += "(no session to take into) "
	default:
		hint += "P pin  U unpin "
	}
	switch {
	case v.Prompt != "":
		hint = " " + v.Prompt + ": " + v.Typed + "▏"
		if over := ansi.StringWidth(hint) - width; over > 0 { // keep the end, where the typing is
			hint = ansi.TruncateLeft(hint, over+1, "…")
		}
	case v.Ask != "":
		hint = " y/n  " + v.Ask + " "
	case v.Status != "":
		hint = " " + v.Status + " "
	}
	count := queueWord(len(v.Items))
	if v.Total > len(v.Items) {
		count = fmt.Sprintf("the first %d of %d", len(v.Items), v.Total)
	}
	title := th.Label.Render(ansi.Truncate(" "+v.Name+" ("+count+")", width, "…"))
	bar := th.Status.Width(width).Render(ansi.Truncate(hint, width, ""))
	switch height {
	case 1:
		return bar
	case 2:
		return strings.Join(append(view, bar), "\n")
	}
	return strings.Join(append(append([]string{title}, view...), bar), "\n")
}
