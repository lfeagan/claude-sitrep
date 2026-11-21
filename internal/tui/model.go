package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const fetchTimeout = 5 * time.Second

type tickMsg time.Time

type snapMsg struct {
	snap Snapshot
	err  error
}

// snoozeMsg is the answer to `sitrep snooze`.
type snoozeMsg struct {
	text string
	err  error
}

// jumpMsg is the answer to a jump: text for the status bar, or an error.
type jumpMsg struct {
	text string
	err  error
}

const (
	messageTTL    = 5 * time.Second
	snoozeTimeout = 5 * time.Second
)

// say puts a message on the status bar for a few seconds.
func (m *model) say(text string) {
	m.msg, m.msgAt = text, m.now()
}

type model struct {
	bin       string
	interval  time.Duration
	snap      Snapshot
	haveSnap  bool
	err       error
	lastOK    time.Time
	fetching  bool
	started   int // fetches started, so tests can check that none overlap
	width     int
	height    int
	tab       int
	showEnded bool
	selID     string                                      // the selected session, by id, so a refresh or a re-sort does not move it
	filter    string                                      // only sessions that contain this text are listed
	filtering bool                                        // keys type into the filter
	run       runner                                      // runs tmux, ps, cmux and sitrep; tests replace it
	fetcher   func(ctx context.Context) (Snapshot, error) // reads the sessions
	readOnly  bool                                        // no key changes any state
	bell      bool                                        // ring the terminal bell when a session starts needing the user
	prune     int                                         // the phase of the prune screen
	pruneText []string
	acting    bool   // a jump is running, so another is ignored
	quitJump  bool   // leave the dashboard after a good jump, for a popup
	msg       string // the last thing an action said
	msgAt     time.Time
	detail    bool         // the detail pane is open for the selected session
	queue     bool         // the queue view is open
	qKind     int          // qInbox or qSession
	qTo       string       // the session it shows, or for the inbox the one `t` hands items to
	qSel      int          // the selected item, by position,
	qText     string       // and by text, so it stays on the same item when the list changes
	pendSel   int          // the item to select when the running change worked, or -1
	refetch   bool         // a change finished while a fetch was running: that fetch is stale
	settling  bool         // a change finished and its snapshot has not arrived, so the list may be old
	ask       *askState    // the open question, if any
	prompt    *promptState // the open line of typing, if any
	mover     *moverState  // the open list of places to send an item to, if any
	scroll    int          // the top line of the detail pane
	sortBy    int
	help      bool
	pending   string // the first key of a two-key command that waits for its second: g (gg, the first row) or d (dd, remove an item)
	isDark    bool
	now       func() time.Time
}

func newModel(bin string, interval time.Duration, tab int) model {
	// fetching starts true because Init sends the first fetch; Init has a value receiver, so it cannot set it
	return model{bin: bin, interval: interval, tab: tab, width: 100, height: 30, isDark: true, now: time.Now, fetching: true, started: 1, run: realRunner, bell: true}
}

