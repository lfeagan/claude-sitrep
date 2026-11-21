package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestParseColors(t *testing.T) {
	got := parseColors("accent=#FF8800, attn=#400020/#ffd0d0 ,hot=236,dim=7/243,error=9")
	want := map[string][2]string{
		"accent": {"#ff8800", "#ff8800"}, "attn": {"#400020", "#ffd0d0"}, "hot": {"236", "236"}, "dim": {"7", "243"}, "error": {"9", "9"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
	for _, bad := range []string{"", "accent", "accent=", "accent=#fff", "accent=#gggggg", "accent=256", "accent=-1", "accent=01", "accent=1.5",
		"nothing=#ffffff", "accent=#ffffff/", "accent=/#ffffff", "accent=#ffffff/#xyzxyz", "=#ffffff", "accent=red", "accent=\x1b[31m", "accent=#ffffff#"} {
		if got := parseColors(bad); len(got) != 0 {
			t.Errorf("%q gave %v", bad, got)
		}
	}
	if got := parseColors("bogus=1,accent=#112233,attn=nope"); !reflect.DeepEqual(got, map[string][2]string{"accent": {"#112233", "#112233"}}) {
		t.Errorf("a bad entry must not spoil the good ones: %v", got)
	}
}

func TestThemeDefaultIsUnchanged(t *testing.T) {
	for _, dark := range []bool{true, false} {
		a := themeFor(dark, Settings{HighlightSGR: "48;5;238"})
		b := newTheme(dark, "48;5;238")
		c := themeFor(dark, Settings{HighlightSGR: "48;5;238", Theme: "nonsense", Colors: "bogus=1"})
		for name, x := range map[string]Theme{"new": b, "unknown name and entry": c} {
			if !reflect.DeepEqual(a, x) {
				t.Errorf("dark=%v: %s differs from the default theme", dark, name)
			}
		}
	}
}

func TestThemeOverrides(t *testing.T) {
	base := themeFor(true, Settings{HighlightSGR: "48;5;238"})
	th := themeFor(true, Settings{HighlightSGR: "48;5;238", Colors: "attn=#112233,hot=#445566,accent=#00ff00"})
	if got := ansi.Strip(th.Attn.Render("x")); got != "x" {
		t.Fatal(got)
	}
	if th.Attn.Render("x") == base.Attn.Render("x") || !strings.Contains(th.Attn.Render("x"), "48;2;17;34;51") {
		t.Errorf("attn: %q", th.Attn.Render("x"))
	}
	if !strings.Contains(th.Hot.Render("x"), "48;2;68;85;102") {
		t.Errorf("hot: %q", th.Hot.Render("x"))
	}
	// a light accent gets dark text on it, and a dark one gets light text
	if !strings.Contains(th.StatusKey.Render("x"), "38;2;16;16;20") {
		t.Errorf("text on a light accent: %q", th.StatusKey.Render("x"))
	}
	dark := themeFor(true, Settings{HighlightSGR: "48;5;238", Colors: "accent=#101040"})
	if !strings.Contains(dark.StatusKey.Render("x"), "38;2;255;255;255") {
		t.Errorf("text on a dark accent: %q", dark.StatusKey.Render("x"))
	}
	// light/dark pairs follow the terminal
	pair := Settings{HighlightSGR: "48;5;238", Colors: "attn=#ff0000/#0000ff"}
	if !strings.Contains(themeFor(false, pair).Attn.Render("x"), "48;2;255;0;0") || !strings.Contains(themeFor(true, pair).Attn.Render("x"), "48;2;0;0;255") {
		t.Error("light and dark values")
	}
}

func TestThemeHighlightSettingStillWorks(t *testing.T) {
	th := themeFor(true, Settings{HighlightSGR: "48;2;1;2;3"})
	if !strings.Contains(th.Hot.Render("x"), "48;2;1;2;3") {
		t.Errorf("SITREP_HIGHLIGHT_SGR: %q", th.Hot.Render("x"))
	}
	// SITREP_COLORS wins over it
	th = themeFor(true, Settings{HighlightSGR: "48;2;1;2;3", Colors: "hot=#0a0b0c"})
	if !strings.Contains(th.Hot.Render("x"), "48;2;10;11;12") {
		t.Errorf("colors over highlight: %q", th.Hot.Render("x"))
	}
	// a theme's own highlight shows when the setting has its default value
	hc := themeFor(true, Settings{HighlightSGR: "48;5;238", Theme: "high-contrast"})
	if hc.Hot.Render("x") == themeFor(true, Settings{HighlightSGR: "48;5;238"}).Hot.Render("x") {
		t.Error("the high-contrast theme has its own highlight")
	}
}

func TestThemes(t *testing.T) {
	for _, name := range ThemeNames {
		for _, dark := range []bool{true, false} {
			th := themeFor(dark, Settings{HighlightSGR: "48;5;238", Theme: name})
			s := newModelWith(t, th)
			if !strings.Contains(s, "Watch") {
				t.Errorf("%s dark=%v: %q", name, dark, s)
			}
		}
	}
	mono := themeFor(true, Settings{Theme: "mono"})
	for name, st := range map[string]string{"attn": mono.Attn.Render("x"), "hot": mono.Hot.Render("x"), "status": mono.Status.Render("x"), "bar": mono.StatusKey.Render("x")} {
		if strings.Contains(st, "38;") || strings.Contains(st, "48;") {
			t.Errorf("mono %s has a colour: %q", name, st)
		}
		if st == "x" {
			t.Errorf("mono %s has no style at all", name)
		}
	}
}

// newModelWith draws the fixture in a theme and returns the frame without its escape codes.
func newModelWith(t *testing.T, th Theme) string {
	t.Helper()
	s := loadFixture(t)
	o := viewOpts{Width: 80, Height: 40, Theme: th, Sel: s.Sessions[0].ID}
	for _, tab := range []int{tabWatch, tabTop} {
		for _, l := range strings.Split(render(s, tab, "", o), "\n") {
			if ansi.StringWidth(l) > 80 {
				t.Errorf("a line wider than the screen: %q", l)
			}
		}
	}
	return ansi.Strip(render(s, tabWatch, "", o))
}

func TestRgbOf(t *testing.T) {
	for in, want := range map[string][3]int{"#ff8000": {255, 128, 0}, "0": {0, 0, 0}, "15": {255, 255, 255}, "9": {255, 85, 85}, "16": {0, 0, 0}, "231": {255, 255, 255}, "232": {8, 8, 8}, "255": {238, 238, 238}} {
		if r, g, b := rgbOf(in); [3]int{r, g, b} != want {
			t.Errorf("rgbOf(%q) = %d %d %d, want %v", in, r, g, b, want)
		}
	}
}

func TestSettingsFromTheDocumentAreCleaned(t *testing.T) {
	doc := `{"version":1,"generated":1,"registry":"ok","settings":{"theme":"high\u001b[31m-contrast","colors":"accent=#fff‮"},"hidden":{},"inbox":{"count":0,"items":[]},"sessions":[]}`
	s, err := decode(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(s.Settings.Theme+s.Settings.Colors, "\x1b‮") {
		t.Errorf("%+v", s.Settings)
	}
}
