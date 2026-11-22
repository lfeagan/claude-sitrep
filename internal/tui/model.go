package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
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
	m.msg, m.msgAt, m.msgErr = text, m.now(), false
}

// sayErr is say for an action that was refused or failed: the status bar draws it in the error colour.
func (m *model) sayErr(text string) {
	m.say(text)
	m.msgErr = true
}

// statusIsError says whether the status text is an error: a refused or failed action, or the last refresh error.
func (m model) statusIsError() bool {
	if m.pending != "" {
		return false
	}
	if m.msg != "" && m.now().Sub(m.msgAt) < messageTTL {
		return m.msgErr
	}
	return m.errText() != ""
}

type model struct {
	msgErr     bool // the message is an error
	bin        string
	interval   time.Duration
	snap       Snapshot
	haveSnap   bool
	err        error
	lastOK     time.Time
	fetching   bool
	started    int // fetches started, so tests can check that none overlap
	width      int
	height     int
	tab        int
	showEnded  bool
	selID      string                                      // the selected session, by id, so a refresh or a re-sort does not move it
	filter     string                                      // only sessions that contain this text are listed
	filtering  bool                                        // keys type into the filter
	run        runner                                      // runs tmux, ps, cmux and sitrep; tests replace it
	fetcher    func(ctx context.Context) (Snapshot, error) // reads the sessions
	readOnly   bool                                        // no key changes any state
	bell       bool                                        // ring the terminal bell when a session starts needing the user
	prune      int                                         // the phase of the prune screen
	pruneText  []string
	acting     bool   // a jump is running, so another is ignored
	quitJump   bool   // leave the dashboard after a good jump, for a popup
	msg        string // the last thing an action said
	msgAt      time.Time
	detail     bool         // the detail pane is open for the selected session
	queue      bool         // the queue view is open
	qKind      int          // qInbox or qSession
	qTo        string       // the session it shows, or for the inbox the one `t` hands items to
	qSel       int          // the selected item, by position,
	qText      string       // and by text, so it stays on the same item when the list changes
	pendSel    int          // the item to select when the running change worked, or -1
	refetch    bool         // a change finished while a fetch was running: that fetch is stale
	settling   bool         // a change finished and its snapshot has not arrived, so the list may be old
	ask        *askState    // the open question, if any
	prompt     *promptState // the open line of typing, if any
	mover      *moverState  // the open list of places to send an item to, if any
	scroll     int          // the top line of the detail pane
	sortBy     int
	help       bool
	helpAt     int    // the first line of the key list that is shown
	pending    string // the first key of a two-key command that waits for its second: g (gg, the first row) or d (dd, remove an item)
	isDark     bool
	noColour   bool // the terminal shows no colour (NO_COLOR, or a terminal that has none): marks carry what colour would
	now        func() time.Time
	wallSize   int // the tile size of the Wall tab, once s has chosen one
	wallSet    bool
	demo       bool           // the sessions are made up
	colorsOpts *ColorsOptions // what C opens, or nil
	colors     *colorsModel   // the colour tool, while it is open over the dashboard
	mouse      bool           // clicks select a row or open a tab, and a double click jumps
	clickID    string         // the session of the last click, and when it came,
	clickAt    time.Time      // for the double click
}

// doubleClick is the longest gap between two clicks on one row that counts as a double click.
const doubleClick = 400 * time.Millisecond

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
		m.sayErr("prune: the dashboard is read-only")
	case m.haveSnap && m.snap.Registry != "ok":
		m.sayErr("prune: Claude Code's live-session registry cannot be read")
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
		m.sayErr("prune: " + msg.err.Error())
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
		m.sayErr("copy: this session's id looks unusual, so it is not copied")
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
		m.sayErr("snooze: the dashboard is read-only")
		return m, nil
	case !idRe.MatchString(x.ID):
		m.sayErr("snooze: this session's id looks unusual, so it is left alone")
		return m, nil
	case !x.NeedsInput && !x.Snoozed:
		m.sayErr("snooze: " + x.Label() + " does not need you, so there is nothing to snooze")
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
	case "ctrl+u":
		m.filter = ""
		m.fixSel()
	case "ctrl+w":
		m.filter = dropWord(m.filter)
		m.fixSel()
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
	case "i":
		return m.queueOpen(qInbox) // closing the queue comes back to the detail view
	case "e":
		return m.queueOpen(qSession)
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

// wallNow is the tile size the Wall shows now.
func (m model) wallNow() int {
	o := viewOpts{Width: m.width, ShowEnded: m.showEnded, Filter: m.filter, Size: m.wallSize, Auto: !m.wallSet}
	if m.height > 0 {
		o.Height = max(m.height-len(tabBar(tabWall, m.width, plainTheme()))-1, 1)
	}
	return wallSizeFor(m.snap, o)
}

// listPage is how many sessions ctrl+d and ctrl+u move: about a screen less a line, as the detail view pages. A session takes a few lines
// on the Cards tab and one on the Table tab.
func (m model) listPage() int {
	switch m.tab {
	case tabTable:
		return max(m.height-5, 1)
	case tabWall:
		return wallPage(m.width, m.height, m.wallNow())
	}
	return max((m.height-5)/6, 1)
}

// atWallEdge says that nothing is left of the first tile (d is -1) or right of the last one (d is 1), so that the arrow goes on to the
// next tab: someone who walked into the Wall with the arrow keys can walk out again the same way.
func (m model) atWallEdge(d int) bool {
	ids := m.visible()
	if len(ids) == 0 {
		return true
	}
	if d < 0 {
		return ids[0] == m.selID
	}
	return ids[len(ids)-1] == m.selID
}