func (m model) fetchCmd() tea.Cmd {
	fetch := m.fetcher
	return func() tea.Msg {
		if fetch == nil {
			return snapMsg{err: errors.New("no source of sessions")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		snap, err := fetch(ctx)
		return snapMsg{snap: snap, err: err}
	}
}

func (m model) tickCmd() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// pruneStart asks for a preview of what prune would do. It refuses when it could not work.
func (m model) pruneStart() (tea.Model, tea.Cmd) {
	switch {
	case m.readOnly:
		m.say("prune: the dashboard is read-only")
	case m.haveSnap && m.snap.Registry != "ok":
		m.say("prune: Claude Code's live-session registry cannot be read")
	default:
		m.prune, m.pruneText = pruneLoading, nil
		run, bin := m.run, m.bin
		return m, func() tea.Msg { return runPrune(bin, pruneArgsPreview, previewTimeout, false, run)() }
	}
	return m, nil
}

// pruneKey handles keys while the prune screen is open. Only y, n and cancelling do anything,
// and nothing does anything while a command runs, so a held key cannot start a second prune.
func (m model) pruneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.prune != pruneConfirm {
		return m, nil
	}
	run, bin := m.run, m.bin
	switch key {
	case "y":
		m.prune = pruneBusy
		return m, func() tea.Msg { return runPrune(bin, pruneArgsMove, pruneTimeout, true, run)() }
	case "n":
		m.prune = pruneBusy
		return m, func() tea.Msg { return runPrune(bin, pruneArgsKeep, pruneTimeout, true, run)() }
	case "esc", "q":
		m.prune, m.pruneText = pruneNone, nil
	}
	return m, nil
}

// pruneResult takes the answer of a preview or of the real prune.
func (m model) pruneResult(msg pruneMsg) (tea.Model, tea.Cmd) {
	lines := pruneLines(msg.text)
	switch {
	case msg.err != nil:
		m.prune, m.pruneText = pruneNone, nil
		m.say("prune: " + msg.err.Error())
	case !msg.real && nothingToPrune(lines):
		m.prune, m.pruneText = pruneNone, nil
		m.say("nothing to prune")
	case !msg.real:
		m.prune, m.pruneText = pruneConfirm, lines
	default:
		m.prune, m.pruneText = pruneNone, nil
		if len(lines) > 0 {
			m.say(lines[len(lines)-1])
		}
		if !m.fetching { // show the result without waiting for the next tick
			m.fetching, m.started = true, m.started+1
			return m, m.fetchCmd()
		}
	}
	return m, nil
}

// idRe is what a session id may look like. It is checked before an id goes on the clipboard,
// because the resume command is meant to be pasted into a shell.
var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// copyKey puts the selected session's id, or its `claude --resume` command, on the clipboard
// through the terminal. The text is also on the status bar, for a terminal that ignores it.
func (m model) copyKey(resume bool) (tea.Model, tea.Cmd) {
	x := m.selected()
	switch {
	case x == nil:
		return m, nil
	case !idRe.MatchString(x.ID):
		m.say("copy: this session's id looks unusual, so it is not copied")
		return m, nil
	}
	text, what := x.ID, "session id"
	if resume {
		text, what = "claude --resume "+x.ID, "command"
	}
	m.say("copied the " + what + ": " + text)
	return m, tea.SetClipboard(text)
}

// snoozeKey mutes the selected session's needs-you state, or ends the mute if it is already
// on. It runs a fixed `sitrep snooze` command, so it is off in read-only mode.
func (m model) snoozeKey() (tea.Model, tea.Cmd) {
	x := m.selected()
	switch {
	case x == nil || m.acting:
		return m, nil
	case m.readOnly:
		m.say("snooze: the dashboard is read-only")
		return m, nil
	case !idRe.MatchString(x.ID):
		m.say("snooze: this session's id looks unusual, so it is left alone")
		return m, nil
	}
	args := []string{"snooze", x.ID}
	if x.Snoozed {
		args = []string{"snooze", "--off", x.ID}
	}
	m.acting = true
	run, bin := m.run, m.bin
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), snoozeTimeout)
		defer cancel()
		out, err := run(ctx, bin, args...)
		return snoozeMsg{text: out, err: err}
	}
}

// jumpCmd starts a jump to the selected session's tmux pane, unless one is already running.
func (m *model) jumpCmd() tea.Cmd {
	x := m.selected()
	if x == nil || m.acting {
		return nil
	}
	m.acting = true
	sess, run, env := *x, m.run, currentEnv()
	return func() tea.Msg {
		text, err := jump(context.Background(), sess, env, run)
		return jumpMsg{text: text, err: err}
	}
}

// statusText is what the status bar says instead of the counts: a fresh message from an
// action, else the last refresh error.
func (m model) statusText() string {
	if m.pending != "" {
		return m.pending + "-"
	}
	if m.msg != "" && m.now().Sub(m.msgAt) < messageTTL {
		return m.msg
	}
	return m.errText()
}

