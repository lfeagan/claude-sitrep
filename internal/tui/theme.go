package tui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme holds every style the views use. The plain theme has none, so tests compare text.
type Theme struct {
	Tab, TabActive, Rule, Label, Dim, Err, Status, StatusKey, Hot, Attn, Warn, Snooze lipgloss.Style
	Head                                                                              lipgloss.Style // the labels of a card and the numbers of a queue: they stand out from the text they head
	// the mark down the left edge of a card and the state word of one that needs you: the colour of what needs the user, of what needed them
	// long ago, and of recent activity, each made to read on the terminal's own background
	Need, Fade, Recent, RecentWord lipgloss.Style // RecentWord is the state of a recent card: the colour of its bar, readable as text
	Quiet                          lipgloss.Style // the bracket of a card that is none of those: a grey that shows where the card is and no more
	Dull                           lipgloss.Style // a session that has needed the user for long: the needs-you colour, weakened
	HasDull                        bool
	Plain                          bool
	Mono                           bool // the mono theme: no colours, so marks carry the meaning
}

func plainTheme() Theme {
	s := lipgloss.NewStyle()
	return Theme{Tab: s, TabActive: s, Rule: s, Label: s, Head: s, Need: s, Fade: s, Recent: s, RecentWord: s, Quiet: s, Dim: s, Err: s, Status: s, StatusKey: s, Hot: s, Attn: s, Warn: s, Snooze: s, Plain: true}
}

// palette is the colours a theme is made of, each as a light and a dark value: a hex colour or the number of a terminal colour.
type palette struct{ accent, dim, hot, attn, errc, snooze [2]string }

