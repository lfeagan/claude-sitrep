package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// Theme holds every style the views use. The plain theme has none, so tests compare text.
type Theme struct {
	Tab, TabActive, Rule, Label, Dim, Err, Status, StatusKey, Hot, Attn lipgloss.Style
	Plain                                                               bool
}

func plainTheme() Theme {
	s := lipgloss.NewStyle()
	return Theme{Tab: s, TabActive: s, Rule: s, Label: s, Dim: s, Err: s, Status: s, StatusKey: s, Hot: s, Attn: s, Plain: true}
}

// newTheme picks light or dark colours. sgr is the highlight setting from sitrep, as
// terminal SGR codes; one that cannot be read falls back to a built-in colour.
func newTheme(isDark bool, sgr string) Theme {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#5b3fd1"), lipgloss.Color("#a78bfa"))
	dim := ld(lipgloss.Color("#6b6b76"), lipgloss.Color("#8b8b96"))
	hot, ok := bgFromSGR(sgr)
	if !ok {
		hot = ld(lipgloss.Color("#e4e0f7"), lipgloss.Color("#2f2b45"))
	}
	return Theme{
		Tab:       lipgloss.NewStyle().Foreground(dim).Padding(0, 1),
		TabActive: lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true).Padding(0, 1),
		Rule:      lipgloss.NewStyle().Foreground(accent),
		Label:     lipgloss.NewStyle().Foreground(dim),
		Dim:       lipgloss.NewStyle().Foreground(dim),
		Err:       lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#b3261e"), lipgloss.Color("#ff8a80"))).Bold(true),
		Status:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#ffffff"), lipgloss.Color("#101014"))).Background(accent),
		StatusKey: lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#ffffff"), lipgloss.Color("#101014"))).Background(accent).Bold(true),
		Hot:       lipgloss.NewStyle().Background(hot),
		// a session waiting for the user: a soft pink-red that stands out from the purple of recent activity
		Attn: lipgloss.NewStyle().Background(ld(lipgloss.Color("#ffd9e4"), lipgloss.Color("#5c1f35"))),
	}
}

// bgFromSGR reads a background colour from SGR codes such as "48;5;238", "48;2;r;g;b" or "44".
func bgFromSGR(sgr string) (color.Color, bool) {
	parts := strings.Split(sgr, ";")
	n := make([]int, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || v > 255 {
			return nil, false
		}
		n = append(n, v)
	}
	switch {
	case len(n) == 3 && n[0] == 48 && n[1] == 5:
		return lipgloss.Color(strconv.Itoa(n[2])), true
	case len(n) == 5 && n[0] == 48 && n[1] == 2:
		return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", n[2], n[3], n[4])), true
	case len(n) == 1 && n[0] >= 40 && n[0] <= 47:
		return lipgloss.Color(strconv.Itoa(n[0] - 40)), true
	case len(n) == 1 && n[0] >= 100 && n[0] <= 107:
		return lipgloss.Color(strconv.Itoa(n[0] - 100 + 8)), true
	}
	return nil, false
}