const maxFilter = 64

// typeFilter adds text to the filter, cleaned and on one line, up to a limit.
func (m *model) typeFilter(text string) {
	m.filter = addText(m.filter, text, maxFilter)
	m.fixSel()
}

// filterKey handles keys while the filter is being typed. Printable keys are text, even the
// ones that are commands otherwise, so typing cannot quit or prune by accident.
func (m model) filterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key := msg.String(); key {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.filtering = false
	case "esc":
		m.filtering, m.filter = false, ""
		m.fixSel()
	case "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
			m.fixSel()
		}
	case "up":
		m.moveSel(-1)
	case "down":
		m.moveSel(1)
	case "space":
		m.typeFilter(" ")
	default:
		if msg.Text != "" && len(key) > 0 && !strings.HasPrefix(key, "ctrl+") && !strings.HasPrefix(key, "alt+") {
			m.typeFilter(msg.Text)
		}
	}
	return m, nil
}

// filterStatus is what the status bar says while a filter is on.
func (m model) filterStatus() string {
	list, _ := shown(m.snap, m.showEnded)
	cursor := ""
	if m.filtering {
		cursor = "▏"
	}
	return fmt.Sprintf("/%s%s  (%d of %d)", m.filter, cursor, len(filterSessions(list, m.filter)), len(list))
}

// selected is the selected session, or nil.
func (m model) selected() *Session {
	for i := range m.snap.Sessions {
		if m.snap.Sessions[i].ID == m.selID && m.selID != "" {
			return &m.snap.Sessions[i]
		}
	}
	return nil
}

// clampScroll keeps the detail pane's scroll inside the text, after a resize or a refresh.
func (m *model) clampScroll() {
	if x := m.selected(); m.detail && x != nil {
		m.scroll = min(max(m.scroll, 0), detailMax(*x, m.width, m.height))
	}
}

// detailKey handles keys while the detail pane is open: up and down change the session, and
// the page keys and the space bar scroll. Esc or d closes it.
func (m model) detailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	x := m.selected()
	if x == nil {
		m.detail = false
		return m, nil
	}
	maxScroll := detailMax(*x, m.width, m.height)
	m.scroll = min(m.scroll, maxScroll)
	page := max(detailBody(m.height)-1, 1)
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc", "v", "backspace": // q goes back; only ctrl+c quits from here
		m.detail = false
	case "enter":
		if cmd := m.jumpCmd(); cmd != nil {
			return m, cmd
		}
	case "y", "Y":
		return m.copyKey(msg.String() == "Y")
	case "z":
		return m.snoozeKey()
	case "up", "k":
		m.moveSel(-1)
		m.scroll = 0
	case "down", "j":
		m.moveSel(1)
		m.scroll = 0
	case "pgdown", "space", " ", "ctrl+d":
		m.scroll = min(m.scroll+page, maxScroll)
	case "pgup", "b", "ctrl+u":
		m.scroll = max(m.scroll-page, 0)
	case "home":
		m.scroll = 0
	case "end", "G":
		m.scroll = maxScroll
	case "?":
		m.help = true
	}
	return m, nil
}

// goTop is gg: the first row of the list on screen, or the top of the detail view.
func (m *model) goTop() {
	switch {
	case m.queue && m.mover != nil:
		v := *m.mover
		v.sel = 0
		m.mover = &v
	case m.queue:
		m.selectQueue(0)
	case m.detail:
		m.scroll = 0
	default:
		m.moveSelTo(0)
	}
}

// listPage is how many sessions ctrl+d and ctrl+u move: about a screen less a line, as the detail view pages. A session takes a few lines
// on the Watch tab and one on the Top tab.
func (m model) listPage() int {
	if m.tab == tabTop {
		return max(m.height-5, 1)
	}
	return max((m.height-5)/6, 1)
}

