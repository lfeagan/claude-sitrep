package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme holds every style the views use. The plain theme has none, so tests compare text.
type Theme struct {
	Tab, TabActive, Rule, Label, Dim, Err, Status, StatusKey, Hot, Attn, Warn, Snooze lipgloss.Style
	Plain                                                                             bool
}

func plainTheme() Theme {
	s := lipgloss.NewStyle()
	return Theme{Tab: s, TabActive: s, Rule: s, Label: s, Dim: s, Err: s, Status: s, StatusKey: s, Hot: s, Attn: s, Warn: s, Snooze: s, Plain: true}
}

// palette is the colours a theme is made of, each as a light and a dark value: a hex colour or the number of a terminal colour.
type palette struct{ accent, dim, hot, attn, errc, snooze [2]string }

var presets = map[string]palette{
	"default": {
		accent: [2]string{"#5b3fd1", "#a78bfa"}, dim: [2]string{"#6b6b76", "#8b8b96"}, hot: [2]string{"238", "238"},
		// a session waiting for the user: a soft pink-red that stands out from the grey of recent activity
		attn: [2]string{"#ffd9e4", "#5c1f35"}, errc: [2]string{"#b3261e", "#ff8a80"}, snooze: [2]string{"#2f6fed", "#7aa2f7"},
	},
	// soft lavender, mint and peach, for a quiet screen: the accent is a periwinkle, recent activity a mint, a session that needs you a peach on
	// a light terminal and a dusky rose on a dark one, as a dark peach would be brown
	"pastel": {
		accent: [2]string{"#7565b8", "#b8a9f0"}, dim: [2]string{"#7a7a8c", "#9a9aae"}, hot: [2]string{"#d8f3e4", "#24433a"},
		attn: [2]string{"#ffe0c2", "#5b3a58"}, errc: [2]string{"#c2476b", "#ff9eb5"}, snooze: [2]string{"#4f86b0", "#8fbcdb"},
	},
	// one colour of the spectrum for each role: green accent, blue for dim text, orange for recent activity, yellow for what needs you, red for errors
	"rainbow": {
		accent: [2]string{"#2e7d32", "#7ddc8a"}, dim: [2]string{"#4a6fa5", "#8fb0e0"}, hot: [2]string{"#ffe0b8", "#ffb870"},
		attn: [2]string{"#fff2a8", "#ffe566"}, errc: [2]string{"#d32f2f", "#ff6b6b"}, snooze: [2]string{"#1565c0", "#64b5f6"},
	},
	"high-contrast": {
		accent: [2]string{"#0000b3", "#ffd400"}, dim: [2]string{"#3a3a3a", "#c8c8c8"}, hot: [2]string{"#c8c8c8", "#505050"},
		attn: [2]string{"#ff9a9a", "#a40000"}, errc: [2]string{"#a00000", "#ff7070"}, snooze: [2]string{"#0033cc", "#6ea8ff"},
	},
	// the themes below are moods, not copies of any terminal's colours: sitrep draws bars, rows and marks, and the terminal keeps its own background
	"ocean": {
		accent: [2]string{"#1f5fa8", "#6cb0f0"}, dim: [2]string{"#5b6f85", "#8aa0b8"}, hot: [2]string{"#e8f2fc", "#1d3550"},
		attn: [2]string{"#f0d3ad", "#494900"}, errc: [2]string{"#c0392b", "#ff8f80"}, snooze: [2]string{"#2b7a9e", "#7fc8e8"},
	},
	// earth: moss, bark and olive, with amber for what needs you
	"forest": {
		accent: [2]string{"#4a6b2a", "#a9c97a"}, dim: [2]string{"#6e7355", "#a3a888"}, hot: [2]string{"#eff3e4", "#2a3320"},
		attn: [2]string{"#f0d499", "#7b2b39"}, errc: [2]string{"#b3261e", "#ff8a80"}, snooze: [2]string{"#8a6a3a", "#d0b078"},
	},
	// a green phosphor screen with amber for what needs you: neon on black, nothing earthy about it
	"homebrew": {
		accent: [2]string{"#0f6f0f", "#39ff6b"}, dim: [2]string{"#3d5f3d", "#4a9a5a"}, hot: [2]string{"#e1f7e1", "#0f3614"},
		attn: [2]string{"#e4d98e", "#494900"}, errc: [2]string{"#b00020", "#ff5555"}, snooze: [2]string{"#1a7f7f", "#55dddd"},
	},
	"red-sands": {
		accent: [2]string{"#a5472f", "#e8967a"}, dim: [2]string{"#8a6f62", "#b59a8c"}, hot: [2]string{"#faefe9", "#4a2a22"},
		attn: [2]string{"#bedddf", "#0f4a52"}, errc: [2]string{"#8e1b1b", "#ff7b6b"}, snooze: [2]string{"#7a6a8f", "#b7a6d0"},
	},
	"novel": {
		accent: [2]string{"#7a5a2e", "#d2b078"}, dim: [2]string{"#8a7a62", "#b0a088"}, hot: [2]string{"#f6f1e5", "#342e20"},
		attn: [2]string{"#f0d49c", "#2e447e"}, errc: [2]string{"#9b2c1f", "#ff8f7a"}, snooze: [2]string{"#5c7a6a", "#9cc0ac"},
	},
	"nord": {
		accent: [2]string{"#4a6f9c", "#88c0d0"}, dim: [2]string{"#4c566a", "#8b95a8"}, hot: [2]string{"#eff1f6", "#292f3b"},
		attn: [2]string{"#e5d6ae", "#67384a"}, errc: [2]string{"#bf616a", "#e08a93"}, snooze: [2]string{"#4a8a89", "#8fbcbb"},
	},
	"solarized": {
		accent: [2]string{"#1f78b8", "#40b0a5"}, dim: [2]string{"#657b83", "#839496"}, hot: [2]string{"#f5f1e6", "#073642"},
		attn: [2]string{"#e8d79a", "#702f59"}, errc: [2]string{"#dc322f", "#ff6b66"}, snooze: [2]string{"#6c71c4", "#9aa0f0"},
	},
	// the seasons; the dark side of each keeps what needs you clearly lighter than recent activity, so that red-green and blue-yellow colour
	// blindness, which take away the hue, leave the difference
	"winter": {
		accent: [2]string{"#2f5f94", "#a9d2f5"}, dim: [2]string{"#5d7389", "#93a9bf"}, hot: [2]string{"#e8f2fb", "#16283a"},
		attn: [2]string{"#c9bdf0", "#4a3a85"}, errc: [2]string{"#b8323f", "#ff9aa0"}, snooze: [2]string{"#4f8fb5", "#a8d8ee"},
	},
	"spring": {
		accent: [2]string{"#2f7d3e", "#a6e39a"}, dim: [2]string{"#5f7a63", "#95b09a"}, hot: [2]string{"#eaf6e2", "#1f3322"},
		attn: [2]string{"#ffb8dc", "#6a2a58"}, errc: [2]string{"#c2403d", "#ff938c"}, snooze: [2]string{"#5a6fb5", "#b3c3f5"},
	},
	"summer": {
		accent: [2]string{"#b4520a", "#ffc15a"}, dim: [2]string{"#7a6a50", "#b5a589"}, hot: [2]string{"#fff6d6", "#38341c"},
		attn: [2]string{"#ffb59a", "#7e2544"}, errc: [2]string{"#b3261e", "#ff8a7a"}, snooze: [2]string{"#1f7f9f", "#6fcbe8"},
	},
	"fall": {
		accent: [2]string{"#9a4a1a", "#e0a060"}, dim: [2]string{"#86705c", "#b8a28c"}, hot: [2]string{"#f6ead6", "#36301c"},
		attn: [2]string{"#f0a070", "#7a2e1c"}, errc: [2]string{"#a52a1a", "#ff8a70"}, snooze: [2]string{"#5f7a3f", "#b0c488"},
	},
}

