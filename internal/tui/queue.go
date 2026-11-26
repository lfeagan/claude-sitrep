package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lfeagan/claude-sitrep/internal/state"
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
	verb   string // "take", "remove" or "alias"
	qid    string // the queue the item is in: inbox, or a session id; for alias, the session that gets the name
	n      int    // its position,
	item   string // and its text; for alias, the name
	target string // for take: the session that gets it; for alias, the ended session that has the name
}

// moverState is the list of places to send the selected item to: the inbox and the running sessions, but not where the item is. It holds
// the item's position and its text, which the move command checks, so that a queue that changed meanwhile is never moved at the wrong place.
type moverState struct {
	from  string // the queue the item is in
	n     int    // its position, 1-based,
	item  string // and its text
	dests []moveDest
	sel   int
}

type moveDest struct {
	id    string // inbox, or a session id
	label string // what is drawn
}

// promptState is a line being typed for `a` (add), `P` (pin), `E` (edit) or `n` (name). An edit also holds the item it is for and the text
// that item had, which the command checks, so that a queue that changed meanwhile is never edited at the wrong place.
type promptState struct {
	verb string
	qid  string
	text string
	n    int    // the item, 1-based
	old  string // its text when the edit began
	// fresh is true while the text is the one the prompt opened with (the current name), which the first typed character replaces. A
	// key that edits it, backspace or ctrl+u or ctrl+w, makes it the user's own.
	fresh bool
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
	if mv := m.mover; mv != nil {
		if qi, ok := m.itemsOf(mv.from); !ok || mv.n < 1 || mv.n > len(qi) || qi[mv.n-1] != mv.item {
			m.mover = nil
			m.sayErr("move: the queue changed")
		}
	}
	if m.ask == nil {
		return
	}
	a := *m.ask
	if a.verb == "alias" { // not about an item: the session is still there, and the other one has still ended and still has the name
		if x, h := m.session(a.qid), m.session(a.target); x == nil || h == nil || h.Running || h.Alias != a.item {
			m.ask = nil
			m.sayErr("alias: the sessions changed")
		}
		return
	}
	qi, ok := m.itemsOf(a.qid)
	switch {
	case !ok || (a.target != "" && !m.running(a.target)):
		m.ask = nil
		m.sayErr(a.verb + ": the session is gone")
	case a.n < 1 || a.n > len(qi) || qi[a.n-1] != a.item:
		m.ask = nil
		m.sayErr(a.verb + ": the queue changed")
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
		m.sayErr("queue: no session is selected")
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
	case m.settling:
		return "refreshing"
	case m.readOnly:
		return "the dashboard is read-only"
	}
	for _, id := range []string{qid, target} {
		switch x := m.session(id); {
		case id == "inbox" || id == "":
		case x == nil:
			return "the session is gone"
		case !x.Running:
			return x.Label() + " has ended"
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
		return m.askKey(key)
	}
	if m.mover != nil {
		return m.moverKey(msg)
	}
	last, i := max(len(m.queueItems())-1, 0), m.queueIndex()
	switch key {
	case "q", "esc", "backspace": // q goes back; only ctrl+c quits from here
		m.queue, m.msg = false, "" // what the queue said is not news on the tabs
	case "i": // the key of the queue on screen closes it, and the other one switches to its queue
		if m.qKind == qInbox {
			m.queue, m.msg = false, ""
		} else {
			return m.queueOpen(qInbox)
		}
	case "e":
		if m.qKind == qSession {
			m.queue, m.msg = false, ""
		} else {
			return m.queueOpen(qSession)
		}
	case "up", "k":
		m.selectQueue(i - 1)
	case "down", "j":
		m.selectQueue(i + 1)
	case "home":
		m.selectQueue(0)
	case "end", "G":
		m.selectQueue(last)
	case "pgdown", "ctrl+d":
		m.selectQueue(i + max(m.height-3, 1))
	case "pgup", "ctrl+u":
		m.selectQueue(i - max(m.height-3, 1))
	case "?":
		m.help = true
	case "enter":
		return m.editOpen()
	case "t":
		if m.qKind == qInbox {
			return m.takeStart()
		}
	case "a", "o": // o is vi's open a line; a stays, so what was learned still works
		return m.promptOpen("add")
	case "E":
		return m.editOpen()
	case "m":
		return m.moveOpen()
	case "P":
		if m.qKind == qSession {
			return m.promptOpen("pin")
		}
	case "U":
		if m.qKind == qSession {
			return m.unpinRun()
		}
	case "x", "delete": // dd is the way to remove an item; x and the delete key still work
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
	m.sayErr(verb + ": " + why)
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
	if a.verb == "alias" {
		if why := m.aliasCheck(m.session(a.qid)); why != "" {
			return m.refuse("alias", why)
		}
		return m.queueRun("alias", -1, "alias", a.qid, a.item, "--take")
	}
	if why := m.editCheck(a.qid, a.target); why != "" {
		return m.refuse(a.verb, why)
	}
	n := fmt.Sprint(a.n)
	if a.verb == "take" {
		return m.queueRun("take", a.n-1, "move", "inbox", a.target, n, "--item", a.item)
	}
	return m.queueRun("remove", a.n-1, "rm", a.qid, n, "--item", a.item)
}

// askKey answers the open question: only y, n and cancelling do anything, so a held key cannot start a second change.
func (m model) askKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "y":
		return m.askRun()
	case "n", "esc", "q":
		m.ask = nil
	}
	return m, nil
}