// visible is the ids on the current tab, in drawing order.
func (m model) visible() []string {
	return visibleIDs(m.snap, m.tab, m.showEnded, m.sortBy, m.filter)
}

// fixSel keeps the selection on a session that is still shown, else on the first one.
func (m *model) fixSel() {
	ids := m.visible()
	for _, id := range ids {
		if id == m.selID {
			return
		}
	}
	m.detail = false // the selected session is gone, so a detail pane would show another one
	m.selID = ""
	if len(ids) > 0 {
		m.selID = ids[0]
	}
}

func (m *model) moveSel(d int) {
	ids := m.visible()
	for i, id := range ids {
		if id == m.selID {
			m.selID = ids[min(max(i+d, 0), len(ids)-1)]
			return
		}
	}
	m.fixSel()
}

func (m *model) moveSelTo(i int) { // 0 for the first, -1 for the last
	if ids := m.visible(); len(ids) > 0 {
		m.selID = ids[0]
		if i < 0 {
			m.selID = ids[len(ids)-1]
		}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.tickCmd(), tea.RequestBackgroundColor)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1) // 0 would mean "no limit"
		m.clampScroll()
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
	case snapMsg:
		m.fetching = false
		if m.refetch { // this fetch began before a change finished, so it may show the old queue
			m.refetch, m.fetching, m.started = false, true, m.started+1
			return m, m.fetchCmd()
		}
		m.settling = false
		if msg.err != nil {
			m.err = msg.err // the last good snapshot stays on screen
			return m, nil
		}
		ring := m.bell && m.haveSnap && newlyNeedYou(m.snap, msg.snap) // never on the first load
		m.snap, m.haveSnap, m.err, m.lastOK = msg.snap, true, nil, m.now()
		m.fixSel()
		m.clampScroll()
		m.queueRefresh()
		if ring {
			return m, tea.Raw("\a")
		}
	case pruneMsg:
		return m.pruneResult(msg)
	case queueMsg:
		return m.queueResult(msg)
	case snoozeMsg:
		m.acting = false
		if msg.err != nil {
			m.say("snooze: " + msg.err.Error())
			return m, nil
		}
		if lines := pruneLines(msg.text); len(lines) > 0 {
			m.say(lines[len(lines)-1])
		}
		if !m.fetching { // show the change without waiting for the next tick
			m.fetching, m.started = true, m.started+1
			return m, m.fetchCmd()
		}
	case jumpMsg:
		m.acting = false
		m.msgAt = m.now()
		if msg.err != nil {
			m.msg = "jump: " + msg.err.Error()
			return m, nil
		}
		m.msg = msg.text
		if m.quitJump {
			return m, tea.Quit
		}
	case tickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		if !m.fetching { // never overlap two calls; a slow one just skips ticks
			m.fetching, m.started = true, m.started+1
			cmds = append(cmds, m.fetchCmd())
		}
		return m, tea.Batch(cmds...)
	case tea.PasteMsg:
		switch {
		case m.filtering: // pasted text goes into the filter, cleaned
			m.typeFilter(msg.Content)
		case m.prompt != nil:
			p := *m.prompt
			p.text = addText(p.text, msg.Content, maxPrompt)
			m.prompt = &p
		}
	case tea.KeyPressMsg:
		if m.filtering {
			return m.filterKey(msg)
		}
		if m.prune != pruneNone {
			return m.pruneKey(msg)
		}
		if m.prompt != nil {
			return m.promptKey(msg)
		}
		if m.help { // any key closes the key list, and what was open is open again; ctrl+c still quits
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.help = false
			return m, nil
		}
		if m.ask != nil && !m.queue { // the question a number on a tab asked
			return m.askKey(msg.String())
		}
		// one place holds the first key of a two-key command: gg goes to the first row, and dd (in a queue) removes the item. Any other key
		// is swallowed (g then x does nothing), as in vim. esc clears it too, and so does anything else, as the next key always clears it.
		if p := m.pending; p != "" {
			m.pending = ""
			switch key := msg.String(); {
			case key == "ctrl+c":
				return m, tea.Quit
			case key == "g" && p == "g":
				m.goTop()
			case key == "d" && p == "d":
				return m.removeStart()
			}
			return m, nil
		}
		switch key := msg.String(); {
		case key == "g" && m.ask == nil:
			m.pending = "g"
			return m, nil
		case key == "d" && m.queue && m.ask == nil && m.mover == nil:
			m.pending = "d"
			return m, nil
		}
		if m.queue {
			return m.queueKey(msg)
		}
		if m.detail {
			return m.detailKey(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % tabCount
		case "shift+tab", "left", "h":
			m.tab = (m.tab + tabCount - 1) % tabCount
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			return m.takeDigit(int(msg.String()[0] - '0'))
		case "up", "k":
			m.moveSel(-1)
		case "down", "j":
			m.moveSel(1)
		case "home":
			m.moveSelTo(0)
		case "end", "G":
			m.moveSelTo(-1)
		case "pgdown", "ctrl+d":
			m.moveSel(m.listPage())
		case "pgup", "ctrl+u":
			m.moveSel(-m.listPage())
		case "enter":
			if cmd := m.jumpCmd(); cmd != nil {
				return m, cmd
			}
		case "y", "Y":
			return m.copyKey(msg.String() == "Y")
		case "z":
			return m.snoozeKey()
		case "/":
			if !m.detail && !m.help {
				m.filtering = true
			}
		case "n":
			return m.aliasOpen()
		case "i":
			return m.queueOpen(qInbox)
		case "e":
			return m.queueOpen(qSession)
		case "esc":
			if m.filter != "" { // a filter left on is cleared by esc
				m.filter = ""
				m.fixSel()
			}
		case "p":
			return m.pruneStart()
		case "v", "space", " ":
			if m.selected() != nil {
				m.detail, m.scroll = true, 0
			}
		case "a":
			m.showEnded = !m.showEnded
			m.fixSel()
		case "s":
			if m.tab == tabTop { // only the Top view has a sort
				m.sortBy = (m.sortBy + 1) % sortCount
			}
		case "?":
			m.help = true
		case "r":
			if !m.fetching {
				m.fetching, m.started = true, m.started+1
				return m, m.fetchCmd()
			}
		}
		m.fixSel() // a tab switch, a sort or the ended toggle may change what is shown
	}
	return m, nil
}