// solidThemes have backgrounds that are light on a dark terminal too, so their text is chosen to read on them.
var solidThemes = map[string]bool{"rainbow": true}

// ThemeNames are the themes SITREP_THEME may name. "mono" uses no colours, only bold, faint and reverse.
var ThemeNames = []string{"default", "pastel", "rainbow", "high-contrast", "ocean", "forest", "homebrew", "red-sands", "novel", "nord", "solarized", "winter", "spring", "summer", "fall", "mono"}

// colorNames are the entries of SITREP_COLORS.
var colorNames = []string{"accent", "dim", "hot", "attn", "error", "snoozed"}

// newTheme picks light or dark colours of the default theme. sgr is the highlight setting from sitrep, as terminal SGR codes; one that cannot
// be read falls back to the theme's own.
func newTheme(isDark bool, sgr string) Theme {
	return themeFor(isDark, Settings{HighlightSGR: sgr})
}

// themeFor builds the theme the settings ask for. Later wins: the theme, then SITREP_HIGHLIGHT_SGR for the highlight (unless it has its
// default value, which is the default theme's own highlight), then SITREP_COLORS. A name or colour that is not understood is ignored.
func themeFor(isDark bool, st Settings) Theme {
	if st.Theme == "mono" {
		return monoTheme()
	}
	p, ok := presets[st.Theme]
	if !ok {
		p = presets["default"]
	}
	if hot, ok := sgrString(st.HighlightSGR); ok && st.HighlightSGR != "48;5;238" {
		p.hot = [2]string{hot, hot}
	}
	custom := map[string]bool{} // a background the user chose gets text that reads on it; the built-in ones keep the terminal's own
	if solidThemes[st.Theme] {
		custom["hot"], custom["attn"] = true, true // bright backgrounds on any terminal: a dark orange or yellow would be brown
	}
	for name, v := range parseColors(st.Colors) {
		custom[name] = true
		switch name {
		case "accent":
			p.accent = v
		case "dim":
			p.dim = v
		case "hot":
			p.hot = v
		case "attn":
			p.attn = v
		case "error":
			p.errc = v
		case "snoozed":
			p.snooze = v
		}
	}
	ld := lipgloss.LightDark(isDark)
	pick := func(v [2]string) color.Color { return ld(lipgloss.Color(v[0]), lipgloss.Color(v[1])) }
	accent, dim := pick(p.accent), pick(p.dim)
	side := 0
	if isDark {
		side = 1
	}
	on := lipgloss.Color(onColor(p.accent[side]))
	return Theme{
		Tab:       lipgloss.NewStyle().Foreground(dim).Padding(0, 1),
		TabActive: lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true).Padding(0, 1),
		Rule:      lipgloss.NewStyle().Foreground(accent),
		Label:     lipgloss.NewStyle().Foreground(dim),
		Dim:       lipgloss.NewStyle().Foreground(dim),
		Err:       lipgloss.NewStyle().Background(pick(p.errc)).Foreground(lipgloss.Color(onColor(p.errc[side]))).Bold(true),
		Status:    lipgloss.NewStyle().Foreground(on).Background(accent),
		StatusKey: lipgloss.NewStyle().Foreground(on).Background(accent).Bold(true),
		Hot:       withText(lipgloss.NewStyle().Background(pick(p.hot)), custom["hot"], p.hot[side]),
		Attn:      withText(lipgloss.NewStyle().Background(pick(p.attn)), custom["attn"], p.attn[side]),
		Warn:      lipgloss.NewStyle().Bold(true),
		Snooze:    lipgloss.NewStyle().Foreground(pick(p.snooze)),
	}
}

