package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"time"
)

func TestDetailLinesShowEverything(t *testing.T) {
	s := loadFixture(t)
	out := strings.Join(detailLines(s.Sessions[0], 80), "\n")
	for _, want := range []string{s.Sessions[0].ID, "/Users/x/claude-sitrep", "working 1m", "pid 4242", "Quiet", "Pinned", "Objective", "Progress", "Now", "7 items, the first 3 shown", "1. Decide whether", "3. Write the README"} {
		if !strings.Contains(out, want) {
			t.Errorf("the detail should contain %q:\n%s", want, out)
		}
	}
	if w := strings.Join(detailLines(s.Sessions[2], 80), "\n"); !strings.Contains(w, "● waiting 3m  (waiting for you)") || !strings.Contains(w, "not known here") {
		t.Errorf("the waiting session says so, and has no pid:\n%s", w)
	}
}

func TestDetailCutsNothing(t *testing.T) {
	long := strings.Repeat("alpha beta gamma delta ", 60)
	x := Session{ID: "x", Short: "x", Objective: long, Progress: long, Now: long, Queue: Queue{Count: 1, Items: []string{long}}}
	got := strings.Join(strings.Fields(strings.Join(detailLines(x, 40), " ")), " ")
	if n := strings.Count(got, "alpha beta gamma delta"); n < 4*60 {
		t.Errorf("a long text must be wrapped whole, not cut: %d of %d repeats survive", n, 4*60)
	}
	if strings.Contains(strings.Join(detailLines(x, 40), "\n"), "…") {
		t.Errorf("the detail pane never uses an ellipsis")
	}
}

func TestDetailLongQueue(t *testing.T) {
	var items []string
	for i := 1; i <= maxItems; i++ {
		items = append(items, fmt.Sprintf("item %d", i))
	}
	x := Session{ID: "x", Short: "x", Queue: Queue{Count: 150, Items: items}}
	out := strings.Join(detailLines(x, 80), "\n")
	if !strings.Contains(out, "150 items, the first 100 shown") || !strings.Contains(out, "100. item 100") {
		t.Errorf("a capped queue says how many there are:\n%s", out[:400])
	}
}

func TestDetailFitsEverySize(t *testing.T) {
	s := loadFixture(t)
	for _, x := range s.Sessions {
		for w := 1; w <= 100; w += 3 {
			for h := 1; h <= 40; h += 3 {
				for _, scroll := range []int{0, 3, 1000} {
					lines := strings.Split(detailScreen(x, w, h, scroll, plainTheme()), "\n")
					if len(lines) > h {
						t.Errorf("%s %dx%d scroll %d: %d lines", x.Short, w, h, scroll, len(lines))
					}
					for _, l := range lines {
						if ansi.StringWidth(l) > w {
							t.Errorf("%s %dx%d: a line is wider than the screen: %q", x.Short, w, h, l)
						}
					}
				}
			}
		}
	}
}

func TestDetailScrollIsClamped(t *testing.T) {
	x := loadFixture(t).Sessions[0]
	low := detailScreen(x, 60, 12, -5, plainTheme())
	top := detailScreen(x, 60, 12, 0, plainTheme())
	if low != top {
		t.Errorf("a negative scroll is the top")
	}
	far := detailScreen(x, 60, 12, 10000, plainTheme())
	end := detailScreen(x, 60, 12, detailMax(x, 60, 12), plainTheme())
	if far != end {
		t.Errorf("a scroll past the end is the end")
	}
	if !strings.Contains(top, "lines 1-") {
		t.Errorf("a detail taller than the screen says which lines are shown:\n%s", top)
	}
}

func TestDetailGolden(t *testing.T) {
	s := loadFixture(t)
	golden(t, "detail_80x24", detailScreen(s.Sessions[0], 80, 24, 0, plainTheme()))
	golden(t, "detail_cjk_50x20", detailScreen(s.Sessions[1], 50, 20, 0, plainTheme()))
	golden(t, "detail_hostile_60x20", detailScreen(s.Sessions[2], 60, 20, 0, plainTheme()))
}

func TestDetailFrameOnAVeryShortScreen(t *testing.T) {
	x := loadFixture(t).Sessions[0]
	for h := 1; h <= 4; h++ {
		if got := strings.Count(detailScreen(x, 60, h, 0, plainTheme()), "\n") + 1; got > h {
			t.Errorf("height %d: %d lines", h, got)
		}
	}
}

func TestStaleScrollIsClamped(t *testing.T) {
	snap := loadFixture(t)
	for i := 0; i < 40; i++ {
		snap.Sessions[0].Queue.Items = append(snap.Sessions[0].Queue.Items, fmt.Sprintf("queued item %d with some words", i))
	}
	snap.Sessions[0].Queue.Count = len(snap.Sessions[0].Queue.Items)
	m := newModel("x", time.Second, tabCards)
	next, _ := m.Update(snapMsg{snap: snap})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	m = next.(model)
	m, _ = press(t, m, "v")
	m, _ = press(t, m, "end")
	if m.scroll == 0 {
		t.Fatalf("the long queue scrolls")
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 200}) // the window grows: everything fits
	m = next.(model)
	if m.scroll != 0 {
		t.Errorf("a resize clamps the scroll: %d", m.scroll)
	}
	m.scroll = 500
	short := loadFixture(t)
	next, _ = m.Update(snapMsg{snap: short}) // the session shrinks
	if got := next.(model).scroll; got > detailMax(short.Sessions[0], 80, 200) {
		t.Errorf("a refresh clamps the scroll: %d", got)
	}
	m.scroll = 500
	m, _ = press(t, m, "pgup")
	if m.scroll > detailMax(*m.selected(), m.width, m.height) {
		t.Errorf("a key clamps a stale scroll first: %d", m.scroll)
	}
}