// errText says what went wrong and how old the data on screen is.
func (m model) errText() string {
	if m.err == nil {
		return ""
	}
	if !m.haveSnap {
		return "error: " + m.err.Error()
	}
	return fmt.Sprintf("error: %s (showing data from %ds ago)", m.err.Error(), int(m.now().Sub(m.lastOK).Seconds()))
}

func (m model) View() tea.View {
	theme := themeFor(m.isDark, m.snap.Settings)
	body := "waiting for sitrep…"
	switch {
	case m.prune != pruneNone:
		body = pruneScreen(m.pruneText, m.prune, m.width, m.height, theme)
	case m.help:
		body = helpScreen(m.width, m.height, m.readOnly, theme)
	case m.queue:
		body = queueScreen(m.queueView(), m.width, m.height, theme)
	case m.detail && m.selected() != nil:
		body = detailScreenKeys(*m.selected(), m.width, m.height, m.scroll, m.pending == "g", theme)
	case m.haveSnap || m.err != nil:
		status := m.statusText()
		if m.prompt != nil && m.prompt.verb == "alias" {
			status = m.aliasStatus()
		}
		if m.ask != nil {
			status = m.takeStatus()
		}
		if status == "" && (m.filtering || m.filter != "") {
			status = m.filterStatus()
		}
		body = render(m.snap, m.tab, status, viewOpts{Width: m.width, Height: m.height, ShowEnded: m.showEnded, Sort: m.sortBy, Sel: m.selID, Filter: m.filter, Theme: theme})
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}