// withText sets the text colour that reads on bg, when the background is one the user chose.
func withText(st lipgloss.Style, custom bool, bg string) lipgloss.Style {
	if !custom {
		return st
	}
	return st.Foreground(lipgloss.Color(onColor(bg)))
}

// monoTheme uses no colour: bold for activity, reverse video for what needs the user and for the bars.
func monoTheme() Theme {
	s := lipgloss.NewStyle()
	return Theme{
		Tab: s.Faint(true).Padding(0, 1), TabActive: s.Bold(true).Underline(true).Padding(0, 1), Rule: s, Label: s.Faint(true), Dim: s.Faint(true),
		Err: s.Bold(true), Status: s.Reverse(true), StatusKey: s.Reverse(true).Bold(true), Hot: s.Bold(true), Attn: s.Reverse(true), Warn: s.Bold(true),
	}
}

// parseColors reads SITREP_COLORS: comma-separated name=colour entries, where a colour is #rrggbb or a number from 0 to 255, or two of them
// as light/dark. An entry that is not of that form is dropped.
func parseColors(s string) map[string][2]string {
	out := map[string][2]string{}
	for _, e := range strings.Split(s, ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(e), "=")
		name, val = strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(val)
		if !ok || !slices.Contains(colorNames, name) {
			continue
		}
		l, d, two := strings.Cut(val, "/")
		if !two {
			d = l
		}
		l, d = normColor(l), normColor(d)
		if l != "" && d != "" {
			out[name] = [2]string{l, d}
		}
	}
	return out
}

