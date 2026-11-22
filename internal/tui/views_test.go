package tui

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// golden compares got with testdata/<name>.golden, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file %s (run go test -update): %v", path, err)
	}
	if strings.TrimRight(string(want), "\n") != got {
		t.Errorf("%s differs from the golden file; run go test -update and review the diff\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func opts(w, h int) viewOpts { return viewOpts{Width: w, Height: h, Theme: plainTheme()} }

func TestCardsGolden(t *testing.T) {
	s := loadFixture(t)
	golden(t, "cards_80x40", render(s, tabCards, "", opts(80, 40)))
	golden(t, "cards_48x40", render(s, tabCards, "", opts(48, 40)))
	golden(t, "cards_80x14_cut", render(s, tabCards, "", opts(80, 14)))
	o := opts(80, 40)
	o.ShowEnded = true
	golden(t, "cards_80x40_all", render(s, tabCards, "", o))
}

func TestTopGolden(t *testing.T) {
	s := loadFixture(t)
	golden(t, "table_80x12", render(s, tabTable, "", opts(80, 12)))
	golden(t, "table_50x12", render(s, tabTable, "", opts(50, 12)))
	o := opts(80, 12)
	o.ShowEnded = true
	golden(t, "table_80x12_all", render(s, tabTable, "", o))
}

func TestErrorShowsInStatusBar(t *testing.T) {
	s := loadFixture(t)
	out := render(s, tabCards, "error: sitrep took longer than 5s (showing data from 12s ago)", opts(100, 20))
	lines := strings.Split(out, "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "showing data from 12s ago") {
		t.Errorf("the status bar should carry the error, got %q", last)
	}
	if !strings.Contains(out, "timescaledb") {
		t.Errorf("the last good data must stay on screen")
	}
}

// Whatever the size, no line may be wider than the screen, nor the frame taller.
func TestFramesFit(t *testing.T) {
	s := loadFixture(t)
	for _, tab := range []int{tabCards, tabTable} {
		for _, w := range []int{20, 30, 40, 60, 80, 120} {
			for _, h := range []int{5, 8, 12, 30} {
				for _, ended := range []bool{false, true} {
					o := opts(w, h)
					o.ShowEnded = ended
					out := render(s, tab, "", o)
					lines := strings.Split(out, "\n")
					if len(lines) > h {
						t.Errorf("tab %d %dx%d ended=%v: %d lines", tab, w, h, ended, len(lines))
					}
					for _, l := range lines {
						if ansi.StringWidth(l) > w {
							t.Errorf("tab %d %dx%d ended=%v: line wider than the screen (%d): %q", tab, w, h, ended, ansi.StringWidth(l), l)
						}
					}
				}
			}
		}
	}
}

func TestPlainThemeHasNoEscapes(t *testing.T) {
	out := render(loadFixture(t), tabCards, "", opts(80, 40))
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("the plain theme must not emit escape codes")
	}
}

func TestStyledMatchesPlainText(t *testing.T) {
	s := loadFixture(t)
	styled := viewOpts{Width: 80, Height: 40, Theme: newTheme(true, "48;5;238")}
	got := render(s, tabCards, "", styled)
	if !strings.ContainsRune(got, 0x1b) {
		t.Fatalf("a styled frame should have escape codes")
	}
	var plain []string
	for _, l := range strings.Split(ansi.Strip(got), "\n") {
		plain = append(plain, strings.TrimRight(l, " "))
	}
	want := strings.Split(render(s, tabCards, "", opts(80, 40)), "\n")
	for i := range want {
		want[i] = strings.TrimRight(want[i], " ")
	}
	// the tab labels differ (brackets against padding), so compare from the rule down
	if strings.Join(plain[1:], "\n") != strings.Join(want[1:], "\n") {
		t.Errorf("colour must only add style, not change the text")
	}
}

func TestHighlightOnlyForRecentRunning(t *testing.T) {
	s := loadFixture(t)
	hot := viewOpts{Width: 60, Height: 0, Theme: newTheme(true, "48;5;238")}
	lines := strings.Split(render(s, tabCards, "", hot), "\n")
	var hotLines, other int
	for _, l := range lines {
		if strings.Contains(l, "48;5;238") {
			hotLines++
		} else if strings.Contains(ansi.Strip(l), "idle?") || strings.Contains(ansi.Strip(l), "waiting 3m") {
			other++
		}
	}
	if hotLines == 0 {
		t.Errorf("the recent session should be highlighted")
	}
	if other == 0 {
		t.Errorf("the other sessions should be drawn without the highlight")
	}
}

func TestWrapText(t *testing.T) {
	if got := wrapText("alpha beta gamma delta", 11, 5); len(got) != 2 || got[0] != "alpha beta" {
		t.Errorf("simple wrap: %q", got)
	}
	got := wrapText(strings.Repeat("word ", 40), 20, 2)
	if len(got) != 2 || !strings.HasSuffix(got[1], "…") {
		t.Errorf("an overlong text keeps maxLines lines and ends in an ellipsis: %q", got)
	}
	for _, l := range wrapText("日本語のテキストが長い場合の折り返し", 10, 4) {
		if ansi.StringWidth(l) > 10 {
			t.Errorf("wide characters must count as two columns: %q (%d)", l, ansi.StringWidth(l))
		}
	}
	if got := wrapText("", 10, 3); len(got) != 1 || got[0] != "" {
		t.Errorf("empty text is one empty line: %q", got)
	}
	if got := wrapText(strings.Repeat("x", 50), 10, 9); len(got) != 5 {
		t.Errorf("a long word is broken: %q", got)
	}
}

func TestBgFromSGR(t *testing.T) {
	for in, ok := range map[string]bool{
		"48;5;238": true, "48;2;10;20;30": true, "44": true, "104": true,
		"": false, "x": false, "38;5;1": false, "48;5": false, "48;5;999": false, "1;2;3;4;5;6": false,
	} {
		if _, got := bgFromSGR(in); got != ok {
			t.Errorf("bgFromSGR(%q) ok = %v, want %v", in, got, ok)
		}
	}
}

func TestStatusBarKeepsTheKeysAt80Columns(t *testing.T) {
	s := loadFixture(t)
	lines := strings.Split(render(s, tabCards, "", opts(80, 20)), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "● 1 need you") || !strings.Contains(last, "q quit") {
		t.Errorf("the status bar should show who needs you and the keys: %q", last)
	}
}

func TestSummaryCounts(t *testing.T) {
	if got := summary(loadFixture(t)); got != "● 1 need you   3 running: 1 working, 1 waiting, 1 idle?, 2 ended" {
		t.Errorf("summary: %q", got)
	}
}

func TestTinyScreensNeverOverflow(t *testing.T) {
	s := loadFixture(t)
	for _, tab := range []int{tabCards, tabTable} {
		for w := 1; w <= 16; w++ {
			for h := 1; h <= 5; h++ {
				lines := strings.Split(render(s, tab, "", opts(w, h)), "\n")
				if len(lines) > h {
					t.Errorf("tab %d %dx%d: %d lines", tab, w, h, len(lines))
				}
				for _, l := range lines {
					if ansi.StringWidth(l) > w {
						t.Errorf("tab %d %dx%d: line wider than the screen: %q", tab, w, h, l)
					}
				}
			}
		}
	}
}

func TestHighlightClipsInsteadOfWrapping(t *testing.T) {
	o := viewOpts{Width: 20, Theme: newTheme(true, "48;5;238")}
	got := paint([]string{strings.Repeat("wide ", 20), "short"}, o.Theme.Hot, o.Width)
	for _, l := range got {
		if strings.Contains(l, "\n") || ansi.StringWidth(l) > 20 {
			t.Errorf("a highlighted line must be clipped to the width, not wrapped: %q", l)
		}
	}
}

func TestNeedsInputIsMarkedAndColoured(t *testing.T) {
	s := loadFixture(t)
	if !s.Sessions[2].NeedsInput || s.Sessions[0].NeedsInput {
		t.Fatalf("the fixture's waiting session needs input, the working one does not")
	}
	plain := render(s, tabCards, "", opts(80, 40))
	if strings.Count(plain, "● waiting 3m") != 1 || strings.Contains(plain, "● working") {
		t.Errorf("only the waiting session carries the marker:\n%s", plain)
	}
	top := render(s, tabTable, "", opts(80, 12))
	if !strings.Contains(top, "● waiting 3m") {
		t.Errorf("the Table tab marks it too:\n%s", top)
	}
	const attn, hot = "48;2;92;31;53", "48;5;238" // the dark attention colour, and the recent-activity colour
	styled := viewOpts{Width: 80, Height: 0, Theme: newTheme(true, "48;5;238")}
	var attnLines, hotLines int
	for _, l := range strings.Split(render(s, tabCards, "", styled), "\n") {
		if strings.Contains(l, attn) {
			attnLines++
		}
		if strings.Contains(l, hot) {
			hotLines++
		}
	}
	if attnLines == 0 || hotLines == 0 {
		t.Errorf("both highlights should appear: attention %d, recent %d", attnLines, hotLines)
	}
}

func TestNeedsInputBeatsRecentActivity(t *testing.T) {
	s := loadFixture(t)
	s.Sessions[2].Highlight = true // recent as well as waiting
	styled := viewOpts{Width: 80, Height: 0, Theme: newTheme(true, "48;5;238")}
	for _, l := range strings.Split(render(s, tabCards, "", styled), "\n") {
		if strings.Contains(ansi.Strip(l), "waiting for your call on the branch") {
			if !strings.Contains(l, "48;2;92;31;53") || strings.Contains(l, "48;5;238") {
				t.Errorf("a waiting session shows the attention colour even when it is recent: %q", l)
			}
		}
	}
}

func TestAFadedSessionKeepsItsMarkerButNotItsColour(t *testing.T) {
	s := loadFixture(t)
	s.Sessions[2].AttnFaded = true
	styled := viewOpts{Width: 80, Height: 0, Theme: newTheme(true, "48;5;238")}
	found := false
	for _, l := range strings.Split(render(s, tabCards, "", styled), "\n") {
		if strings.Contains(ansi.Strip(l), "waiting for your call on the branch") || strings.Contains(ansi.Strip(l), "● waiting 3m") {
			found = true
			if strings.Contains(l, "48;2;92;31;53") {
				t.Errorf("a faded session is not painted: %q", l)
			}
		}
	}
	if !found {
		t.Fatal("the session is not on screen")
	}
	if got := render(s, tabCards, "", opts(80, 40)); strings.Count(got, "● waiting 3m") != 1 {
		t.Errorf("the marker stays:\n%s", got)
	}
	if needYou(s) != 1 {
		t.Errorf("it still counts as needing you: %d", needYou(s))
	}
}

func TestEndedSessionsNeverNeedYou(t *testing.T) {
	s := loadFixture(t)
	s.Sessions[3].NeedsInput = true // an ended session, which the shell would never flag
	o := opts(80, 40)
	o.ShowEnded = true
	if got := render(s, tabCards, "", o); strings.Contains(got, "● ended") {
		t.Errorf("an ended session must not be marked:\n%s", got)
	}
	if needYou(s) != 1 {
		t.Errorf("only running sessions count: %d", needYou(s))
	}
}

func ids(list []Session) string {
	var out []string
	for _, x := range list {
		out = append(out, x.Short)
	}
	return strings.Join(out, " ")
}

func TestSortOrders(t *testing.T) {
	list, _ := shown(loadFixture(t), false)
	if got := ids(list); got != "15f1 2cd8 b1fb" {
		t.Fatalf("the fixture order is activity order: %s", got)
	}
	for _, c := range []struct {
		by   int
		want string
	}{
		{sortActivity, "15f1 2cd8 b1fb"},
		{sortState, "b1fb 15f1 2cd8"}, // needs you, then working, then idle
		{sortDir, "15f1 2cd8 b1fb"},   // claude-sitrep, savannah..., timescaledb...
		{sortQueue, "15f1 b1fb 2cd8"}, // 7 items, 1 item, empty
	} {
		if got := ids(sorted(list, c.by)); got != c.want {
			t.Errorf("sort %s: got %s, want %s", sortNames[c.by], got, c.want)
		}
	}
	if ids(list) != "15f1 2cd8 b1fb" {
		t.Errorf("sorting must not change the caller's slice")
	}
}

func TestSortTiesKeepSitrepOrder(t *testing.T) {
	list := []Session{{Short: "aaaa", Dir: "x"}, {Short: "bbbb", Dir: "x"}, {Short: "cccc", Dir: "x"}}
	for by := 0; by < sortCount; by++ {
		if got := ids(sorted(list, by)); got != "aaaa bbbb cccc" {
			t.Errorf("sort %s reshuffled equal rows: %s", sortNames[by], got)
		}
	}
}

func TestTopNamesItsSort(t *testing.T) {
	s := loadFixture(t)
	for by := 0; by < sortCount; by++ {
		o := opts(100, 12)
		o.Sort = by
		if got := render(s, tabTable, "", o); !strings.Contains(got, "sort: "+sortNames[by]) {
			t.Errorf("the Table view should say it is sorted by %s:\n%s", sortNames[by], got)
		}
	}
	o := opts(80, 12)
	o.Sort = sortState
	golden(t, "table_80x12_by_state", render(s, tabTable, "", o))
	if got := render(s, tabCards, "", opts(100, 40)); strings.Contains(got, "sort:") {
		t.Errorf("the Cards view has no sort")
	}
}

func TestStatusBarHints(t *testing.T) {
	last := func(w int) string {
		lines := strings.Split(render(loadFixture(t), tabCards, "", opts(w, 20)), "\n")
		return lines[len(lines)-1]
	}
	if got := last(80); !strings.Contains(got, "enter jump") || !strings.Contains(got, "? help") || !strings.Contains(got, "● 1 need you") || strings.Contains(got, "s sort") {
		t.Errorf("at 80 columns the Cards bar keeps who needs you and the key hints, and has no sort: %q", got)
	}
	top := strings.Split(render(loadFixture(t), tabTable, "", opts(80, 20)), "\n")
	if got := top[len(top)-1]; !strings.Contains(got, "s sort") || !strings.Contains(got, "? help") {
		t.Errorf("the Table bar has the sort: %q", got)
	}
}

func TestFooterHintsDropFromTheEndAndKeepTheHelp(t *testing.T) {
	hints := []string{"q quit", "enter jump", "/ filter", "n name", "? help"}
	for w, want := range map[int]string{60: " q quit  enter jump  / filter  n name  ? help ", 30: " q quit  enter jump  ? help ", 20: " q quit  ? help ", 12: " ? help ", 8: " ? help ", 6: ""} {
		if got := fitHints(hints, w); got != want {
			t.Errorf("width %d: %q, want %q", w, got, want)
		}
	}
	for _, tab := range []int{tabCards, tabTable} {
		for w := 1; w <= 120; w++ {
			last := func(out string) string { l := strings.Split(out, "\n"); return l[len(l)-1] }
			if got := last(render(loadFixture(t), tab, "", opts(w, 12))); ansi.StringWidth(got) > w {
				t.Fatalf("tab %d width %d: the bar is %d wide: %q", tab, w, ansi.StringWidth(got), got)
			}
		}
	}
}

func TestHelpScreen(t *testing.T) {
	out := helpScreen(80, 60, false, 0, plainTheme())
	for _, want := range []string{"Keys", "switch tab", "ended sessions", "Table sort", "refresh", "quit", "Cards", "Table ", "pink-red", "purple", "any key closes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the help should mention %q", want)
		}
	}
	golden(t, "help_80x60", out)
	for w := 1; w <= 100; w += 7 {
		for h := 1; h <= 40; h += 3 {
			lines := strings.Split(helpScreen(w, h, false, 0, plainTheme()), "\n")
			if len(lines) > h {
				t.Errorf("help %dx%d has %d lines", w, h, len(lines))
			}
			for _, l := range lines {
				if ansi.StringWidth(l) > w {
					t.Errorf("help %dx%d: a line is wider than the screen: %q", w, h, l)
				}
			}
		}
	}
}

