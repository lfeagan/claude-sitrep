package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// wallSnap is n sessions in every state, with and without a queue, and with text of every length, in the order a snapshot has them.
func wallSnap(n int) Snapshot {
	s := Snapshot{Version: 1, Generated: 1_800_000_000, Registry: "ok", Inbox: Queue{Count: 2, Items: []string{"a", "b"}}}
	states := []struct {
		state, label string
		needs, snz   bool
	}{{"waiting", "waiting 3m", true, false}, {"working", "working 20s", false, false}, {"waiting", "waiting 40m", true, false}, {"waiting", "waiting 1h", false, true}}
	for i := 0; i < n; i++ {
		st := states[i%len(states)]
		x := Session{
			ID: fmt.Sprintf("s%03d-%03d", i, i), Short: fmt.Sprintf("s%03d", i), Dir: fmt.Sprintf("project-%d", i), Running: true,
			State: st.state, StateLabel: st.label, NeedsInput: st.needs, Snoozed: st.snz, Highlight: st.state == "working",
			Objective: fmt.Sprintf("Objective %d: a goal that is rather long, to see where it is cut %s", i, strings.Repeat("word ", i%7)),
			Progress:  fmt.Sprintf("progress %d", i), Now: fmt.Sprintf("now %d 日本語のテキスト", i),
		}
		if i%3 != 0 { // a queue for two sessions in three
			x.Queue = Queue{Count: i%5 + 1, Items: []string{fmt.Sprintf("the next item of %d, which is long enough to be cut", i)}}
		}
		if i%4 == 1 {
			x.Context = &Context{UsedPercent: 62, Window: 1_000_000}
		}
		if i%6 == 5 {
			x.Now = ""
		}
		s.Sessions = append(s.Sessions, x)
	}
	return s
}

func TestEveryTileOfASizeHasTheSameRows(t *testing.T) {
	s := wallSnap(24)
	for size := 0; size < wallSizes; size++ {
		for _, cw := range []int{1, 5, 12, 27, 60} {
			for _, x := range s.Sessions {
				rows := tileRows(x, size, cw)
				if len(rows) != wallRows[size] {
					t.Fatalf("size %s: %d rows for %s, want %d", wallSizeNames[size], len(rows), x.ID, wallRows[size])
				}
				for _, r := range rows {
					if ansi.StringWidth(r) > cw {
						t.Fatalf("size %s width %d: %q is %d wide", wallSizeNames[size], cw, r, ansi.StringWidth(r))
					}
				}
			}
		}
	}
	// a session with no queue and one with a long queue are as high as each other
	none, some := tileRows(Session{ID: "a", Running: true}, wallStandard, 30), tileRows(Session{ID: "b", Running: true, Queue: Queue{Count: 9, Items: []string{"x"}}}, wallStandard, 30)
	if len(none) != len(some) || none[2] != "no queue" || !strings.HasPrefix(some[2], "9 queued: x") {
		t.Errorf("queue row: %q and %q", none, some)
	}
}

func TestWallLayoutHoldsAtEverySize(t *testing.T) {
	s := wallSnap(25)
	ids := visibleIDs(s, tabWall, false, 0, "")
	for size := 0; size < wallSizes; size++ {
		for w := 1; w <= 150; w += 7 {
			for h := 3; h <= 40; h += 3 {
				for _, sel := range []string{ids[0], ids[len(ids)/2], ids[len(ids)-1]} {
					var cells []wallCell
					out := render(s, tabWall, "", viewOpts{Width: w, Height: h, Size: size, Sel: sel, Theme: plainTheme(), Cells: &cells})
					lines := strings.Split(out, "\n")
					if len(lines) > h {
						t.Fatalf("size %d %dx%d: %d lines", size, w, h, len(lines))
					}
					for _, l := range lines {
						if ansi.StringWidth(l) > w {
							t.Fatalf("size %d %dx%d: %q is %d wide", size, w, h, l, ansi.StringWidth(l))
						}
					}
					room := h - 2 - 1 - 1 - wallRows[size] // tab bar, rule, summary, status bar, one tile
					if room < 0 {
						continue
					}
					seen := map[string]bool{}
					for _, c := range cells {
						seen[c.ID] = true
						if c.Y1-c.Y0+1 != wallRows[size] || c.X1 < c.X0 {
							t.Fatalf("size %d %dx%d: cell %+v is not one tile", size, w, h, c)
						}
						if c.Y1 >= len(lines) {
							t.Fatalf("size %d %dx%d: cell %+v is below the screen", size, w, h, c)
						}
					}
					if !seen[sel] {
						t.Fatalf("size %d %dx%d: the selected tile %s is not on screen:\n%s", size, w, h, sel, out)
					}
				}
			}
		}
	}
}

