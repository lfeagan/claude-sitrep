package tui

import (
	"math"
	"reflect"
	"slices"
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
			if !strings.Contains(s, "Cards") {
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
	for _, tab := range []int{tabCards, tabTable} {
		for _, l := range strings.Split(render(s, tab, "", o), "\n") {
			if ansi.StringWidth(l) > 80 {
				t.Errorf("a line wider than the screen: %q", l)
			}
		}
	}
	return ansi.Strip(render(s, tabCards, "", o))
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

func TestTextReadsOnAChosenBackground(t *testing.T) {
	pink := themeFor(true, Settings{HighlightSGR: "48;5;238", Colors: "attn=#ffd1dc"})
	if got := pink.Attn.Render("x"); !strings.Contains(got, "48;2;255;209;220") || !strings.Contains(got, "38;2;16;16;20") {
		t.Errorf("dark text on pastel pink: %q", got)
	}
	night := themeFor(false, Settings{HighlightSGR: "48;5;238", Colors: "hot=#101040"})
	if got := night.Hot.Render("x"); !strings.Contains(got, "38;2;255;255;255") {
		t.Errorf("light text on a dark highlight: %q", got)
	}
	// the built-in backgrounds keep the terminal's own text colour
	def := themeFor(true, Settings{HighlightSGR: "48;5;238"})
	if strings.Contains(def.Attn.Render("x"), "38;") || strings.Contains(def.Hot.Render("x"), "38;") {
		t.Errorf("the default theme sets text colours: %q %q", def.Attn.Render("x"), def.Hot.Render("x"))
	}
}

// contrast is the WCAG contrast ratio of two colours given as #rrggbb or a terminal colour number.
func contrast(a, b string) float64 {
	lum := func(c string) float64 {
		r, g, bl := rgbOf(c)
		ch := func(v int) float64 {
			f := float64(v) / 255
			if f <= 0.03928 {
				return f / 12.92
			}
			return math.Pow((f+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(r) + 0.7152*ch(g) + 0.0722*ch(bl)
	}
	l1, l2 := lum(a), lum(b)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func TestRainbowSetsItsOwnTextAndPastelDoesNot(t *testing.T) {
	st := Settings{HighlightSGR: "48;5;238"}
	st.Theme = "rainbow"
	if !strings.Contains(themeFor(true, st).Attn.Render("x"), "38;2;16;16;20") { // dark text on the bright yellow
		t.Errorf("rainbow attn: %q", themeFor(true, st).Attn.Render("x"))
	}
	st.Theme = "pastel"
	if strings.Contains(themeFor(true, st).Attn.Render("x"), "38;") {
		t.Errorf("pastel keeps the terminal's own text: %q", themeFor(true, st).Attn.Render("x"))
	}
	// no background of a colourful theme is brown: red must not be far above green and blue together
	for _, name := range []string{"pastel", "rainbow"} {
		p := presets[name]
		for _, bg := range []string{p.attn[1], p.hot[1]} {
			r, g, b := rgbOf(bg)
			if r > g+20 && g > b+20 && r < 130 {
				t.Errorf("%s dark %s is brown", name, bg)
			}
		}
	}
}

func TestColourfulThemes(t *testing.T) {
	for _, name := range []string{"pastel", "rainbow", "ocean", "forest", "homebrew", "red-sands", "novel", "nord", "solarized", "winter", "spring", "summer", "fall"} {
		checkColourfulTheme(t, name)
	}
}

func checkColourfulTheme(t *testing.T, name string) {
	p := presets[name]
	d := presets["default"]
	if p.accent == d.accent || p.attn == d.attn || p.hot == d.hot {
		t.Errorf("%s must have colours of its own", name)
	}
	for side, mode := range []string{"light", "dark"} {
		who := name + " " + mode
		// the status bar's text, chosen to read on the accent
		if c := contrast(onColor(p.accent[side]), p.accent[side]); c < 4.5 {
			t.Errorf("%s: text on the accent has contrast %.1f", who, c)
		}
		// what needs you must stand out from recent activity
		if p.attn[side] == p.hot[side] {
			t.Errorf("%s: attn and hot are the same", who)
		}
		// the terminal's own text colour (near black on light, near white on dark) must read on both backgrounds
		for role, bg := range map[string]string{"hot": p.hot[side], "attn": p.attn[side]} {
			text := map[int]string{0: "#1a1a1a", 1: "#e6e6e6"}[side]
			if solidThemes[name] {
				text = onColor(bg) // a theme with bright backgrounds on any terminal sets its own text
			}
			if c := contrast(text, bg); c < 7 {
				t.Errorf("%s %s: terminal text has contrast %.1f", who, role, c)
			}
		}
		// the error and dim colours are text on the terminal's background
		bgTerm := map[int]string{0: "#ffffff", 1: "#1e1e1e"}[side]
		if c := contrast(p.errc[side], bgTerm); c < 3 {
			t.Errorf("%s: error text has contrast %.1f", who, c)
		}
		if c := contrast(p.dim[side], bgTerm); c < 3 {
			t.Errorf("%s: dim text has contrast %.1f", who, c)
		}
	}
	th := themeFor(true, Settings{HighlightSGR: "48;5;238", Theme: name})
	if th.Hot.Render("x") == themeFor(true, Settings{HighlightSGR: "48;5;238"}).Hot.Render("x") {
		t.Errorf("the %s highlight shows when the setting has its default value", name)
	}
	if !slices.Contains(ThemeNames, name) {
		t.Errorf("%s is not offered", name)
	}
}
