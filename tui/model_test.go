package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func press(t *testing.T, m model, s string) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key(s))
	return next.(model), cmd
}

func TestKeysSwitchTabs(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	m, _ = press(t, m, "tab")
	if m.tab != tabTop {
		t.Errorf("tab should go to Top, got %d", m.tab)
	}
	m, _ = press(t, m, "tab")
	if m.tab != tabWatch {
		t.Errorf("tab should wrap to Watch, got %d", m.tab)
	}
	m, _ = press(t, m, "shift+tab")
	if m.tab != tabTop {
		t.Errorf("shift+tab should wrap back to Top, got %d", m.tab)
	}
	m, _ = press(t, m, "1")
	if m.tab != tabWatch {
		t.Errorf("1 is Watch")
	}
	m, _ = press(t, m, "2")
	if m.tab != tabTop {
		t.Errorf("2 is Top")
	}
	m, _ = press(t, m, "left")
	m, _ = press(t, m, "right")
	if m.tab != tabTop {
		t.Errorf("left then right should come back, got %d", m.tab)
	}
}

func TestKeyAShowsEnded(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	m, _ = press(t, m, "a")
	if !m.showEnded {
		t.Errorf("a should show ended sessions")
	}
	m, _ = press(t, m, "a")
	if m.showEnded {
		t.Errorf("a again should hide them")
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := press(t, newModel("x", time.Second, tabWatch), k)
		if cmd == nil {
			t.Fatalf("%s should quit", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s should produce a quit message", k)
		}
	}
}

func TestSnapshotAndErrorKeepLastGood(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	m.now = func() time.Time { return time.Unix(1000, 0) }
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	m = next.(model)
	if !m.haveSnap || m.errText() != "" {
		t.Fatalf("a good snapshot should be stored")
	}
	m.now = func() time.Time { return time.Unix(1012, 0) }
	next, _ = m.Update(snapMsg{err: errors.New("sitrep failed: boom")})
	m = next.(model)
	if !m.haveSnap || len(m.snap.Sessions) != 4 {
		t.Errorf("an error must keep the last good snapshot")
	}
	if got := m.errText(); !strings.Contains(got, "boom") || !strings.Contains(got, "showing data from 12s ago") {
		t.Errorf("error text: %q", got)
	}
	next, _ = m.Update(snapMsg{snap: loadFixture(t)})
	if next.(model).errText() != "" {
		t.Errorf("a later good snapshot clears the error")
	}
}

func TestErrorBeforeAnyData(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(snapMsg{err: errors.New("cannot run")})
	m = next.(model)
	if got := m.errText(); got != "error: cannot run" {
		t.Errorf("error text: %q", got)
	}
	if v := m.View(); !strings.Contains(v.Content, "error: cannot run") {
		t.Errorf("the screen should show the error, got %q", v.Content)
	}
}

func TestTickDoesNotOverlapFetches(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	m.fetching = true
	next, cmd := m.Update(tickMsg(time.Now()))
	if !next.(model).fetching || cmd == nil {
		t.Errorf("a tick during a fetch must keep fetching and re-arm the timer")
	}
	m.fetching = false
	next, _ = m.Update(tickMsg(time.Now()))
	if !next.(model).fetching {
		t.Errorf("a tick with no fetch running starts one")
	}
}

func TestWindowSizeAndBackground(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 77, Height: 22})
	m = next.(model)
	if m.width != 77 || m.height != 22 {
		t.Errorf("size not stored")
	}
}

func TestViewUsesAltScreenAndShowsTabs(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	v := next.(model).View()
	if !v.AltScreen {
		t.Errorf("the dashboard should use the alternate screen")
	}
	for _, want := range []string{"Watch", "Top", "claude-sitrep", "q quit"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("the view should contain %q", want)
		}
	}
}

func TestFirstFetchIsInFlightFromTheStart(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	if !m.fetching {
		t.Fatalf("Init sends the first fetch, so the model must start as fetching")
	}
	// a tick that arrives before the first answer must not start a second fetch
	next, _ := m.Update(tickMsg(time.Now()))
	if !next.(model).fetching {
		t.Errorf("still fetching")
	}
	next, _ = next.Update(snapMsg{snap: loadFixture(t)})
	if next.(model).fetching {
		t.Errorf("an answer ends the fetch")
	}
}

func TestNoTwoFetchesOverlap(t *testing.T) {
	m := newModel("x", time.Second, tabWatch) // the first fetch is in flight
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tickMsg(time.Now()))
		m = next.(model)
	}
	if m.started != 1 {
		t.Errorf("ticks during a fetch must not start another: %d started", m.started)
	}
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	m = next.(model)
	next, _ = m.Update(tickMsg(time.Now()))
	m = next.(model)
	next, _ = m.Update(tickMsg(time.Now()))
	m = next.(model)
	if m.started != 2 {
		t.Errorf("once the first answer is in, exactly one new fetch starts: %d started", m.started)
	}
}

func TestRefreshKeyDoesNotOverlap(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	m, cmd := press(t, m, "r")
	if m.started != 1 || cmd != nil {
		t.Errorf("r during a fetch does nothing: started %d", m.started)
	}
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	m = next.(model)
	m, cmd = press(t, m, "r")
	if m.started != 2 || cmd == nil || !m.fetching {
		t.Errorf("r after an answer starts one fetch: started %d", m.started)
	}
	m, cmd = press(t, m, "r")
	if m.started != 2 || cmd != nil {
		t.Errorf("a second r while it runs does nothing: started %d", m.started)
	}
}

func TestZeroSizedWindowMeansOneRow(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	m = next.(model)
	if m.width < 1 || m.height < 1 {
		t.Fatalf("a window of 0 must not mean no limit: %dx%d", m.width, m.height)
	}
	if lines := strings.Count(m.View().Content, "\n") + 1; lines > 1 {
		t.Errorf("a one-row window gets one row, got %d", lines)
	}
}
