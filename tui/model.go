package main

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

const fetchTimeout = 5 * time.Second

type tickMsg time.Time

type snapMsg struct {
	snap Snapshot
	err  error
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
	sortBy    int
	help      bool
	isDark    bool
	now       func() time.Time
}

func newModel(bin string, interval time.Duration, tab int) model {
	// fetching starts true because Init sends the first fetch; Init has a value receiver, so it cannot set it
	return model{bin: bin, interval: interval, tab: tab, width: 100, height: 30, isDark: true, now: time.Now, fetching: true, started: 1}
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

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.tickCmd(), tea.RequestBackgroundColor)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1) // 0 would mean "no limit"
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
	case snapMsg:
		m.fetching = false
		if msg.err != nil {
			m.err = msg.err // the last good snapshot stays on screen
			return m, nil
		}
		m.snap, m.haveSnap, m.err, m.lastOK = msg.snap, true, nil, m.now()
	case tickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		if !m.fetching { // never overlap two calls; a slow one just skips ticks
			m.fetching, m.started = true, m.started+1
			cmds = append(cmds, m.fetchCmd())
		}
		return m, tea.Batch(cmds...)
	case tea.KeyPressMsg:
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
		case "a":
			m.showEnded = !m.showEnded
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
	case m.help:
		body = helpScreen(m.width, m.height, theme)
	case m.haveSnap || m.err != nil:
		body = render(m.snap, m.tab, m.errText(), viewOpts{Width: m.width, Height: m.height, ShowEnded: m.showEnded, Sort: m.sortBy, Theme: theme})
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}
