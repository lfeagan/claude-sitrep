package main

import (
	"errors"
	"fmt"
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
	case "?":
		return tea.KeyPressMsg{Code: '?', Text: "?"}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
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

func TestSKeyCyclesTheTopSort(t *testing.T) {
	m := newModel("x", time.Second, tabTop)
	for want := 1; want <= sortCount; want++ {
		m, _ = press(t, m, "s")
		if m.sortBy != want%sortCount {
			t.Fatalf("after %d presses the sort is %d", want, m.sortBy)
		}
	}
	w := newModel("x", time.Second, tabWatch)
	w, _ = press(t, w, "s")
	if w.sortBy != sortActivity {
		t.Errorf("s does nothing on the Watch tab")
	}
}

func TestQuestionMarkOpensAndAnyKeyCloses(t *testing.T) {
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	m = next.(model)
	m, _ = press(t, m, "?")
	if !m.help {
		t.Fatalf("? opens the key list")
	}
	if v := m.View(); !strings.Contains(v.Content, "Keys") || strings.Contains(v.Content, "claude-sitrep") || !v.AltScreen {
		t.Errorf("the key list replaces the screen, in the alternate screen: %q", v.Content)
	}
	m, cmd := press(t, m, "x")
	if m.help || cmd != nil {
		t.Errorf("any key closes the list and does nothing else")
	}
	m, _ = press(t, m, "?")
	m, _ = press(t, m, "tab")
	if m.help || m.tab != tabWatch {
		t.Errorf("a key that closes the list must not also act: tab %d", m.tab)
	}
	m, _ = press(t, m, "?")
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Errorf("ctrl+c quits even from the key list")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c should produce a quit")
	}
	m, _ = press(t, m, "q")
	if m.help {
		t.Errorf("q closes the key list")
	}
}

func loaded(t *testing.T, tab int) model {
	t.Helper()
	m := newModel("x", time.Second, tab)
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	return next.(model)
}

func TestSelectionStartsOnTheFirstSession(t *testing.T) {
	m := loaded(t, tabWatch)
	if m.selID != m.snap.Sessions[0].ID {
		t.Errorf("the first session is selected: %q", m.selID)
	}
}

func TestSelectionKeys(t *testing.T) {
	m := loaded(t, tabWatch)
	first, second, third := m.snap.Sessions[0].ID, m.snap.Sessions[1].ID, m.snap.Sessions[2].ID
	for _, k := range []string{"down", "j"} {
		m = loaded(t, tabWatch)
		m, _ = press(t, m, k)
		if m.selID != second {
			t.Errorf("%s moves down: %q", k, m.selID)
		}
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	if m.selID != third {
		t.Errorf("selection stops at the last session, not wrapping: %q", m.selID)
	}
	for _, k := range []string{"up", "k"} {
		m2, _ := press(t, m, k)
		if m2.selID != second {
			t.Errorf("%s moves up: %q", k, m2.selID)
		}
	}
	m, _ = press(t, m, "home")
	if m.selID != first {
		t.Errorf("home selects the first")
	}
	m, _ = press(t, m, "up")
	if m.selID != first {
		t.Errorf("selection stops at the first")
	}
	m, _ = press(t, m, "end")
	if m.selID != third {
		t.Errorf("end selects the last visible: %q", m.selID)
	}
}

func TestSelectionFollowsTheSessionNotTheRow(t *testing.T) {
	m := loaded(t, tabTop)
	m, _ = press(t, m, "down")
	want := m.selID
	m, _ = press(t, m, "s") // a new sort changes the rows, not the selection
	if m.selID != want {
		t.Errorf("a re-sort must not move the selection: %q != %q", m.selID, want)
	}
	// a refresh in which the sessions come in another order
	snap := loadFixture(t)
	snap.Sessions[0], snap.Sessions[2] = snap.Sessions[2], snap.Sessions[0]
	next, _ := m.Update(snapMsg{snap: snap})
	if next.(model).selID != want {
		t.Errorf("a refresh must not move the selection")
	}
}

func TestSelectionFallsBackWhenTheSessionLeaves(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "down")
	snap := loadFixture(t)
	snap.Sessions = append(snap.Sessions[:1], snap.Sessions[2:]...) // the selected one is gone
	next, _ := m.Update(snapMsg{snap: snap})
	if got := next.(model).selID; got != snap.Sessions[0].ID {
		t.Errorf("a vanished session selects the first: %q", got)
	}
	empty, _ := m.Update(snapMsg{snap: Snapshot{Version: 1}})
	if empty.(model).selID != "" {
		t.Errorf("nothing to select")
	}
}

func TestEndedToggleKeepsOrMovesTheSelection(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "a")
	m, _ = press(t, m, "end") // the ended session, now shown
	if m.selID != m.snap.Sessions[3].ID {
		t.Fatalf("the ended session is selectable once shown: %q", m.selID)
	}
	m, _ = press(t, m, "a") // hidden again: the selection cannot stay on it
	if m.selID != m.snap.Sessions[0].ID {
		t.Errorf("hiding the selected session selects the first: %q", m.selID)
	}
}

