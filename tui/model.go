package main

import (
	"context"
	"fmt"
	"regexp"
	"time"

	tea "charm.land/bubbletea/v2"
)

const fetchTimeout = 5 * time.Second

type tickMsg time.Time

type snapMsg struct {
	snap Snapshot
	err  error
}

// jumpMsg is the answer to a jump: text for the status bar, or an error.
type jumpMsg struct {
	text string
	err  error
}

const messageTTL = 5 * time.Second

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
	selID     string // the selected session, by id, so a refresh or a re-sort does not move it
	run       runner // runs tmux, ps, cmux and sitrep; tests replace it
	readOnly  bool   // no key changes any state
	prune     int    // the phase of the prune screen
	pruneText []string
	acting    bool   // a jump is running, so another is ignored
	quitJump  bool   // leave the dashboard after a good jump, for a popup
	msg       string // the last thing an action said
	msgAt     time.Time
	detail    bool // the detail pane is open for the selected session
	scroll    int  // the top line of the detail pane
	sortBy    int
	help      bool
	isDark    bool
	now       func() time.Time
}

func newModel(bin string, interval time.Duration, tab int) model {
	// fetching starts true because Init sends the first fetch; Init has a value receiver, so it cannot set it
	return model{bin: bin, interval: interval, tab: tab, width: 100, height: 30, isDark: true, now: time.Now, fetching: true, started: 1, run: realRunner}
}

func (m model) fetchCmd() tea.Cmd {
	bin := m.bin
	return func() tea.Msg {
		snap, err := fetch(context.Background(), bin, fetchTimeout)
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
	if m.msg != "" && m.now().Sub(m.msgAt) < messageTTL {
		return m.msg
	}
	return m.errText()
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
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "d", "backspace":
		m.detail = false
	case "enter":
		if cmd := m.jumpCmd(); cmd != nil {
			return m, cmd
		}
	case "y", "Y":
		return m.copyKey(msg.String() == "Y")
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
	case "end":
		m.scroll = maxScroll
	}
	return m, nil
}

// visible is the ids on the current tab, in drawing order.
func (m model) visible() []string { return visibleIDs(m.snap, m.tab, m.showEnded, m.sortBy) }

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
		if msg.err != nil {
			m.err = msg.err // the last good snapshot stays on screen
			return m, nil
		}
		m.snap, m.haveSnap, m.err, m.lastOK = msg.snap, true, nil, m.now()
		m.fixSel()
		m.clampScroll()
	case pruneMsg:
		return m.pruneResult(msg)
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
	case tea.KeyPressMsg:
		if m.prune != pruneNone {
			return m.pruneKey(msg)
		}
		if m.detail {
			return m.detailKey(msg)
		}
		if m.help { // any key closes the key list; ctrl+c still quits
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.help = false
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right":
			m.tab = (m.tab + 1) % tabCount
		case "shift+tab", "left":
			m.tab = (m.tab + tabCount - 1) % tabCount
		case "1":
			m.tab = tabWatch
		case "2":
			m.tab = tabTop
		case "up", "k":
			m.moveSel(-1)
		case "down", "j":
			m.moveSel(1)
		case "home":
			m.moveSelTo(0)
		case "end":
			m.moveSelTo(-1)
		case "enter":
			if cmd := m.jumpCmd(); cmd != nil {
				return m, cmd
			}
		case "y", "Y":
			return m.copyKey(msg.String() == "Y")
		case "p":
			return m.pruneStart()
		case "d", "space", " ":
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
	theme := newTheme(m.isDark, m.snap.Settings.HighlightSGR)
	body := "waiting for sitrep…"
	switch {
	case m.prune != pruneNone:
		body = pruneScreen(m.pruneText, m.prune, m.width, m.height, theme)
	case m.detail && m.selected() != nil:
		body = detailScreen(*m.selected(), m.width, m.height, m.scroll, theme)
	case m.help:
		body = helpScreen(m.width, m.height, m.readOnly, theme)
	case m.haveSnap || m.err != nil:
		body = render(m.snap, m.tab, m.statusText(), viewOpts{Width: m.width, Height: m.height, ShowEnded: m.showEnded, Sort: m.sortBy, Sel: m.selID, Theme: theme})
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}