func TestVisibleIDsFollowTheTab(t *testing.T) {
	s := loadFixture(t)
	if got := strings.Join(visibleIDs(s, tabCards, false, sortState, ""), " "); !strings.HasPrefix(got, "b1fb") {
		t.Errorf("Cards ignores the sort and uses the Wall's order, needs you first: %s", got)
	}
	if got := visibleIDs(s, tabTable, false, sortState, ""); len(got) != 3 || !strings.HasPrefix(got[0], "b1fb") {
		t.Errorf("Table follows the sort: %v", got)
	}
	if got := visibleIDs(s, tabCards, true, sortActivity, ""); len(got) != 4 {
		t.Errorf("ended sessions appear when asked for: %v", got)
	}
}

func TestSelectionMarker(t *testing.T) {
	s := loadFixture(t)
	o := opts(80, 40)
	o.Sel = s.Sessions[2].ID
	for _, tab := range []int{tabCards, tabTable} {
		out := render(s, tab, "", o)
		if strings.Count(out, "▶ ") != 1 {
			t.Fatalf("tab %d: exactly one marker:\n%s", tab, out)
		}
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "▶ ") && !strings.Contains(l, "b1fb") {
				t.Errorf("tab %d: the marker is on the wrong row: %q", tab, l)
			}
		}
	}
	o.Sel = ""
	if strings.Contains(render(s, tabCards, "", o), "▶") {
		t.Errorf("no selection, no marker")
	}
	o.Sel, o.Width = s.Sessions[2].ID, 19
	if strings.Contains(render(s, tabTable, "", o), "▶") {
		t.Errorf("a narrow screen has no gutter")
	}
	golden(t, "cards_80x40_sel", render(s, tabCards, "", viewOpts{Width: 80, Height: 40, Sel: s.Sessions[1].ID, Theme: plainTheme()}))
	golden(t, "table_80x12_sel", render(s, tabTable, "", viewOpts{Width: 80, Height: 12, Sel: s.Sessions[1].ID, Theme: plainTheme()}))
}

