package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// rowOf finds a screen row that belongs to the session, by the same map a click uses.
func rowOf(t *testing.T, m model, id string, nth int) int {
	t.Helper()
	var hits []string
	render(m.snap, m.tab, "", viewOpts{Width: m.width, Height: m.height, Sel: m.selID, Theme: plainTheme(), Hits: &hits})
	for y, h := range hits {
		if h == id {
			if nth == 0 {
				return y
			}
			nth--
		}
	}
	t.Fatalf("session %s is not on screen", id)
	return -1
}

func clickAt(m model, y int) (model, tea.Cmd) {
	next, cmd := m.Update(tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	return next.(model), cmd
}

func mouseModel(t *testing.T, tab int) (model, *time.Time) {
	m := loaded(t, tab)
	m.mouse = true
	clock := time.Unix(1000, 0)
	m.now = func() time.Time { return clock }
	return m, &clock
}

func TestClickSelectsAndDoubleClickJumps(t *testing.T) {
	for _, tab := range []int{tabWatch, tabTable} {
		m, clock := mouseModel(t, tab)
		id := m.snap.Sessions[1].ID
		y := rowOf(t, m, id, 0)
		m, cmd := clickAt(m, y)
		if m.selID != id || cmd != nil || m.acting {
			t.Fatalf("tab %d: one click only selects: sel %q cmd %v acting %v", tab, m.selID, cmd != nil, m.acting)
		}
		*clock = clock.Add(doubleClick / 2)
		m, cmd = clickAt(m, y)
		if cmd == nil || !m.acting {
			t.Errorf("tab %d: a second click at once jumps", tab)
		}
	}
}

func TestSlowSecondClickOrAnotherRowIsNotADoubleClick(t *testing.T) {
	m, clock := mouseModel(t, tabWatch)
	a, b := m.snap.Sessions[0].ID, m.snap.Sessions[1].ID
	m, _ = clickAt(m, rowOf(t, m, a, 0))
	*clock = clock.Add(doubleClick + time.Millisecond)
	m, cmd := clickAt(m, rowOf(t, m, a, 0))
	if cmd != nil {
		t.Error("a slow second click only selects")
	}
	*clock = clock.Add(time.Millisecond)
	m, cmd = clickAt(m, rowOf(t, m, b, 0))
	if cmd != nil || m.selID != b {
		t.Error("a click on another row selects it")
	}
	m, cmd = clickAt(m, rowOf(t, m, b, 0))
	if cmd == nil {
		t.Error("two quick clicks on the new row jump")
	}
	_ = m
}

func TestAnyLineOfABlockCounts(t *testing.T) {
	m, _ := mouseModel(t, tabWatch)
	id := m.snap.Sessions[0].ID
	m, _ = clickAt(m, rowOf(t, m, id, 0))
	_, cmd := clickAt(m, rowOf(t, m, id, 1)) // a later line of the same block
	if cmd == nil {
		t.Error("a click on a later line of the block is a click on the session")
	}
}

func TestDoubleClickNeedsTheSecondClickToBeNew(t *testing.T) {
	m, _ := mouseModel(t, tabWatch)
	id := m.snap.Sessions[0].ID
	y := rowOf(t, m, id, 0)
	m, _ = clickAt(m, y)
	m, cmd := clickAt(m, y)
	if cmd == nil {
		t.Fatal("double click jumps")
	}
	m.acting = false
	if _, cmd = clickAt(m, y); cmd != nil {
		t.Error("a third click starts a new pair")
	}
}

func TestMouseIsOffByDefault(t *testing.T) {
	m := loaded(t, tabWatch)
	if m.mouse || m.View().MouseMode != tea.MouseModeNone {
		t.Error("the mouse is off unless asked for")
	}
	y := rowOf(t, m, m.snap.Sessions[1].ID, 0)
	m2, cmd := clickAt(m, y)
	if m2.selID != m.selID || cmd != nil {
		t.Error("a click does nothing when the mouse is off")
	}
	m.mouse = true
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Error("the mouse is on when asked for")
	}
}