// takeDigit is a number on the Cards or Table tab: it asks whether to move that item of the inbox to the end of the selected session's queue.
// What it asks, and runs, is what t does in the inbox, so the same checks and the same guard apply.
func (m model) takeDigit(n int) (tea.Model, tea.Cmd) {
	x := m.selected()
	if x == nil {
		return m.refuse("take", "no session is selected")
	}
	if why := m.editCheck("inbox", x.ID); why != "" {
		return m.refuse("take", why)
	}
	items := m.snap.Inbox.Items
	if n < 1 || n > len(items) {
		return m.refuse("take", fmt.Sprintf("no inbox item %d", n))
	}
	m.msg = ""
	m.ask = &askState{verb: "take", qid: "inbox", n: n, item: items[n-1], target: x.ID}
	return m, nil
}

// takeStatus is the question on the status bar of a tab.
func (m model) takeStatus() string {
	if a := m.ask; a.verb == "alias" {
		who, from := a.qid, a.target
		if x := m.session(a.qid); x != nil {
			who = x.Short
		}
		if h := m.session(a.target); h != nil {
			from = h.Short
		}
		return fmt.Sprintf("y/n  name %s %s, taking it from %s, which has ended?", who, a.item, from)
	}
	to := m.ask.target
	if t := m.session(to); t != nil {
		to = t.Label() + " " + t.Dir
	}
	return fmt.Sprintf("y/n  move inbox item %d to %s?  %s", m.ask.n, to, m.ask.item)
}

// moveOpen opens the list of places to send the selected item to. It is for the rare move between queues, so it is a short list to choose
// from and not a key for each place.
func (m model) moveOpen() (tea.Model, tea.Cmd) {
	items, _, qid, _ := m.queueInfo()
	if why := m.editCheck(qid, ""); why != "" {
		return m.refuse("move", why)
	}
	if len(items) == 0 {
		return m.refuse("move", "the queue is empty")
	}
	var dests []moveDest
	if qid != "inbox" {
		dests = append(dests, moveDest{id: "inbox", label: "Inbox"})
	}
	for _, x := range m.snap.Sessions {
		if x.Running && x.ID != qid && idRe.MatchString(x.ID) {
			dests = append(dests, moveDest{id: x.ID, label: x.Label() + "  " + x.Dir})
		}
	}
	if len(dests) == 0 {
		return m.refuse("move", "there is no other queue to send it to")
	}
	i := m.queueIndex()
	m.msg = ""
	m.mover = &moverState{from: qid, n: i + 1, item: items[i], dests: dests}
	return m, nil
}

// moverKey handles keys while the list of places is open: choose with the movement keys, enter sends, esc or q cancels.
func (m model) moverKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	v := *m.mover
	last := len(v.dests) - 1
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "backspace":
		m.mover = nil
		return m, nil
	case "?":
		m.help = true
	case "up", "k":
		v.sel = max(v.sel-1, 0)
	case "down", "j":
		v.sel = min(v.sel+1, last)
	case "home":
		v.sel = 0
	case "end", "G":
		v.sel = last
	case "enter":
		m.mover = nil
		d := v.dests[v.sel]
		if why := m.editCheck(v.from, d.id); why != "" {
			return m.refuse("move", why)
		}
		return m.queueRun("move", v.n-1, "move", v.from, d.id, fmt.Sprint(v.n), "--item", v.item)
	}
	m.mover = &v
	return m, nil
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