func TestWallCountsTheTilesAboveAndBelow(t *testing.T) {
	s := wallSnap(25)
	ids := visibleIDs(s, tabWall, false, 0, "")
	var cells []wallCell
	o := viewOpts{Width: 100, Height: 14, Sel: ids[24], Theme: plainTheme(), Cells: &cells} // 3 columns, standard
	out := render(s, tabWall, "", o)
	cols := wallCols(100)
	if cols != 3 {
		t.Fatalf("columns: %d", cols)
	}
	shown := map[string]bool{}
	for _, c := range cells {
		shown[c.ID] = true
	}
	first := -1
	for i, id := range ids {
		if shown[id] {
			first = i
			break
		}
	}
	if !strings.Contains(out, fmt.Sprintf("+%d above", first)) || strings.Contains(out, "more") {
		t.Errorf("the last tile is on screen, with %d tiles above and none below:\n%s", first, out)
	}
	o.Sel = ids[0]
	out = render(s, tabWall, "", o)
	want := len(ids) - len(cells)
	if !strings.Contains(out, fmt.Sprintf("+%d more", want)) || strings.Contains(out, "above") {
		t.Errorf("the first tile is on screen, with %d below:\n%s", want, out)
	}
}

func TestWallOrdersTheSessionsThatNeedYouFirst(t *testing.T) {
	s := wallSnap(12)
	ids := visibleIDs(s, tabWall, false, 0, "")
	need := true
	for _, id := range ids {
		x := byIDof(s, id)
		if !x.NeedsInput {
			need = false
		} else if !need {
			t.Fatalf("a session that needs you comes after one that does not: %v", ids)
		}
	}
}

func byIDof(s Snapshot, id string) Session {
	for _, x := range s.Sessions {
		if x.ID == id {
			return x
		}
	}
	return Session{}
}

func wallModel(t *testing.T, n int) model {
	t.Helper()
	m := newModel("x", 0, tabWall)
	m.width, m.height = 100, 40 // three columns
	next, _ := m.Update(snapMsg{snap: wallSnap(n)})
	return next.(model)
}

func TestWallKeysMoveAcrossAndDownTheGrid(t *testing.T) {
	m := wallModel(t, 7) // rows of 3, 3 and 1
	ids := m.visible()
	idx := func() int {
		for i, id := range ids {
			if id == m.selID {
				return i
			}
		}
		return -1
	}
	if idx() != 0 {
		t.Fatalf("the first tile is selected: %d", idx())
	}
	for _, step := range []struct {
		key  string
		want int
	}{{"l", 1}, {"right", 2}, {"j", 5}, {"down", 6}, {"h", 5}, {"left", 4}, {"k", 1}, {"up", 0}} {
		m, _ = press(t, m, step.key)
		if idx() != step.want {
			t.Fatalf("%s: tile %d, want %d", step.key, idx(), step.want)
		}
	}
	m, _ = press(t, m, "G")
	m, _ = press(t, m, "j")
	if idx() != 6 {
		t.Errorf("j at the last row stays: %d", idx())
	}
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "g")
	if idx() != 0 {
		t.Errorf("gg: %d", idx())
	}
	// a short last row: down from the middle of the row above goes to the last tile there is
	m, _ = press(t, m, "j")
	m, _ = press(t, m, "l")
	m, _ = press(t, m, "j")
	if idx() != 6 {
		t.Errorf("down into a short row: %d", idx())
	}
	// tab, not h and l, changes tab
	if m2, _ := press(t, m, "tab"); m2.tab != tabWatch {
		t.Errorf("tab leaves the Wall: %d", m2.tab)
	}
	if m2, _ := press(t, m, "h"); m2.tab != tabWall {
		t.Errorf("h stays on the Wall: %d", m2.tab)
	}
}

func TestSCyclesTheTileSizeOnTheWallOnly(t *testing.T) {
	m := wallModel(t, 7)
	for _, want := range []int{wallRoomy, wallCompact, wallStandard} {
		m, _ = press(t, m, "s")
		if m.wallSize != want {
			t.Fatalf("s: size %d, want %d", m.wallSize, want)
		}
	}
	if v := m.View().Content; !strings.Contains(v, "size: standard") || !strings.Contains(v, "s size") {
		t.Errorf("the tab says its size and the key:\n%s", v)
	}
	top := loaded(t, tabTop)
	if top2, _ := press(t, top, "s"); top2.wallSize != 0 || top2.sortBy == 0 {
		t.Error("s on Top sorts and leaves the Wall alone")
	}
}