// enterWall, after an arrow key has brought the user to the Wall, selects the tile at the edge they came from (the first when they moved
// right, the last when they moved left), so that the same arrow key back walks out again at once. Other ways to a tab keep the selection.
func (m *model) enterWall(i int) {
	if m.tab == tabWall {
		m.moveSelTo(i)
	}
}

// rowStep is how many sessions j and k move: one, and on the Wall a row of tiles.
func (m model) rowStep() int {
	if m.tab == tabWall {
		return wallCols(m.width)
	}
	return 1
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
	if m.colors != nil {
		if next, cmd, handled := m.colorsUpdate(msg); handled {
			return next, cmd
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1) // 0 would mean "no limit"
		m.clampScroll()
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
	case tea.ColorProfileMsg:
		m.noColour = msg.Profile <= colorprofile.Ascii
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
			m.sayErr("snooze: " + msg.err.Error())
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
			p.text, p.fresh = addText(p.freshText(), msg.Content, maxPrompt), false
			m.prompt = &p
		}
	case tea.MouseClickMsg:
		return m.click(msg.Mouse())
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
			if d, ok := helpStep(msg.String(), m.height); ok {
				room := max(m.height-1, 1)
				m.helpAt = max(0, min(m.helpAt+d, max(len(helpLines(m.width, m.readOnly))-room, 0)))
				return m, nil
			}
			m.help, m.helpAt = false, 0
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
		case "tab":
			m.tab = (m.tab + 1) % tabCount
		case "shift+tab":
			m.tab = (m.tab + tabCount - 1) % tabCount
		case "right", "l":
			if m.tab == tabWall && !m.atWallEdge(1) { // the sessions are laid out in two directions there
				m.moveSel(1)
			} else {
				m.tab = (m.tab + 1) % tabCount
				m.enterWall(0)
			}
		case "left", "h":
			if m.tab == tabWall && !m.atWallEdge(-1) {
				m.moveSel(-1)
			} else {
				m.tab = (m.tab + tabCount - 1) % tabCount
				m.enterWall(-1)
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			return m.takeDigit(int(msg.String()[0] - '0'))
		case "up", "k":
			m.moveSel(-m.rowStep())
		case "down", "j":
			m.moveSel(m.rowStep())
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
		case "C":
			return m.colorsOpen()
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
			switch m.tab {
			case tabTable: // only the Table view has a sort
				m.sortBy = (m.sortBy + 1) % sortCount
			case tabWall:
				m.wallSize, m.wallSet = (m.wallNow()+1)%wallSizes, true
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

// click handles a click on a row of a tab: the first selects the session, a second on the same one within doubleClick jumps, as enter does.
// Nothing happens while a screen, a question or a line of typing is open, as no row is then on show.
func (m model) click(c tea.Mouse) (tea.Model, tea.Cmd) {
	if !m.mouse || c.Button != tea.MouseLeft || m.filtering || m.prune != pruneNone || m.prompt != nil || m.help || m.ask != nil ||
		m.queue || m.detail || m.mover != nil {
		return m, nil
	}
	if c.Y == 0 { // the tab bar: the name of a tab opens it
		if i := tabAt(c.X, themeFor(m.isDark, m.snap.Settings)); i >= 0 {
			m.tab, m.pending, m.clickID = i, "", ""
			m.fixSel()
		}
		return m, nil
	}
	if !m.haveSnap {
		return m, nil
	}
	var hits []string
	var cells []wallCell
	render(m.snap, m.tab, "", viewOpts{Width: m.width, Height: m.height, ShowEnded: m.showEnded, Sort: m.sortBy, Sel: m.selID, Filter: m.filter, Size: m.wallSize, Auto: !m.wallSet, Theme: plainTheme(), Hits: &hits, Cells: &cells})
	id := ""
	if m.tab == tabWall {
		for _, cell := range cells {
			if c.X >= cell.X0 && c.X <= cell.X1 && c.Y >= cell.Y0 && c.Y <= cell.Y1 {
				id = cell.ID
			}
		}
	} else if c.Y >= 0 && c.Y < len(hits) {
		id = hits[c.Y]
	}
	if id == "" {
		return m, nil
	}
	now := m.now()
	m.selID, m.pending = id, ""
	if id == m.clickID && now.Sub(m.clickAt) <= doubleClick {
		m.clickID = ""
		return m, m.jumpCmd()
	}
	m.clickID, m.clickAt = id, now
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
		body = helpScreen(m.width, m.height, m.readOnly, m.helpAt, theme)
	case m.queue:
		body = queueScreen(m.queueView(), m.width, m.height, theme)
	case m.detail && m.selected() != nil:
		body = detailScreenKeys(*m.selected(), m.width, m.height, m.scroll, m.pending == "g", m.readOnly, theme)
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
		body = render(m.snap, m.tab, status, viewOpts{Width: m.width, Height: m.height, ShowEnded: m.showEnded, Sort: m.sortBy, Sel: m.selID, Filter: m.filter, Typing: m.filtering, Size: m.wallSize, Auto: !m.wallSet, Demo: m.demo, NoColour: m.noColour, Error: m.statusIsError() && m.prompt == nil && m.ask == nil && !m.filtering, Theme: theme})
	}
	if m.colors != nil {
		body = m.colors.body()
	}
	v := tea.NewView(body)
	v.AltScreen = true
	if m.mouse && m.colors == nil {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
