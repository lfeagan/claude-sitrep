package main

import (
	"flag"
	"os"
	"path/filepath"
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

func TestWatchGolden(t *testing.T) {
	s := loadFixture(t)
	golden(t, "watch_80x40", render(s, tabWatch, "", opts(80, 40)))
	golden(t, "watch_48x40", render(s, tabWatch, "", opts(48, 40)))
	golden(t, "watch_80x14_cut", render(s, tabWatch, "", opts(80, 14)))
	o := opts(80, 40)
	o.ShowEnded = true
	golden(t, "watch_80x40_all", render(s, tabWatch, "", o))
}

func TestTopGolden(t *testing.T) {
	s := loadFixture(t)
	golden(t, "top_80x12", render(s, tabTop, "", opts(80, 12)))
	golden(t, "top_50x12", render(s, tabTop, "", opts(50, 12)))
	o := opts(80, 12)
	o.ShowEnded = true
	golden(t, "top_80x12_all", render(s, tabTop, "", o))
}

func TestErrorShowsInStatusBar(t *testing.T) {
	s := loadFixture(t)
	out := render(s, tabWatch, "error: sitrep took longer than 5s (showing data from 12s ago)", opts(100, 20))
	lines := strings.Split(out, "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "showing data from 12s ago") {
		t.Errorf("the status bar should carry the error, got %q", last)
	}
	if !strings.Contains(out, "claude-sitrep") {
		t.Errorf("the last good data must stay on screen")
	}
}

// Whatever the size, no line may be wider than the screen, nor the frame taller.
func TestFramesFit(t *testing.T) {
	s := loadFixture(t)
	for _, tab := range []int{tabWatch, tabTop} {
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
	out := render(loadFixture(t), tabWatch, "", opts(80, 40))
	if strings.ContainsRune(out, 0x1b) {
		t.Errorf("the plain theme must not emit escape codes")
	}
}

func TestStyledMatchesPlainText(t *testing.T) {
	s := loadFixture(t)
	styled := viewOpts{Width: 80, Height: 40, Theme: newTheme(true, "48;5;238")}
	got := render(s, tabWatch, "", styled)
	if !strings.ContainsRune(got, 0x1b) {
		t.Fatalf("a styled frame should have escape codes")
	}
	var plain []string
	for _, l := range strings.Split(ansi.Strip(got), "\n") {
		plain = append(plain, strings.TrimRight(l, " "))
	}
	want := strings.Split(render(s, tabWatch, "", opts(80, 40)), "\n")
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
	lines := strings.Split(render(s, tabWatch, "", hot), "\n")
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
	lines := strings.Split(render(s, tabWatch, "", opts(80, 20)), "\n")
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
	for _, tab := range []int{tabWatch, tabTop} {
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
	plain := render(s, tabWatch, "", opts(80, 40))
	if strings.Count(plain, "● waiting 3m") != 1 || strings.Contains(plain, "● working") {
		t.Errorf("only the waiting session carries the marker:\n%s", plain)
	}
	top := render(s, tabTop, "", opts(80, 12))
	if !strings.Contains(top, "● waiting 3m") {
		t.Errorf("the Top tab marks it too:\n%s", top)
	}
	const attn, hot = "48;2;92;31;53", "48;5;238" // the dark attention colour, and the recent-activity colour
	styled := viewOpts{Width: 80, Height: 0, Theme: newTheme(true, "48;5;238")}
	var attnLines, hotLines int
	for _, l := range strings.Split(render(s, tabWatch, "", styled), "\n") {
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
	for _, l := range strings.Split(render(s, tabWatch, "", styled), "\n") {
		if strings.Contains(ansi.Strip(l), "waiting for your call on the branch") {
			if !strings.Contains(l, "48;2;92;31;53") || strings.Contains(l, "48;5;238") {
				t.Errorf("a waiting session shows the attention colour even when it is recent: %q", l)
			}
		}
	}
}

func TestEndedSessionsNeverNeedYou(t *testing.T) {
	s := loadFixture(t)
	s.Sessions[3].NeedsInput = true // an ended session, which the shell would never flag
	o := opts(80, 40)
	o.ShowEnded = true
	if got := render(s, tabWatch, "", o); strings.Contains(got, "● ended") {
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
		if got := render(s, tabTop, "", o); !strings.Contains(got, "sort: "+sortNames[by]) {
			t.Errorf("the Top view should say it is sorted by %s:\n%s", sortNames[by], got)
		}
	}
	o := opts(80, 12)
	o.Sort = sortState
	golden(t, "top_80x12_by_state", render(s, tabTop, "", o))
	if got := render(s, tabWatch, "", opts(100, 40)); strings.Contains(got, "sort:") {
		t.Errorf("the Watch view has no sort")
	}
}

func TestStatusBarHints(t *testing.T) {
	last := func(w int) string {
		lines := strings.Split(render(loadFixture(t), tabWatch, "", opts(w, 20)), "\n")
		return lines[len(lines)-1]
	}
	if got := last(80); !strings.Contains(got, "s sort") || !strings.Contains(got, "? help") || !strings.Contains(got, "● 1 need you") {
		t.Errorf("at 80 columns the bar keeps who needs you and the key hints: %q", got)
	}
}

func TestHelpScreen(t *testing.T) {
	out := helpScreen(80, 30, plainTheme())
	for _, want := range []string{"Keys", "switch tab", "ended sessions", "Top sort", "refresh", "quit", "Watch", "Top ", "pink-red", "purple", "Any key closes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the help should mention %q", want)
		}
	}
	golden(t, "help_80x30", out)
	for w := 1; w <= 100; w += 7 {
		for h := 1; h <= 40; h += 3 {
			lines := strings.Split(helpScreen(w, h, plainTheme()), "\n")
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