func TestScrollingKeepsTheSelectionOnScreen(t *testing.T) {
	s := loadFixture(t)
	for _, tab := range []int{tabCards, tabTable} {
		minH := 10 // below this the chrome, the footers and one block cannot all fit
		if tab == tabTable {
			minH = 7
		}
		for _, sess := range s.Sessions[:3] {
			for h := minH; h <= 30; h++ {
				o := viewOpts{Width: 80, Height: h, Sel: sess.ID, Theme: plainTheme()}
				out := render(s, tab, "", o)
				if !strings.Contains(out, "▶ "+sess.Short) {
					t.Errorf("tab %d height %d: the selected session %s is not on screen:\n%s", tab, h, sess.Short, out)
				}
			}
		}
	}
	// the last session, in a short Cards frame: blocks above are scrolled off and counted
	o := viewOpts{Width: 80, Height: 15, Sel: s.Sessions[1].ID, Theme: plainTheme()}
	out := render(s, tabCards, "", o)
	if !strings.Contains(out, "+2 above") && !strings.Contains(out, "+1 above") {
		t.Errorf("scrolled-off blocks are counted:\n%s", out)
	}
	golden(t, "cards_80x15_scrolled", out)
}

func TestAliasInTheDashboard(t *testing.T) {
	s := loadFixture(t)
	if len(s.Sessions) < 2 {
		t.Fatal("the fixture needs two sessions")
	}
	s.Sessions[0].Alias = "sitrep"
	s.Sessions[1].Alias = "a-name-that-is-much-too-long"
	for _, tab := range []int{tabCards, tabTable} {
		for _, w := range []int{20, 40, 80, 120} {
			o := opts(w, 40)
			o.ShowEnded = true
			out := render(s, tab, "", o)
			for _, l := range strings.Split(out, "\n") {
				if ansi.StringWidth(l) > w {
					t.Errorf("tab %d width %d: wide line %q", tab, w, l)
				}
			}
			if w >= 80 && (!strings.Contains(out, "sitrep ") || !strings.Contains(out, "a-name-that-is-…")) {
				t.Errorf("tab %d: the labels are not shown\n%s", tab, out)
			}
		}
	}
	// the column does not move while a filter is typed: it is as wide as the whole snapshot needs
	if w := idWidth(s); w != maxLabelWidth {
		t.Errorf("width %d", w)
	}
	only := render(s, tabTable, strings.ToLower(s.Sessions[0].ID[:4]), opts(80, 40))
	if !strings.Contains(only, "ID                ") {
		t.Errorf("the column narrowed with a filter:\n%s", only)
	}
	// the filter finds an alias
	if !matches(s.Sessions[0], "sitrep") || matches(s.Sessions[1], "sitrep") {
		t.Error("the filter does not search the alias")
	}
	s.Sessions[0].Alias = ""
	if w := idWidth(s); w != maxLabelWidth {
		t.Errorf("width %d", w)
	}
}