func TestSelectionKeysIgnoredInTheKeyList(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "?")
	m, _ = press(t, m, "j")
	if m.help || m.selID != m.snap.Sessions[0].ID {
		t.Errorf("a key that closes the list must not also move the selection")
	}
}

func TestDetailOpensAndCloses(t *testing.T) {
	for _, k := range []string{"d", "space"} {
		m, _ := press(t, loaded(t, tabWatch), k)
		if !m.detail {
			t.Errorf("%s opens the detail pane", k)
		}
		for _, c := range []string{"esc", "d", "backspace"} {
			closed, _ := press(t, m, c)
			if closed.detail {
				t.Errorf("%s closes the detail pane", c)
			}
		}
	}
	m, _ := press(t, loaded(t, tabWatch), "enter")
	if m.detail {
		t.Errorf("enter is for the jump, not the detail pane")
	}
	if empty := newModel("x", time.Second, tabWatch); true {
		e, _ := press(t, empty, "d")
		if e.detail {
			t.Errorf("with nothing selected there is nothing to open")
		}
	}
}

func TestDetailUpDownSwitchSessions(t *testing.T) {
	m, _ := press(t, loaded(t, tabWatch), "d")
	first := m.selID
	m.scroll = 5
	m, _ = press(t, m, "down")
	if m.selID == first || !m.detail || m.scroll != 0 {
		t.Errorf("down shows the next session from the top: sel %q scroll %d", m.selID, m.scroll)
	}
	m, _ = press(t, m, "k")
	if m.selID != first {
		t.Errorf("k goes back")
	}
}

func TestDetailScrolls(t *testing.T) {
	snap := loadFixture(t)
	for i := 0; i < 40; i++ {
		snap.Sessions[0].Queue.Items = append(snap.Sessions[0].Queue.Items, fmt.Sprintf("queued item number %d with some words", i))
	}
	snap.Sessions[0].Queue.Count = len(snap.Sessions[0].Queue.Items)
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(snapMsg{snap: snap})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = next.(model)
	m, _ = press(t, m, "d")
	maxScroll := detailMax(*m.selected(), 80, 20)
	if maxScroll < 20 {
		t.Fatalf("the long queue should need scrolling: %d", maxScroll)
	}
	m, _ = press(t, m, "pgdown")
	if m.scroll <= 0 || m.scroll > maxScroll {
		t.Errorf("pgdown scrolls within range: %d", m.scroll)
	}
	m, _ = press(t, m, "space")
	m, _ = press(t, m, "end")
	if m.scroll != maxScroll {
		t.Errorf("end scrolls to the bottom: %d of %d", m.scroll, maxScroll)
	}
	m, _ = press(t, m, "pgdown")
	if m.scroll != maxScroll {
		t.Errorf("scrolling stops at the bottom: %d", m.scroll)
	}
	m, _ = press(t, m, "pgup")
	if m.scroll >= maxScroll {
		t.Errorf("pgup scrolls back")
	}
	m, _ = press(t, m, "home")
	if m.scroll != 0 {
		t.Errorf("home scrolls to the top")
	}
	m, _ = press(t, m, "pgup")
	if m.scroll != 0 {
		t.Errorf("scrolling stops at the top")
	}
}

func TestDetailQuitAndRefresh(t *testing.T) {
	m, _ := press(t, loaded(t, tabWatch), "d")
	for _, k := range []string{"q", "ctrl+c"} {
		if _, cmd := press(t, m, k); cmd == nil {
			t.Errorf("%s quits from the detail pane", k)
		}
	}
	next, _ := m.Update(snapMsg{snap: loadFixture(t)}) // a refresh must not close it
	if !next.(model).detail {
		t.Errorf("a refresh keeps the detail pane open")
	}
	snap := loadFixture(t)
	snap.Sessions = snap.Sessions[1:] // the session being shown is gone
	next, _ = m.Update(snapMsg{snap: snap})
	if next.(model).detail {
		t.Errorf("the detail pane closes when its session is gone")
	}
}

func TestDetailView(t *testing.T) {
	m, _ := press(t, loaded(t, tabWatch), "d")
	v := m.View()
	if !v.AltScreen {
		t.Errorf("alternate screen")
	}
	for _, want := range []string{"Session", m.selID, "pid 4242", "Objective", "writing the golden tests"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("the detail view should contain %q", want)
		}
	}
	if strings.Contains(v.Content, "[Watch]") {
		t.Errorf("the detail pane replaces the tabs")
	}
}