func TestClicksOutsideRowsAndOnScreensAreIgnored(t *testing.T) {
	m, _ := mouseModel(t, tabWatch)
	id := m.snap.Sessions[1].ID
	y := rowOf(t, m, id, 0)
	for _, y := range []int{0, -1, m.height + 5, m.height - 1} { // the tab bar, off screen, the status bar
		if m2, _ := clickAt(m, y); m2.selID != m.selID {
			t.Errorf("a click on row %d selected %q", y, m2.selID)
		}
	}
	right := tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseRight}
	if next, _ := m.Update(right); next.(model).selID == id {
		t.Error("only the left button selects")
	}
	for name, set := range map[string]func(*model){
		"help":      func(m *model) { m.help = true },
		"detail":    func(m *model) { m.detail = true },
		"queue":     func(m *model) { m.queue = true },
		"filtering": func(m *model) { m.filtering = true },
		"prompt":    func(m *model) { m.prompt = &promptState{} },
		"prune":     func(m *model) { m.prune = pruneConfirm },
		"ask":       func(m *model) { m.ask = &askState{} },
	} {
		o := m
		set(&o)
		if o2, cmd := clickAt(o, y); o2.selID != m.selID || cmd != nil {
			t.Errorf("%s: a click is ignored", name)
		}
	}
}

func TestHitsFollowTheScroll(t *testing.T) {
	m := loaded(t, tabWatch)
	m.height = 12
	ids := m.visible()
	last := ids[len(ids)-1]
	m.selID = last
	var hits []string
	out := render(m.snap, m.tab, "", viewOpts{Width: m.width, Height: m.height, Sel: m.selID, Theme: plainTheme(), Hits: &hits})
	if len(hits) != m.height {
		t.Fatalf("one entry per screen row: %d for %d", len(hits), m.height)
	}
	found := false
	for _, h := range hits {
		found = found || h == last
	}
	if !found {
		t.Errorf("the selected session is on screen and clickable:\n%s", out)
	}
	if hits[0] != "" || hits[len(hits)-1] != "" {
		t.Error("the tab bar and the status bar are no session")
	}
}

func TestClickOnATabNameOpensTheTab(t *testing.T) {
	for _, name := range ThemeNames { // the names are drawn with different padding in different themes
		m, _ := mouseModel(t, tabWatch)
		m.snap.Settings.Theme = name
		parts := tabParts(0, themeFor(m.isDark, m.snap.Settings))
		col := 0
		for i, p := range parts {
			w := ansi.StringWidth(p)
			for _, x := range []int{col, col + w/2, col + w - 1} { // either end and the middle of the name with its space
				next, cmd := m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
				if got := next.(model); got.tab != i || cmd != nil {
					t.Errorf("a click at column %d opens tab %d, got %d", x, i, got.tab)
				}
			}
			col += w
		}
		for _, x := range []int{col, col + 10, -1} {
			if next, _ := m.Update(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft}); next.(model).tab != tabWatch {
				t.Errorf("a click at column %d, beyond the names, opens nothing", x)
			}
		}
	}
}

func TestClickOnATabNeedsTheMouseAndNoScreenOpen(t *testing.T) {
	m, _ := mouseModel(t, tabWatch)
	click := tea.MouseClickMsg{X: 9, Y: 0, Button: tea.MouseLeft} // inside the second name
	if next, _ := m.Update(click); next.(model).tab != tabTable {
		t.Fatal("the click opens the Table tab")
	}
	off := m
	off.mouse = false
	if next, _ := off.Update(click); next.(model).tab != tabWatch {
		t.Error("with the mouse off a click does nothing")
	}
	right := tea.MouseClickMsg{X: 9, Y: 0, Button: tea.MouseRight}
	if next, _ := m.Update(right); next.(model).tab != tabWatch {
		t.Error("only the left button")
	}
	for name, set := range map[string]func(*model){
		"help":   func(m *model) { m.help = true },
		"detail": func(m *model) { m.detail = true },
		"queue":  func(m *model) { m.queue = true },
		"filter": func(m *model) { m.filtering = true },
	} {
		o := m
		set(&o)
		if next, _ := o.Update(click); next.(model).tab != tabWatch {
			t.Errorf("%s: the tab bar is not on screen, so the click does nothing", name)
		}
	}
	// before the first snapshot there are tabs to click and nothing in them
	empty := newModel("x", 0, tabWatch)
	empty.mouse = true
	if next, _ := empty.Update(click); next.(model).tab != tabTable {
		t.Error("a tab opens before any data has come")
	}
}

func TestClickOnATabKeepsTheSelectionAndStopsADoubleClick(t *testing.T) {
	m, _ := mouseModel(t, tabWatch)
	id := m.snap.Sessions[1].ID
	m, _ = clickAt(m, rowOf(t, m, id, 0)) // the first click of a pair
	next, _ := m.Update(tea.MouseClickMsg{X: 9, Y: 0, Button: tea.MouseLeft})
	m = next.(model)
	if m.selID != id || m.tab != tabTable {
		t.Errorf("the selection follows the session to the other tab: %q tab %d", m.selID, m.tab)
	}
	if m.clickID != "" {
		t.Error("a click on a tab ends the pair of clicks that a double click is")
	}
}
