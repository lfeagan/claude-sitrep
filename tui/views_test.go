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

func TestSummaryCounts(t *testing.T) {
	if got := summary(loadFixture(t)); got != "3 running: 1 working, 1 waiting, 1 idle?, 2 ended" {
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
	got := highlight([]string{strings.Repeat("wide ", 20), "short"}, o)
	for _, l := range got {
		if strings.Contains(l, "\n") || ansi.StringWidth(l) > 20 {
			t.Errorf("a highlighted line must be clipped to the width, not wrapped: %q", l)
		}
	}
}