var presets = map[string]palette{
	"default": {
		accent: [2]string{"#5b3fd1", "#a78bfa"}, dim: [2]string{"#6b6b76", "#8b8b96"}, hot: [2]string{"#e4e4ea", "238"},
		// a session waiting for the user: a pink-red that is lighter than the grey of recent activity on a dark terminal and deeper on a light one
		attn: [2]string{"#ffb8cc", "#96345c"}, errc: [2]string{"#b3261e", "#ff8a80"}, snooze: [2]string{"#2f6fed", "#7aa2f7"},
	},
	// soft lavender, mint and peach, for a quiet screen: the accent is a periwinkle, recent activity a mint, a session that needs you a peach on
	// a light terminal and a dusky rose on a dark one, as a dark peach would be brown
	"pastel": {
		accent: [2]string{"#7565b8", "#b8a9f0"}, dim: [2]string{"#6e6e82", "#9a9aae"}, hot: [2]string{"#d8f3e4", "#1f3a32"},
		attn: [2]string{"#ffc99a", "#5e405a"}, errc: [2]string{"#c2476b", "#ff9eb5"}, snooze: [2]string{"#4f86b0", "#8fbcdb"},
	},
	// one colour of the spectrum for each role: green accent, blue for dim text, orange for recent activity, yellow for what needs you, red for errors
	"rainbow": {
		accent: [2]string{"#2e7d32", "#7ddc8a"}, dim: [2]string{"#4a6fa5", "#8fb0e0"}, hot: [2]string{"#ffe0b8", "#e08a3c"},
		attn: [2]string{"#f5c400", "#ffe566"}, errc: [2]string{"#d32f2f", "#ff6b6b"}, snooze: [2]string{"#1565c0", "#64b5f6"},
	},
	"high-contrast": {
		accent: [2]string{"#0000b3", "#ffd400"}, dim: [2]string{"#3a3a3a", "#c8c8c8"}, hot: [2]string{"#c8c8c8", "#3a3a3a"},
		attn: [2]string{"#ff9090", "#a40000"}, errc: [2]string{"#a00000", "#ff7070"}, snooze: [2]string{"#0033cc", "#6ea8ff"},
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
		accent: [2]string{"#7a5a2e", "#d2b078"}, dim: [2]string{"#7d6e56", "#b0a088"}, hot: [2]string{"#f6f1e5", "#342e20"},
		attn: [2]string{"#f0d49c", "#2e447e"}, errc: [2]string{"#9b2c1f", "#ff8f7a"}, snooze: [2]string{"#5c7a6a", "#9cc0ac"},
	},
	"nord": {
		accent: [2]string{"#4a6f9c", "#88c0d0"}, dim: [2]string{"#4c566a", "#8b95a8"}, hot: [2]string{"#eff1f6", "#292f3b"},
		attn: [2]string{"#e5d6ae", "#67384a"}, errc: [2]string{"#bf616a", "#e08a93"}, snooze: [2]string{"#4a8a89", "#8fbcbb"},
	},
	"solarized": {
		accent: [2]string{"#1f78b8", "#40b0a5"}, dim: [2]string{"#5f757d", "#839496"}, hot: [2]string{"#f5f1e6", "#073642"},
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
	// neon pink and cyan on deep purple, as on an arcade cabinet or a Memphis poster; what needs you is a hot pink that stands out from the purple
	"eighties": {
		accent: [2]string{"#b5127f", "#ff4fd8"}, dim: [2]string{"#1b6f85", "#4de3ff"}, hot: [2]string{"#e9dcff", "#2b1458"},
		attn: [2]string{"#ff8fe0", "#7a1f6a"}, errc: [2]string{"#d0103a", "#ff5c7a"}, snooze: [2]string{"#5a2fc0", "#a995ff"},
	},
	// Kind of Blue: midnight indigo and smoke, with the brass of a trumpet for what needs you
	"blues": {
		accent: [2]string{"#2a3a9c", "#8c9cff"}, dim: [2]string{"#4a5280", "#9096c0"}, hot: [2]string{"#dfe3f7", "#1a1f4a"},
		attn: [2]string{"#e8c04a", "#554700"}, errc: [2]string{"#b0303a", "#ff8f9a"}, snooze: [2]string{"#7a4fa8", "#b79cf0"},
	},
	// green, gold and red on warm black: the gold is what needs you on a light terminal, the red on a dark one, where gold would be brown
	"reggae": {
		accent: [2]string{"#1e7a2e", "#3fd45a"}, dim: [2]string{"#6f6310", "#c9b858"}, hot: [2]string{"#dcf2d4", "#082010"},
		attn: [2]string{"#f2c230", "#8a1f1f"}, errc: [2]string{"#b3261e", "#ff7a6a"}, snooze: [2]string{"#a04a00", "#ffa04d"},
	},
	// burgundy velvet and gold leaf, for the opera house; ivory on a light terminal
	"opera": {
		accent: [2]string{"#8c1c3a", "#e0b050"}, dim: [2]string{"#7a5a62", "#b89aa0"}, hot: [2]string{"#f6ecd8", "#2a0f1c"},
		attn: [2]string{"#e8c060", "#7a1a3c"}, errc: [2]string{"#b3261e", "#ff8a7a"}, snooze: [2]string{"#6a4a8a", "#b9a0e0"},
	},
	// black, chrome and blood red
	"metal": {
		accent: [2]string{"#3a3d42", "#c8ccd2"}, dim: [2]string{"#55595f", "#9aa0a8"}, hot: [2]string{"#e2e4e8", "#26282c"},
		attn: [2]string{"#ff9488", "#8c1010"}, errc: [2]string{"#b00020", "#ff6b6b"}, snooze: [2]string{"#3a6a9a", "#7fb0e0"},
	},
	// a 1960s album sleeve: sea-foam, sand and coral
	"bossa-nova": {
		accent: [2]string{"#1a7a6c", "#7fe0cc"}, dim: [2]string{"#5a7a74", "#9cc4bc"}, hot: [2]string{"#dff5ee", "#17332e"},
		attn: [2]string{"#ffa88a", "#7d2f2a"}, errc: [2]string{"#c23a2a", "#ff8a78"}, snooze: [2]string{"#8a6a2a", "#d8c08a"},
	},
	// a gold chain, purple and concrete
	"hip-hop": {
		accent: [2]string{"#8a6400", "#ffcc33"}, dim: [2]string{"#5e6066", "#a4a6ac"}, hot: [2]string{"#e6e0f2", "#2a2140"},
		attn: [2]string{"#c9a0ff", "#5b2a9a"}, errc: [2]string{"#c0392b", "#ff7b6b"}, snooze: [2]string{"#2a7a9a", "#6cc8e8"},
	},
	// gold and mirror-ball silver on burgundy
	"disco": {
		accent: [2]string{"#9a6400", "#ffd24d"}, dim: [2]string{"#6a5a74", "#c8b0c0"}, hot: [2]string{"#f8e6ee", "#3a1226"},
		attn: [2]string{"#f0b84a", "#7d1f72"}, errc: [2]string{"#c2185b", "#ff6b9a"}, snooze: [2]string{"#3a6ab0", "#8fb4ff"},
	},
	// acid yellow and red on black, with nothing soft about it
	"punk": {
		accent: [2]string{"#7a6d00", "#f2ff2a"}, dim: [2]string{"#5a5a5a", "#a8a8a8"}, hot: [2]string{"#e4e4e4", "#2a2a2a"},
		attn: [2]string{"#ff8080", "#8a0f4a"}, errc: [2]string{"#b34700", "#ff9a3c"}, snooze: [2]string{"#0a7a8a", "#4de3ff"},
	},
	// pink, cyan and lavender in the haze of an old mall; mint (teal on a dark terminal) is what needs you
	"vaporwave": {
		accent: [2]string{"#9a3fc0", "#ff71ce"}, dim: [2]string{"#2f7a8a", "#7fe8f5"}, hot: [2]string{"#e6defa", "#1f1a4a"},
		attn: [2]string{"#5fd4a8", "#0f5055"}, errc: [2]string{"#d6246e", "#ff6b9e"}, snooze: [2]string{"#7a4fd0", "#b967ff"},
	},
	// indigo sky, sand and a coral flare
	"desert-night": {
		accent: [2]string{"#9a5b2a", "#e8c48a"}, dim: [2]string{"#6a5a7a", "#a89ac8"}, hot: [2]string{"#ece6f6", "#10344a"},
		attn: [2]string{"#ff9a7a", "#7a2a3a"}, errc: [2]string{"#b3261e", "#ff8a7a"}, snooze: [2]string{"#3a6a9a", "#7fb0e0"},
	},
	// rain on neon: teal and magenta, with amber for what needs you on a light terminal
	"tokyo": {
		accent: [2]string{"#0a7a8a", "#29e0d0"}, dim: [2]string{"#4a5a78", "#8a9ac0"}, hot: [2]string{"#dde6f2", "#141c2e"},
		attn: [2]string{"#f5b030", "#7a1f5a"}, errc: [2]string{"#d01050", "#ff5c8a"}, snooze: [2]string{"#5a4fc0", "#9a90ff"},
	},
	// ice, snow and a single orange parka
	"arctic": {
		accent: [2]string{"#1a5a8a", "#bfe6ff"}, dim: [2]string{"#4a6a80", "#9ab8cc"}, hot: [2]string{"#e4f1fa", "#1c2b38"},
		attn: [2]string{"#ffb070", "#882a1a"}, errc: [2]string{"#c0392b", "#ff8f80"}, snooze: [2]string{"#3a7a9a", "#7fd0e8"},
	},
	// charcoal and ash, lava orange and ember red
	"volcano": {
		accent: [2]string{"#b83a00", "#ff7a2a"}, dim: [2]string{"#6a5248", "#b89a8c"}, hot: [2]string{"#f2e4de", "#2a1a18"},
		attn: [2]string{"#ff9060", "#8a1a1a"}, errc: [2]string{"#8e1b1b", "#ff6b4a"}, snooze: [2]string{"#6a6a8a", "#9a9ac0"},
	},
	// lagoon teal and palm green, with hibiscus pink for what needs you
	"tropical": {
		accent: [2]string{"#0a7a6a", "#2fe0c0"}, dim: [2]string{"#3a7a5a", "#8ad0a8"}, hot: [2]string{"#d8f5ea", "#0e2f2a"},
		attn: [2]string{"#ff8fb8", "#7a1f4a"}, errc: [2]string{"#c01a3a", "#ff7a8a"}, snooze: [2]string{"#c06a00", "#ffb050"},
	},
	// violet dusk and apricot sky
	"sunset": {
		accent: [2]string{"#b0402a", "#ffa05a"}, dim: [2]string{"#7a5a8a", "#c0a0d0"}, hot: [2]string{"#f4e4f0", "#2a1840"},
		attn: [2]string{"#ffb870", "#861e3e"}, errc: [2]string{"#b3261e", "#ff8a70"}, snooze: [2]string{"#4a5ab0", "#8fa0ff"},
	},
	// bubblegum pink, mint and lemon
	"candy": {
		accent: [2]string{"#d6307a", "#ff7ab8"}, dim: [2]string{"#147a6c", "#7ae8d0"}, hot: [2]string{"#e0f8ee", "#123a30"},
		attn: [2]string{"#ffd23a", "#861e58"}, errc: [2]string{"#d0102a", "#ff6070"}, snooze: [2]string{"#6a3fd0", "#a98fff"},
	},
	// espresso greys and caramel
	"coffee": {
		accent: [2]string{"#8a5a2a", "#d9b080"}, dim: [2]string{"#6e625a", "#b0a498"}, hot: [2]string{"#f0e8e0", "#2c2826"},
		attn: [2]string{"#e8b878", "#70382a"}, errc: [2]string{"#a8281e", "#ff8a78"}, snooze: [2]string{"#5a6a7a", "#98a8b8"},
	},
	// an amber screen from 1982
	"terminal-amber": {
		accent: [2]string{"#8a5a00", "#ffb000"}, dim: [2]string{"#6a5400", "#c89a30"}, hot: [2]string{"#fdf0d0", "#2a1c00"},
		attn: [2]string{"#ffd060", "#544500"}, errc: [2]string{"#b3261e", "#ff7a5a"}, snooze: [2]string{"#3a6a5a", "#78c0a8"},
	},
	// navy ink on newsprint, with red pencil for what needs you
	"paper": {
		accent: [2]string{"#1a2a4a", "#c8d0e8"}, dim: [2]string{"#5a5a5a", "#a8a8a8"}, hot: [2]string{"#ece8dc", "#262d40"},
		attn: [2]string{"#ffa090", "#8a1a1a"}, errc: [2]string{"#a8201a", "#ff7a70"}, snooze: [2]string{"#3a5a8a", "#8aa8d8"},
	},
	// four olive greens, as on a handheld from 1989
	"gameboy": {
		accent: [2]string{"#306230", "#9bbc0f"}, dim: [2]string{"#4a6a30", "#8bac0f"}, hot: [2]string{"#d4e4a0", "#0f380f"},
		attn: [2]string{"#9bbc0f", "#265026"}, errc: [2]string{"#a8281e", "#ff8a70"}, snooze: [2]string{"#5a7a6a", "#a0c8b0"},
	},
	// blue and orange only, apart in lightness, for red-green colour blindness
	"cvd-safe": {
		accent: [2]string{"#0a4f9a", "#7ab8ff"}, dim: [2]string{"#4a5a6a", "#a0acba"}, hot: [2]string{"#e0ecfa", "#14243a"},
		attn: [2]string{"#ffb060", "#833000"}, errc: [2]string{"#b34700", "#ff9a4a"}, snooze: [2]string{"#a8507f", "#e8a0c8"},
	},
	"fall": {
		accent: [2]string{"#9a4a1a", "#e0a060"}, dim: [2]string{"#86705c", "#b8a28c"}, hot: [2]string{"#f6ead6", "#36301c"},
		attn: [2]string{"#f0a070", "#7a2e1c"}, errc: [2]string{"#a52a1a", "#ff8a70"}, snooze: [2]string{"#5f7a3f", "#b0c488"},
	},
}

// solidThemes have backgrounds that are light on a dark terminal too, so their text is chosen to read on them.
var solidThemes = map[string]bool{"rainbow": true}

// ThemeNames are the themes SITREP_THEME may name. "mono" uses no colours, only bold, faint and reverse.
var ThemeNames = []string{"default", "pastel", "rainbow", "high-contrast", "ocean", "forest", "homebrew", "red-sands", "novel", "nord", "solarized", "winter", "spring", "summer", "fall", "eighties", "blues", "reggae", "opera", "metal", "bossa-nova", "hip-hop", "disco", "punk", "vaporwave", "desert-night", "tokyo", "arctic", "volcano", "tropical", "sunset", "candy", "coffee", "terminal-amber", "paper", "gameboy", "cvd-safe", "mono"}

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
		Tab:        lipgloss.NewStyle().Foreground(dim).Padding(0, 1),
		TabActive:  lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true).Padding(0, 1),
		Rule:       lipgloss.NewStyle().Foreground(accent),
		Label:      lipgloss.NewStyle().Foreground(dim),
		Need:       lipgloss.NewStyle().Foreground(lipgloss.Color(readableOn(p.attn[side], isDark, 4.5))),
		Fade:       lipgloss.NewStyle().Foreground(lipgloss.Color(fadedBar(p.attn[side], isDark))),
		Recent:     lipgloss.NewStyle().Foreground(lipgloss.Color(readableOn(p.hot[side], isDark, 3))),
		Quiet:      lipgloss.NewStyle().Foreground(lipgloss.Color(quietBar(isDark))),
		RecentWord: lipgloss.NewStyle().Foreground(lipgloss.Color(readableOn(p.hot[side], isDark, 4.5))),
		Head:       lipgloss.NewStyle().Foreground(ld(lipgloss.Color(p.accent[0]), lipgloss.Color(headOnDark(p.accent[1], custom["accent"])))).Bold(true),
		Dim:        lipgloss.NewStyle().Foreground(dim),
		Err:        lipgloss.NewStyle().Background(pick(p.errc)).Foreground(lipgloss.Color(onColor(p.errc[side]))).Bold(true),
		Status:     lipgloss.NewStyle().Foreground(on).Background(accent),
		StatusKey:  lipgloss.NewStyle().Foreground(on).Background(accent).Bold(true),
		Hot:        withText(lipgloss.NewStyle().Background(pick(p.hot)), custom["hot"], p.hot[side]),
		Attn:       withText(lipgloss.NewStyle().Background(pick(p.attn)), custom["attn"], p.attn[side]),
		Dull:       withText(lipgloss.NewStyle().Background(lipgloss.Color(blendHex(p.attn[side], []string{"#ffffff", "#1e1e1e"}[side], dullShare))), custom["attn"], p.attn[side]),
		HasDull:    true,
		Warn:       lipgloss.NewStyle().Bold(true),
		Snooze:     lipgloss.NewStyle().Foreground(pick(p.snooze)),
	}
}