// editOpen starts editing the text of the selected item, which the line starts with.
func (m model) editOpen() (tea.Model, tea.Cmd) {
	items, _, qid, _ := m.queueInfo()
	if why := m.editCheck(qid, ""); why != "" {
		return m.refuse("edit", why)
	}
	if len(items) == 0 {
		return m.refuse("edit", "the queue is empty")
	}
	i := m.queueIndex()
	m.msg = ""
	m.prompt = &promptState{verb: "edit", qid: qid, n: i + 1, old: items[i], text: items[i]}
	return m, nil
}

// aliasOpen starts typing a name for the selected session, with the name it has already. It works on an ended session too: the name is
// a file, and nothing about the session needs to be alive.
func (m model) aliasOpen() (tea.Model, tea.Cmd) {
	x := m.selected()
	if x == nil {
		return m.refuse("alias", "no session is selected")
	}
	if why := m.aliasCheck(x); why != "" {
		return m.refuse("alias", why)
	}
	m.msg = ""
	m.prompt = &promptState{verb: "alias", qid: x.ID, text: x.Alias, fresh: x.Alias != ""}
	return m, nil
}

// aliasCheck says why a session cannot be named now, or "".
func (m model) aliasCheck(x *Session) string {
	switch {
	case x == nil:
		return "the session is gone"
	case m.acting:
		return "busy"
	case m.settling:
		return "refreshing"
	case m.readOnly:
		return "the dashboard is read-only"
	case !idRe.MatchString(x.ID):
		return "this session's id looks unusual, so it is left alone"
	}
	return ""
}

// aliasStatus is the line shown while a name is typed.
func (m model) aliasStatus() string {
	label := m.prompt.qid
	if x := m.session(m.prompt.qid); x != nil {
		label = x.Label()
		if x.Alias != "" && x.Short != "" {
			label += " (" + x.Short + ")"
		} else if x.Alias == "" {
			label = x.Short
		}
	}
	return fmt.Sprintf("name for %s: %s▏  enter sets, empty clears, esc cancels", label, m.prompt.text)
}

// freshText is the text to add to: none while the text is still the one the prompt opened with.
func (p promptState) freshText() string {
	if p.fresh {
		return ""
	}
	return p.text
}

// dropWord is the text without its last word and the spaces before it, as ctrl+w does in a shell.
func dropWord(s string) string {
	s = strings.TrimRight(s, " ")
	if i := strings.LastIndex(s, " "); i >= 0 {
		return s[:i+1]
	}
	return ""
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
		p.fresh = false
	case "ctrl+u":
		p.text, p.fresh = "", false
	case "ctrl+w":
		p.text, p.fresh = dropWord(p.text), false
	case "space":
		p.text, p.fresh = addText(p.freshText(), " ", maxPrompt), false
	default:
		if msg.Text != "" && !strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") {
			p.text, p.fresh = addText(p.freshText(), msg.Text, maxPrompt), false
		}
	}
	m.prompt = &p
	return m, nil
}