func TestWallClickSelectsATileAndDoubleClickJumps(t *testing.T) {
	m := wallModel(t, 7)
	m.mouse = true
	clock := time.Unix(1000, 0)
	m.now = func() time.Time { return clock }
	var cells []wallCell
	render(m.snap, m.tab, "", viewOpts{Width: m.width, Height: m.height, Size: m.wallSize, Sel: m.selID, Theme: plainTheme(), Cells: &cells})
	target := cells[4]
	click := tea.MouseClickMsg{X: target.X0 + 3, Y: target.Y0 + 1, Button: tea.MouseLeft}
	next, cmd := m.Update(click)
	m = next.(model)
	if m.selID != target.ID || cmd != nil {
		t.Fatalf("a click selects the tile: %q want %q", m.selID, target.ID)
	}
	next, cmd = m.Update(click)
	if cmd == nil || !next.(model).acting {
		t.Error("a second click on it jumps")
	}
	// the gap between tiles, and the summary line, select nothing
	m.selID = cells[0].ID
	gap := tea.MouseClickMsg{X: cells[0].X1 + 1, Y: cells[0].Y0, Button: tea.MouseLeft}
	if next, _ := m.Update(gap); next.(model).selID != cells[0].ID {
		t.Error("the gap between two tiles is no tile")
	}
}

func TestWallGolden(t *testing.T) {
	s := wallSnap(11)
	ids := visibleIDs(s, tabWall, false, 0, "")
	for size := 0; size < wallSizes; size++ {
		o := viewOpts{Width: 100, Height: 26, Size: size, Sel: ids[4], Theme: plainTheme()}
		golden(t, "wall_"+wallSizeNames[size]+"_100x26", render(s, tabWall, "", o))
	}
	golden(t, "wall_narrow_standard_40x24", render(s, tabWall, "", viewOpts{Width: 40, Height: 24, Sel: ids[1], Theme: plainTheme()}))
}

func TestWallTabByNameAndHelp(t *testing.T) {
	if n, err := tabByName("wall"); err != nil || n != tabWall {
		t.Errorf("--tab wall: %d %v", n, err)
	}
	if _, err := tabByName("bogus"); err == nil || !strings.Contains(err.Error(), "wall") {
		t.Errorf("the error names the tabs: %v", err)
	}
	h := helpScreen(100, 60, false, plainTheme())
	for _, want := range []string{"Wall", "h, l, j, k"} {
		if !strings.Contains(h, want) {
			t.Errorf("the help should say %q", want)
		}
	}
}

func TestArrowsLeaveTheWallAtItsEnds(t *testing.T) {
	m := wallModel(t, 5)
	ids := m.visible()
	// walk in with the right arrow from Watch, and out the same way
	w := loaded(t, tabWatch)
	w, _ = press(t, w, "right")
	w, _ = press(t, w, "right")
	if w.tab != tabWall {
		t.Fatalf("two right arrows reach the Wall: %d", w.tab)
	}
	for _, key := range []string{"left", "h"} {
		if x, _ := press(t, w, key); x.tab != tabTop {
			t.Errorf("%s on the first tile goes back to the tab it came from: %d", key, x.tab)
		}
	}
	// in the middle they move between tiles, and at the last tile they go on to the next tab
	m, _ = press(t, m, "right")
	if m.tab != tabWall || m.selID != ids[1] {
		t.Fatalf("right moves a tile: tab %d sel %q", m.tab, m.selID)
	}
	m, _ = press(t, m, "G")
	if x, _ := press(t, m, "right"); x.tab != tabWatch {
		t.Errorf("right on the last tile goes to the next tab, which wraps to Watch: %d", x.tab)
	}
	// and the same from the other side: left from Watch wraps to the Wall, on its last tile, and right walks out again
	back, _ := press(t, loaded(t, tabWatch), "left")
	if back.tab != tabWall || back.selID != back.visible()[len(back.visible())-1] {
		t.Fatalf("left from Watch lands on the last tile of the Wall: tab %d sel %q", back.tab, back.selID)
	}
	if x, _ := press(t, back, "right"); x.tab != tabWatch {
		t.Errorf("right walks out again: %d", x.tab)
	}
	if x, _ := press(t, m, "left"); x.tab != tabWall || x.selID != ids[3] {
		t.Errorf("left on the last tile moves a tile: tab %d sel %q", x.tab, x.selID)
	}
	// an empty Wall has no tile to move to, so the arrows change tab
	e := newModel("x", 0, tabWall)
	if x, _ := press(t, e, "right"); x.tab != tabWatch {
		t.Errorf("an empty Wall: %d", x.tab)
	}
}