// maxHeadContrast is how bright the labels of a card may be against a dark terminal. A theme's accent colours the bars and the tabs, where a
// bright one is wanted, but a label in it outshouts the text it heads.
const maxHeadContrast = 11.0

// headOnDark is the accent for a label on a dark terminal: the accent itself, or a darker one when it would be brighter than
// maxHeadContrast against the terminal's #1e1e1e. A colour the user chose is kept as it is.
func headOnDark(accent string, chosen bool) string {
	bg := "#1e1e1e"
	if chosen || contrastRatio(accent, bg) <= maxHeadContrast {
		return accent
	}
	for share := 0.02; share < 1; share += 0.02 {
		if c := blendHex(accent, bg, share); contrastRatio(c, bg) <= maxHeadContrast {
			return c
		}
	}
	return accent
}

// contrastRatio is the WCAG contrast of two colours, each #rrggbb or a terminal colour number.
func contrastRatio(a, b string) float64 {
	lum := func(c string) float64 {
		r, g, bl := rgbOf(c)
		lin := func(v int) float64 {
			x := float64(v) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(bl)
	}
	hi, lo := lum(a), lum(b)
	if lo > hi {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05)
}

// readableOn is colour c, or the nearest lighter one on a dark terminal (darker on a light one) that has the contrast target against
// the terminal's own background, taken as #1e1e1e or #ffffff. The hue stays: it is the colour of a background, made into one for text.
func readableOn(c string, dark bool, target float64) string {
	bg, toward := "#ffffff", "#000000"
	if dark {
		bg, toward = "#1e1e1e", "#ffffff"
	}
	if contrastRatio(c, bg) >= target {
		return c
	}
	for share := 0.04; share < 1; share += 0.04 {
		if h := blendHex(c, toward, share); contrastRatio(h, bg) >= target {
			return h
		}
	}
	return toward
}

// fadedBar is the bar of a card that needed the user long ago: the colour of one that needs them now, mixed with the terminal's own
// background, and not below 3:1 against it. A colour that is bright already (rainbow's yellow) is weakened too, which a floor alone would not do.
func fadedBar(attn string, dark bool) string {
	bg := "#ffffff"
	if dark {
		bg = "#1e1e1e"
	}
	return readableOn(blendHex(readableOn(attn, dark, 4.5), bg, fadeShare), dark, 3)
}

// quietBar is the grey of a bracket that says nothing about the session: dimmer than the bar of recent activity, and not below 2:1 against the
// terminal's own background.
func quietBar(dark bool) string {
	if dark {
		return "#5a5a5a"
	}
	return "#b4b4b4"
}

// fadeShare is how much of the terminal's background the bar of a faded card is mixed with.
const fadeShare = 0.45

// dullShare is how much of the terminal's own background a needs-you colour is mixed with once its session has waited long: the row stays
// tinted, so that the session that has waited longest is not the quietest, but it is clearly weaker than a new one.
const dullShare = 0.55

// blendHex mixes colour a with colour b, share of b, and returns #rrggbb.
func blendHex(a, b string, share float64) string {
	ar, ag, ab := rgbOf(a)
	br, bg, bb := rgbOf(b)
	mix := func(x, y int) int { return int(math.Round(float64(x)*(1-share) + float64(y)*share)) }
	return fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb))
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
		Mono: true, Need: s.Bold(true),
		Tab: s.Faint(true).Padding(0, 1), TabActive: s.Bold(true).Underline(true).Padding(0, 1), Rule: s, Label: s.Faint(true), Head: s.Bold(true), Dim: s.Faint(true),
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