func (m model) promptSubmit() (tea.Model, tea.Cmd) {
	p := *m.prompt
	m.prompt = nil
	text := strings.TrimSpace(p.text)
	if p.verb == "alias" {
		x := m.session(p.qid)
		if why := m.aliasCheck(x); why != "" {
			return m.refuse("alias", why)
		}
		switch {
		case text != "" && !state.ValidAlias(text): // the same grammar as the command, said here so that the first try is the last
			return m.refuse("alias", "a name is lowercase letters, digits and hyphens, starting with a letter (32 at most)")
		case text == "" && x.Alias == "":
			return m.refuse("alias", "nothing to set")
		case text == "":
			return m.queueRun("alias", -1, "alias", p.qid, "--off")
		}
		for _, o := range m.snap.Sessions { // a name that another session has: from an ended one it can be taken, once asked
			if o.ID == p.qid || o.Alias != text {
				continue
			}
			if o.Running {
				return m.refuse("alias", text+" is the name of "+o.Label()+", which is running")
			}
			m.ask = &askState{verb: "alias", qid: p.qid, item: text, target: o.ID}
			return m, nil
		}
		return m.queueRun("alias", -1, "alias", p.qid, text)
	}
	switch {
	case text == "":
		return m.refuse(p.verb, "nothing to "+p.verb)
	case p.verb == "edit" && text == p.old:
		return m.refuse("edit", "unchanged")
	}
	if why := m.editCheck(p.qid, ""); why != "" {
		return m.refuse(p.verb, why)
	}
	if p.verb == "edit" {
		return m.queueRun("edit", p.n-1, "edit", p.qid, fmt.Sprint(p.n), text, "--item", p.old)
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
	m.acting, m.settling = false, true
	if msg.err != nil {
		m.sayErr(msg.verb + ": " + clean(msg.err.Error()))
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
	Hints    []string // the keys of the screen, when the list is not the queue's own
}

func (m model) queueView() queueView {
	items, total, _, x := m.queueInfo()
	v := queueView{Items: items, Total: total, Sel: m.queueIndex(), Inbox: m.qKind == qInbox, Status: m.statusText(), ReadOnly: m.readOnly}
	switch {
	case v.Inbox:
		v.Name = "Inbox"
		if t := m.session(m.qTo); t != nil {
			v.Target = t.Label() + " " + t.Dir
		}
	case x != nil:
		v.Name = "Queue " + x.Label() + " " + x.Dir
	default:
		v.Name = "Queue (the session is gone)"
	}
	if m.ask != nil {
		v.Ask = fmt.Sprintf("remove item %d?", m.ask.n)
		if m.ask.verb == "take" {
			to := m.ask.target
			if t := m.session(to); t != nil {
				to = t.Label() + " " + t.Dir
			}
			v.Ask = fmt.Sprintf("move item %d to %s?", m.ask.n, to)
		}
	}
	if m.prompt != nil {
		v.Prompt, v.Typed = m.prompt.verb, m.prompt.text
	}
	if mv := m.mover; mv != nil { // the places to choose from take the place of the items
		v.Name, v.Items, v.Total, v.Sel, v.Inbox = fmt.Sprintf("Send item %d to", mv.n), nil, len(mv.dests), mv.sel, false
		for _, d := range mv.dests {
			v.Items = append(v.Items, d.label)
		}
		v.Hints = []string{"enter send here", "j/k choose", "esc cancel", "? help"}
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
		wrapped := wrapText(it, avail, 1<<20)
		for j, l := range wrapped {
			mark, label := "  ", strings.Repeat(" ", ansi.StringWidth(num))
			if j == 0 {
				label = th.Head.Render(strings.TrimRight(num, " ")) + " "
				if i == v.Sel {
					mark = "▸ "
				}
			}
			lines = append(lines, mark+label+l)
		}
		if i == v.Sel {
			end = len(lines)
		}
		if len(wrapped) > 1 && i < len(v.Items)-1 { // an item that took several lines is set apart, so that the next number is not mistaken for text
			lines = append(lines, "")
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
	// the most useful keys first: a narrow screen drops from the end, and never the help
	var hints []string
	switch {
	case v.Hints != nil:
		hints = v.Hints
	case v.ReadOnly:
		hints = []string{"q back", "j/k move", "(read-only)", "? help"}
	case v.Inbox && v.Target != "":
		hints = []string{"q back", "t take into " + v.Target, "o add", "enter edit", "dd remove", "m send", "K J T move", "e session queue", "? help"}
	case v.Inbox:
		hints = []string{"q back", "o add", "enter edit", "dd remove", "m send", "K J T move", "(no session to take into)", "? help"}
	default:
		hints = []string{"q back", "o add", "enter edit", "dd remove", "m send", "K J T move", "P pin", "U unpin", "i inbox", "? help"}
	}
	hint := fitHints(hints, width)
	switch {
	case v.Prompt != "":
		hint = " " + v.Prompt + ": " + v.Typed + "▏"
		if over := ansi.StringWidth(hint) - width; over > 0 { // keep the end, where the typing is
			hint = ansi.TruncateLeft(hint, over+1, "…")
		}
	case v.Ask != "":
		hint = " y/n  " + v.Ask + " "
	case v.Status != "":
		msg := " " + v.Status + " "
		rest := fitHints(hints, width-ansi.StringWidth(msg)-1)
		hint = msg
		if rest != "" {
			hint = msg + strings.Repeat(" ", width-ansi.StringWidth(msg)-ansi.StringWidth(rest)) + rest
		}
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