func TestHostileAliasIsDroppedOnDecode(t *testing.T) {
	doc := `{"version":1,"generated":1,"registry":"ok","settings":{},"hidden":{},"inbox":{"count":0,"items":[]},"sessions":[
	 {"id":"a1","short":"a1","alias":"ok-name","queue":{"count":0,"items":[]}},
	 {"id":"a2","short":"a2","alias":"bad\u001b[31m","queue":{"count":0,"items":[]}},
	 {"id":"a3","short":"a3","alias":"Upper","queue":{"count":0,"items":[]}},
	 {"id":"a4","short":"a4","alias":"x‮y","queue":{"count":0,"items":[]}},
	 {"id":"a5","short":"a5","alias":"inbox","queue":{"count":0,"items":[]}}]}`
	s, err := decode(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, x := range s.Sessions {
		got = append(got, x.Label())
	}
	if strings.Join(got, ",") != "ok-name,a2,a3,a4,a5" {
		t.Errorf("labels %v", got)
	}
}

func TestSelectionIsASolidBarOverTheWholeBlock(t *testing.T) {
	s := loadFixture(t)
	o := viewOpts{Width: 80, Height: 40, Theme: newTheme(true, "48;5;238"), Sel: s.Sessions[0].ID}
	lines := strings.Split(render(s, tabCards, "", o), "\n")
	bar, first := 0, -1
	for i, l := range lines {
		if strings.HasPrefix(l, o.Theme.StatusKey.Render("  ")) || strings.HasPrefix(l, o.Theme.StatusKey.Render("▶ ")) {
			bar++
			if strings.HasPrefix(l, o.Theme.StatusKey.Render("▶ ")) {
				first = i
			}
		}
	}
	if first < 0 || bar < 5 {
		t.Errorf("the selected block has %d bar lines, arrow at %d:\n%s", bar, first, strings.Join(lines, "\n"))
	}
	if !strings.Contains(ansi.Strip(lines[first]), s.Sessions[0].Short) {
		t.Errorf("the arrow is not on the session row: %q", ansi.Strip(lines[first]))
	}
}

func TestContextColumn(t *testing.T) {
	s := loadFixture(t)
	o := opts(100, 20)
	for want, x := range map[string]Context{"62%": {UsedPercent: 37.5, Window: 1}, "100%": {UsedPercent: 0, Window: 1}, "20%": {UsedPercent: 80, Window: 1}, "21%": {UsedPercent: 78.2, Window: 1}, "!10%": {UsedPercent: 90, Window: 1}, "!9%": {UsedPercent: 90.1, Window: 1}, "!0%": {UsedPercent: 100, Window: 1}} {
		x := x
		sess := Session{Running: true, Context: &x}
		if got := strings.TrimSpace(ansi.Strip(ctxCell(sess, o))); got != want {
			t.Errorf("%+v: %q, want %q", x, got, want)
		}
	}
	for name, sess := range map[string]Session{"no record": {Running: true}, "ended": {Running: false, Context: &Context{UsedPercent: 5, Window: 1}}} {
		if got := strings.TrimSpace(ctxCell(sess, o)); got != "-" {
			t.Errorf("%s: %q", name, got)
		}
	}
	for _, w := range []int{80, 100, 120} {
		top := render(s, tabTable, "", opts(w, 12))
		if !strings.Contains(top, "CTX") || !strings.Contains(top, "62%") || !strings.Contains(top, "!6%") {
			t.Errorf("width %d: the column and its values are drawn:\n%s", w, top)
		}
	}
	if w := ansi.StringWidth(ctxCell(Session{Running: true, Context: &Context{UsedPercent: 0}}, o)); w != ctxWidth {
		t.Errorf("width %d", w)
	}
}

func TestContextColumnDropsByTheRoomNowHas(t *testing.T) {
	s := loadFixture(t)
	for w := 1; w <= 140; w++ {
		top := render(s, tabTable, "", opts(w, 12))
		for _, l := range strings.Split(top, "\n") {
			if ansi.StringWidth(l) > w {
				t.Fatalf("width %d: a line is %d wide: %q", w, ansi.StringWidth(l), l)
			}
		}
		// when the column is there, the text of NOW keeps its room
		for _, l := range strings.Split(top, "\n") {
			if strings.Contains(l, "15f1") && strings.Contains(l, "62%") {
				i := strings.Index(l, "62%") + len("62%") + 2
				if room := w - ansi.StringWidth(l[:i]); room < minNow {
					t.Errorf("width %d: NOW has %d columns: %q", w, room, l)
				}
			}
		}
	}
	if top := render(s, tabTable, "", opts(50, 12)); strings.Contains(top, "CTX") {
		t.Errorf("a narrow screen has no CTX column:\n%s", top)
	}
	if top := render(s, tabTable, "", opts(80, 12)); !strings.Contains(top, "CTX") {
		t.Errorf("80 columns has it:\n%s", top)
	}
	// the column appears, and disappears, with a change of width: both are exercised
	seen := map[bool]bool{}
	for w := 40; w <= 120; w++ {
		seen[strings.Contains(render(s, tabTable, "", opts(w, 12)), "CTX")] = true
	}
	if !seen[true] || !seen[false] {
		t.Errorf("the sweep must cross the threshold: %v", seen)
	}
}

func TestContextBoldNeverInsideAColouredRow(t *testing.T) {
	th := newTheme(true, "48;5;238")
	o := viewOpts{Width: 100, Height: 20, Theme: th}
	low := &Context{UsedPercent: 85, Window: 1} // 15% left: bold
	plainRow := Session{Running: true, Context: low}
	if got := ctxCell(plainRow, o); !strings.Contains(got, "\x1b[1m") {
		t.Errorf("a plain row gets bold: %q", got)
	}
	for name, x := range map[string]Session{"needs you": {Running: true, NeedsInput: true, Context: low}, "recent": {Running: true, Highlight: true, Context: low}} {
		if got := ctxCell(x, o); strings.Contains(got, "\x1b") {
			t.Errorf("%s: a style inside a coloured row would end its colour: %q", name, got)
		}
	}
	ok := &Context{UsedPercent: 50, Window: 1}
	if got := ctxCell(Session{Running: true, Context: ok}, o); strings.Contains(got, "\x1b") {
		t.Errorf("no bold above 20%%: %q", got)
	}
	// the plain theme has no style, and the marker is still there
	if got := ctxCell(Session{Running: true, Context: &Context{UsedPercent: 95, Window: 1}}, opts(100, 20)); strings.TrimSpace(got) != "!5%" {
		t.Errorf("plain: %q", got)
	}
}

func TestContextAgeComesFromTheDocumentsOwnTime(t *testing.T) {
	doc := `{"version":1,"generated":2000,"registry":"ok","settings":{},"hidden":{},"inbox":{"count":0,"items":[]},"sessions":[
	 {"id":"a1","short":"a1","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":40,"window":200000,"tokens":80000,"at":1940}},
	 {"id":"a2","short":"a2","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":40,"window":200000,"tokens":80000,"at":5000}},
	 {"id":"a3","short":"a3","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":40,"window":200000,"tokens":80000,"at":0}}]}`
	s, err := decode(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	var ages []int64
	for _, x := range s.Sessions {
		ages = append(ages, x.Context.Age)
	}
	if want := []int64{60, 0, 0}; !reflect.DeepEqual(ages, want) { // a time after the listing's own (a clock that moved) is 0, not negative
		t.Errorf("ages %v, want %v", ages, want)
	}
}

func TestContextOfTheDocumentIsChecked(t *testing.T) {
	doc := `{"version":1,"generated":1,"registry":"ok","settings":{},"hidden":{},"inbox":{"count":0,"items":[]},"sessions":[
	 {"id":"a1","short":"a1","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":40,"window":200000,"tokens":80000,"at":1}},
	 {"id":"a2","short":"a2","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":150,"window":200000}},
	 {"id":"a3","short":"a3","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":-1,"window":200000}},
	 {"id":"a4","short":"a4","running":true,"queue":{"count":0,"items":[]},"context":{"used_percent":5,"window":0}},
	 {"id":"a5","short":"a5","running":true,"queue":{"count":0,"items":[]},"context":null},
	 {"id":"a6","short":"a6","running":true,"queue":{"count":0,"items":[]}}]}`
	s, err := decode(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	var got []bool
	for _, x := range s.Sessions {
		got = append(got, x.Context != nil)
	}
	if want := []bool{true, false, false, false, false, false}; !reflect.DeepEqual(got, want) {
		t.Errorf("records kept: %v, want %v", got, want)
	}
	if s.Sessions[0].Context.Left() != 60 {
		t.Errorf("left %d", s.Sessions[0].Context.Left())
	}
}

func TestTokenAndPercentText(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 950: "950", 999: "999", 1000: "1k", 1500: "1.5k", 12_500: "12.5k", 186_800: "186.8k", 375_000: "375k", 999_999: "1000k", 1_000_000: "1M", 1_250_000: "1.25M", 2_000_000: "2M", 10_000_000_000: "10000M"} {
		if got := tokenText(in); got != want {
			t.Errorf("tokenText(%d) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[float64]string{0: "0", 37.5: "37.5", 56: "56", 56.04: "56", 93.45: "93.5", 100: "100", 6.6: "6.6"} {
		if got := percentText(in); got != want {
			t.Errorf("percentText(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestDetailShowsTheContextWindow(t *testing.T) {
	x := Session{ID: "a1", Running: true, Context: &Context{UsedPercent: 37.5, Window: 1_000_000, Tokens: 375_000, At: 1000, Age: 2}}
	if got := strings.Join(detailLines(x, 80), "\n"); !strings.Contains(got, "Context   375k / 1M tokens  (37.5% used, 62.5% left)  seen now") {
		t.Errorf("a running session:\n%s", got)
	}
	for age, want := range map[int64]string{4: "seen now", 5: "seen 5s ago", 59: "seen 59s ago", 60: "seen 1m ago", 3600: "seen 1h ago", 90000: "seen 25h ago"} {
		x.Context.Age = age
		if got := strings.Join(detailLines(x, 100), "\n"); !strings.Contains(got, "left)  "+want) {
			t.Errorf("age %d: want %q in\n%s", age, want, got)
		}
	}
	x.Context.At = 0 // a record with no time says nothing about it
	if got := strings.Join(detailLines(x, 100), "\n"); strings.Contains(got, "seen") {
		t.Errorf("no time:\n%s", got)
	}
	x.Context = &Context{UsedPercent: 37.5, Window: 1_000_000, Tokens: 375_000, At: 1000}
	x.Context = &Context{UsedPercent: 93.4, Window: 200_000, Tokens: 186_800}
	if got := strings.Join(detailLines(x, 80), "\n"); !strings.Contains(got, "186.8k / 200k tokens  (93.4% used, 6.6% left)") {
		t.Errorf("a nearly full window:\n%s", got)
	}
	x.Context = nil
	if got := strings.Join(detailLines(x, 80), "\n"); !strings.Contains(got, "Context   not reported yet") {
		t.Errorf("a running session that has not said:\n%s", got)
	}
	x.Running = false
	if got := strings.Join(detailLines(x, 80), "\n"); strings.Contains(got, "Context") {
		t.Errorf("an ended session has no line:\n%s", got)
	}
	x.Context = &Context{UsedPercent: 50, Window: 1000, Tokens: 500}
	if got := strings.Join(detailLines(x, 80), "\n"); strings.Contains(got, "Context") {
		t.Errorf("nor an ended one with an old record:\n%s", got)
	}
	// a narrow screen wraps the line and cuts nothing
	x.Running, x.Context = true, &Context{UsedPercent: 37.5, Window: 1_000_000, Tokens: 375_000}
	if got := strings.Join(strings.Fields(strings.Join(detailLines(x, 30), "\n")), " "); !strings.Contains(got, "375k / 1M tokens (37.5% used, 62.5% left)") {
		t.Errorf("narrow:\n%s", got)
	}
}

func TestBlockedSessionCountsAsWaitingAndShowsWhy(t *testing.T) {
	snap := Snapshot{Sessions: []Session{
		{ID: "a", Running: true, State: "working", StateLabel: "blocked 1m", NeedsInput: true, Blocked: "permission"},
		{ID: "b", Running: true, State: "working", StateLabel: "working 5s"},
	}}
	if _, working, waiting, _, _ := counts(snap); working != 1 || waiting != 1 {
		t.Errorf("working %d, waiting %d", working, waiting)
	}
	if got := stateText(snap.Sessions[0]); got != "● blocked 1m" {
		t.Errorf("state text %q", got)
	}
	snap.sanitize()
	bad := Snapshot{Sessions: []Session{{ID: "a", Blocked: "\x1b[31mx"}}}
	bad.sanitize()
	if bad.Sessions[0].Blocked != "" {
		t.Error("an unknown kind must be dropped")
	}
}

func TestHelpWrapsInsteadOfCuttingOnANarrowScreen(t *testing.T) {
	for _, w := range []int{50, 60, 70, 80, 100} {
		out := helpScreen(w, 140, false, 0, plainTheme())
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "…") {
				t.Errorf("width %d cuts a line: %q", w, l)
			}
			if ansi.StringWidth(l) > w {
				t.Errorf("width %d: %q is %d wide", w, l, ansi.StringWidth(l))
			}
		}
		flat := strings.Join(strings.Fields(out), " ")
		for _, want := range []string{"the selected session in full (q closes; e, i: queues)", "snooze it until it changes; z again ends it", "the colour fades after 10 minutes (SITREP_ATTN_FADE); the ● stays", "any key closes"} {
			if !strings.Contains(flat, want) {
				t.Errorf("width %d: the help lost %q", w, want)
			}
		}
	}
	// a screen too narrow for a description column still draws, cut
	if out := helpScreen(20, 30, false, 0, plainTheme()); !strings.Contains(out, "…") {
		t.Error("a very narrow screen cuts")
	}
}

func TestAKeptFilterSaysHowToClearIt(t *testing.T) {
	s := loadFixture(t)
	o := viewOpts{Width: 110, Height: 24, Theme: plainTheme(), Filter: "api"}
	if got := render(s, tabCards, "", o); !strings.Contains(got, "esc clear filter") {
		t.Errorf("a filter that is on says how to clear it:\n%s", got)
	}
	o.Typing = true
	if got := render(s, tabCards, "", o); strings.Contains(got, "esc clear filter") || !strings.Contains(got, "esc cancel") {
		t.Errorf("while typing the footer lists the typing keys:\n%s", got)
	}
	o.Filter, o.Typing = "", false
	if got := render(s, tabCards, "", o); strings.Contains(got, "esc clear filter") {
		t.Error("no filter, no hint")
	}
}

func TestNoMatchSaysWhenEndedSessionsMatch(t *testing.T) {
	s := loadFixture(t)
	o := viewOpts{Width: 100, Height: 20, Filter: "ended", Theme: plainTheme()}
	var gone Session
	for _, x := range s.Sessions {
		if !x.Running {
			gone = x
		}
	}
	o.Filter = gone.Short
	if out := render(s, tabCards, "", o); !strings.Contains(out, "ended hidden, press a") {
		t.Errorf("a filter that matches only ended sessions says so:\n%s", out)
	}
	o.ShowEnded = true
	if out := render(s, tabCards, "", o); strings.Contains(out, "hidden") {
		t.Errorf("with ended sessions shown there is nothing hidden:\n%s", out)
	}
	o.ShowEnded, o.Filter = false, "zzzz-nothing"
	if out := render(s, tabCards, "", o); !strings.Contains(out, "no session matches the filter") || strings.Contains(out, "hidden") {
		t.Errorf("no match at all:\n%s", out)
	}
}

func TestTableColumnsDoNotChangeWithTheFilter(t *testing.T) {
	s := loadFixture(t)
	head := func(filter string) string {
		return strings.Split(render(s, tabTable, "", viewOpts{Width: 100, Height: 20, Filter: filter, Theme: plainTheme()}), "\n")[2]
	}
	if all, one := head(""), head("timescaledb"); all != one {
		t.Errorf("the header moves with the filter:\n%q\n%q", all, one)
	}
}

func TestHelpScrolls(t *testing.T) {
	long := helpScreen(60, 10, false, 0, plainTheme())
	if !strings.Contains(long, "j/k scroll") {
		t.Errorf("a cut list says it scrolls:\n%s", long)
	}
	end := helpScreen(60, 10, false, 1<<20, plainTheme())
	if !strings.Contains(end, "snoozed") || end == long {
		t.Errorf("scrolled to the end the colours show:\n%s", end)
	}
	if tall := helpScreen(60, 80, false, 5, plainTheme()); !strings.Contains(tall, "any key closes") {
		t.Errorf("a list that fits does not scroll:\n%s", tall)
	}
}