func TestListsCapQueueItemsBySetting(t *testing.T) {
	snap := loadFixture(t)
	snap.Settings.QueueItems = 2
	snap.Sessions[0].Queue = Queue{Count: 9, Items: []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9"}}
	out := render(snap, tabWatch, "", opts(80, 60))
	if !strings.Contains(out, "2. a2") || strings.Contains(out, "3. a3") || !strings.Contains(out, "+7 more") {
		t.Errorf("the Watch view lists the setting's number of items and counts the rest:\n%s", out)
	}
}

func typed(t *testing.T, m model, text string) model {
	t.Helper()
	for _, r := range text {
		k := string(r)
		if r == ' ' {
			k = "space"
		}
		m, _ = press(t, m, k)
	}
	return m
}

func TestFilterMatchesCaseInsensitivelyOverEveryField(t *testing.T) {
	s := loadFixture(t)
	for _, c := range []struct{ text, want string }{
		{"TIMESCALEDB", "b1fb"},  // directory
		{"15f1", "15f1"},         // id
		{"waiting", "b1fb"},      // state
		{"go front-end", "15f1"}, // Objective
		{"worktrees", "2cd8"},    // Progress
		{"golden tests", "15f1"}, // Now
		{"  Golden  ", "15f1"},   // trimmed
	} {
		got := visibleIDs(s, tabWatch, false, sortActivity, c.text)
		if len(got) != 1 || !strings.HasPrefix(got[0], c.want) {
			t.Errorf("filter %q: %v, want only %s", c.text, got, c.want)
		}
	}
	if got := visibleIDs(s, tabWatch, false, sortActivity, "nothing like this"); len(got) != 0 {
		t.Errorf("no match lists nothing: %v", got)
	}
}

func TestFilterKeysAreTextNotCommands(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "/")
	if !m.filtering {
		t.Fatal("/ starts the filter")
	}
	m = typed(t, m, "pqaszy")
	if m.filter != "pqaszy" || !m.filtering || m.showEnded || m.prune != pruneNone || m.tab != tabWatch {
		t.Errorf("command letters are text while filtering: %+v", m)
	}
	if _, cmd := press(t, m, "q"); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Error("q is text, it does not quit")
		}
	}
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c still quits")
	}
}

func TestFilterEnterKeepsAndEscClears(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "/")
	m = typed(t, m, "timescale")
	if got := m.visible(); len(got) != 1 || m.selID != got[0] {
		t.Errorf("typing narrows the list and moves the selection: %v %q", got, m.selID)
	}
	m, _ = press(t, m, "enter")
	if m.filtering || m.filter != "timescale" || len(m.visible()) != 1 {
		t.Error("enter keeps the filter and returns to the normal keys")
	}
	if s := m.filterStatus(); !strings.Contains(s, "/timescale") || !strings.Contains(s, "(1 of 3)") {
		t.Errorf("status shows the filter and the counts: %q", s)
	}
	m, _ = press(t, m, "esc")
	if m.filter != "" || len(m.visible()) != 3 {
		t.Error("esc clears a kept filter")
	}
	m, _ = press(t, m, "/")
	m = typed(t, m, "zzz")
	m, _ = press(t, m, "esc")
	if m.filtering || m.filter != "" {
		t.Error("esc while typing clears and leaves")
	}
}

func TestFilterBackspaceAndSelectionKeys(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "/")
	m = typed(t, m, "ab")
	m, _ = press(t, m, "backspace")
	if m.filter != "a" {
		t.Errorf("backspace removes a character: %q", m.filter)
	}
	m, _ = press(t, m, "backspace")
	m, _ = press(t, m, "backspace")
	if m.filter != "" {
		t.Errorf("backspace on an empty filter does nothing: %q", m.filter)
	}
	first, second := m.snap.Sessions[0].ID, m.snap.Sessions[1].ID
	m, _ = press(t, m, "down")
	if m.selID != second || !m.filtering {
		t.Errorf("down moves the selection while typing: %q", m.selID)
	}
	m, _ = press(t, m, "up")
	if m.selID != first {
		t.Errorf("up moves it back: %q", m.selID)
	}
	m = typed(t, m, "go front")
	if m.filter != "go front" {
		t.Errorf("space is text: %q", m.filter)
	}
}

func TestFilterPasteIsCleanedAndCapped(t *testing.T) {
	m := loaded(t, tabWatch)
	next, _ := m.Update(tea.PasteMsg{Content: "ignored"})
	if next.(model).filter != "" {
		t.Error("a paste outside the filter goes nowhere")
	}
	m, _ = press(t, m, "/")
	next, _ = m.Update(tea.PasteMsg{Content: "ti\x1b[31mmes‮cale\nx" + strings.Repeat("y", 100)})
	m = next.(model)
	if strings.ContainsAny(m.filter, "\x1b‮\n") {
		t.Errorf("control and format characters are removed: %q", m.filter)
	}
	if n := len([]rune(m.filter)); n != maxFilter {
		t.Errorf("a paste is capped at %d, got %d", maxFilter, n)
	}
}

func TestFilterSlashIsIgnoredInDetailAndHelp(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "d")
	m, _ = press(t, m, "/")
	if m.filtering {
		t.Error("/ does nothing in the detail pane")
	}
}

func TestFilterViewSaysWhenNothingMatches(t *testing.T) {
	for _, tab := range []int{tabWatch, tabTop} {
		m := loaded(t, tab)
		m.width, m.height = 80, 20
		m, _ = press(t, m, "/")
		m = typed(t, m, "zzzz")
		out := fmt.Sprint(m.View().Content)
		if !strings.Contains(out, "no session matches the filter") || !strings.Contains(out, "/zzzz▏  (0 of 3)") {
			t.Errorf("tab %d: no match is said, and the status bar shows the filter:\n%s", tab, out)
		}
	}
}