// normColor returns a colour as lipgloss reads it ("#rrggbb" in lower case, or the number of a terminal colour), or "" for anything else.
func normColor(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) == 7 && s[0] == '#' {
		for _, r := range s[1:] {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				return ""
			}
		}
		return s
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 && strconv.Itoa(n) == s {
		return s
	}
	return ""
}

// onColor is the text colour that reads on a background: dark on a light one, light on a dark one.
func onColor(bg string) string {
	r, g, b := rgbOf(bg)
	if (299*r+587*g+114*b)/1000 > 140 {
		return "#101014"
	}
	return "#ffffff"
}

// rgbOf is the red, green and blue (0 to 255) of #rrggbb or of a terminal colour number, in the usual xterm palette.
func rgbOf(c string) (r, g, b int) {
	if len(c) == 7 && c[0] == '#' {
		v, _ := strconv.ParseUint(c[1:], 16, 32)
		return int(v >> 16 & 255), int(v >> 8 & 255), int(v & 255)
	}
	n, _ := strconv.Atoi(c)
	switch {
	case n < 8:
		return int(n&1) * 170, int(n>>1&1) * 170, int(n>>2&1) * 170
	case n < 16:
		return int(n&1)*170 + 85, int(n>>1&1)*170 + 85, int(n>>2&1)*170 + 85
	case n < 232:
		n -= 16
		lv := func(i int) int { return []int{0, 95, 135, 175, 215, 255}[i] }
		return lv(n / 36), lv(n / 6 % 6), lv(n % 6)
	}
	v := 8 + 10*(n-232)
	return v, v, v
}

// bgFromSGR reads a background colour from SGR codes such as "48;5;238", "48;2;r;g;b" or "44".
func bgFromSGR(sgr string) (color.Color, bool) {
	c, ok := sgrString(sgr)
	if !ok {
		return nil, false
	}
	return lipgloss.Color(c), true
}

// sgrString is the colour of bgFromSGR as a palette holds it: "#rrggbb", or the number of a terminal colour.
func sgrString(sgr string) (string, bool) {
	parts := strings.Split(sgr, ";")
	n := make([]int, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || v > 255 {
			return "", false
		}
		n = append(n, v)
	}
	switch {
	case len(n) == 3 && n[0] == 48 && n[1] == 5:
		return strconv.Itoa(n[2]), true
	case len(n) == 5 && n[0] == 48 && n[1] == 2:
		return fmt.Sprintf("#%02x%02x%02x", n[2], n[3], n[4]), true
	case len(n) == 1 && n[0] >= 40 && n[0] <= 47:
		return strconv.Itoa(n[0] - 40), true
	case len(n) == 1 && n[0] >= 100 && n[0] <= 107:
		return strconv.Itoa(n[0] - 100 + 8), true
	}
	return "", false
}
