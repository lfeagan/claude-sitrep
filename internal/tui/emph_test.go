package tui

import (
	"charm.land/lipgloss/v2"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func styledOpts(theme Theme) viewOpts { return viewOpts{Width: 80, Height: 60, Theme: theme} }

func TestLabelsAndNamesStandOut(t *testing.T) {
	s := loadFixture(t)
	frame := render(s, tabCards, "", styledOpts(newTheme(true, "48;5;238")))
	var heads, labels int
	for _, l := range strings.Split(frame, "\n") {
		plain := ansi.Strip(l)
		switch {
		case strings.Contains(plain, "Objective:") || strings.Contains(plain, "Progress:") || strings.Contains(plain, "Now:") || strings.Contains(plain, "Queue:"):
			labels++ // under a header: the label is bold and in the accent colour
			if strings.Contains(l, "48;") || !strings.Contains(l, "\x1b[22;39m") {
				t.Errorf("a label is bold and in the accent colour: %q", l)
			}
		case strings.Contains(plain, " waiting ") || strings.Contains(plain, " working "):
			heads++ // a header: the name is bold
			if !strings.Contains(l, "\x1b[1m") || !strings.Contains(l, "\x1b[22m") {
				t.Errorf("the name is bold: %q", l)
			}
		}
	}
	if heads == 0 || labels == 0 {
		t.Errorf("headers %d, labels %d", heads, labels)
	}
}

func TestEmphasisAddsNoText(t *testing.T) {
	s := loadFixture(t)
	for name, th := range map[string]Theme{"default": newTheme(true, "48;5;238"), "light": newTheme(false, "48;5;238")} {
		o := styledOpts(th)
		o.NoColour = true // with colour the bars are one glyph, with none a recent card has a · instead
		got := ansi.Strip(render(s, tabCards, "", o))
		want := ansi.Strip(render(s, tabCards, "", styledOpts(plainTheme())))
		trim := func(x string) string {
			lines := strings.Split(x, "\n")
			for i := range lines {
				lines[i] = strings.TrimRight(lines[i], " ")
			}
			return strings.Join(lines[1:], "\n") // the tab line differs
		}
		if trim(got) != trim(want) {
			t.Errorf("%s: emphasis changed the text", name)
		}
	}
}

func TestAPlainThemeHasNoCodesAndMonoNoColours(t *testing.T) {
	s := loadFixture(t)
	if strings.ContainsRune(render(s, tabCards, "", styledOpts(plainTheme())), 0x1b) {
		t.Error("the plain theme has no escape codes")
	}
	frame := render(s, tabCards, "", styledOpts(monoTheme()))
	if strings.Contains(frame, "38;") || strings.Contains(frame, "48;") {
		t.Errorf("the mono theme has colours:\n%q", frame)
	}
	if !strings.Contains(frame, "\x1b[1m┃") || !strings.Contains(ansi.Strip(frame), " ·") {
		t.Errorf("mono marks a card that needs you with a bold bar and a recent one with a ·:\n%s", ansi.Strip(frame))
	}
}

func TestSKeyCyclesTheCardsSort(t *testing.T) {
	m := loaded(t, tabCards)
	if cardsSort(m.cardsBy) != sortState || strings.Contains(m.View().Content, "sort:") {
		t.Fatal("the Cards tab starts on the state sort, and does not say so")
	}
	seen := map[int]bool{}
	for i := 1; i <= sortCount; i++ {
		m, _ = press(t, m, "s")
		seen[cardsSort(m.cardsBy)] = true
		if m.sortBy != sortActivity {
			t.Fatal("s on Cards must leave the Table's sort alone")
		}
		if i == 1 && !strings.Contains(ansi.Strip(m.View().Content), "sort: activity") {
			t.Errorf("the sort is not shown:\n%s", ansi.Strip(m.View().Content))
		}
	}
	if len(seen) != sortCount || cardsSort(m.cardsBy) != sortState {
		t.Errorf("four presses go through every sort and come back: %v", seen)
	}
	s := loadFixture(t)
	activity := visibleIDsFor(s, tabCards, false, 0, 1, "")
	var order []string
	for _, x := range s.Sessions {
		if x.Running {
			order = append(order, x.ID)
		}
	}
	if strings.Join(activity, " ") != strings.Join(order, " ") {
		t.Errorf("the activity sort keeps the order sitrep sent: %v, not %v", activity, order)
	}
	if st := visibleIDsFor(s, tabCards, false, 0, 0, ""); strings.Join(st, " ") == strings.Join(activity, " ") {
		t.Skip("the fixture's state order is its activity order")
	}
}

func TestSelectionFollowsItsSessionWhenTheCardsAreResorted(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "j")
	id := m.selID
	m, _ = press(t, m, "s")
	if m.selID != id {
		t.Errorf("the selection moved from %q to %q", id, m.selID)
	}
}

func TestAWrappedQueueItemIsSetApart(t *testing.T) {
	long := strings.Repeat("word ", 12)
	v := queueView{Name: "Inbox", Items: []string{"short", long, "next", long}}
	out := ansi.Strip(queueScreen(v, 40, 20, plainTheme()))
	lines := strings.Split(out, "\n")
	var texts []string
	for _, l := range lines[:8] {
		texts = append(texts, strings.TrimRight(l, " "))
	}
	// short and next follow with no gap, a wrapped item is followed by an empty line
	if !strings.HasPrefix(texts[1], "▸ 1. short") || !strings.HasPrefix(texts[2], "  2. word") {
		t.Fatalf("lines: %q", texts)
	}
	blank := 0
	for i, l := range texts {
		if l == "" && i > 0 && i < len(texts)-1 {
			blank++
		}
	}
	if blank < 1 {
		t.Errorf("no gap after the wrapped item: %q", texts)
	}
	for i := 1; i < len(texts)-1; i++ {
		if strings.HasPrefix(strings.TrimSpace(texts[i]), "1. short") && texts[i+1] == "" {
			t.Errorf("a one-line item has a gap after it: %q", texts)
		}
	}
}

func TestLabelsAreNeverLouderThanTheirText(t *testing.T) {
	hex := func(th Theme) string {
		r, g, b, _ := th.Head.GetForeground().RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	for _, name := range ThemeNames {
		if name == "mono" {
			continue
		}
		th := themeFor(true, Settings{Theme: name, HighlightSGR: "48;5;238"})
		c := contrastRatio(hex(th), "#1e1e1e")
		if c > 11.3 || c < 4.5 {
			t.Errorf("%s: the label colour %s has contrast %.1f on a dark terminal", name, hex(th), c)
		}
		if p := presets[name]; strings.HasPrefix(p.accent[1], "#") && contrastRatio(p.accent[1], "#1e1e1e") <= maxHeadContrast && hex(th) != strings.ToLower(p.accent[1]) {
			t.Errorf("%s: a label colour within the limit is kept, not %s", name, hex(th))
		}
	}
	if th := themeFor(true, Settings{Theme: "rainbow", HighlightSGR: "48;5;238"}); hex(th) != strings.ToLower(presets["rainbow"].accent[1]) {
		t.Errorf("rainbow's labels keep its accent: %s", hex(th))
	}
	if th := themeFor(true, Settings{Theme: "punk", Colors: "accent=#ffff00/#ffff00", HighlightSGR: "48;5;238"}); hex(th) != "#ffff00" {
		t.Errorf("a colour the user chose is kept: %s", hex(th))
	}
}

func TestTheBarAndTheStateReadOnTheTerminalsBackground(t *testing.T) {
	hex := func(st lipgloss.Style) string {
		r, g, b, _ := st.GetForeground().RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	for _, name := range ThemeNames {
		if name == "mono" {
			continue
		}
		for _, dark := range []bool{true, false} {
			bg := "#ffffff"
			if dark {
				bg = "#1e1e1e"
			}
			th := themeFor(dark, Settings{Theme: name, HighlightSGR: "48;5;238"})
			for what, c := range map[string]struct {
				st  lipgloss.Style
				min float64
			}{"needs you": {th.Need, 4.5}, "faded": {th.Fade, 3}, "recent": {th.Recent, 3}, "recent state": {th.RecentWord, 4.5}} {
				if got := contrastRatio(hex(c.st), bg); got < c.min-0.05 {
					t.Errorf("%s, dark=%v: the %s colour %s has contrast %.1f on %s, want %.1f", name, dark, what, hex(c.st), got, bg, c.min)
				}
			}
		}
	}
	if th := themeFor(true, Settings{Theme: "rainbow", HighlightSGR: "48;5;238"}); hex(th.Need) != "#ffe566" {
		t.Errorf("rainbow's yellow is kept as it is: %s", hex(th.Need))
	}
}

func TestTheStateOfARecentCardIsInItsColour(t *testing.T) {
	s := loadFixture(t)
	th := newTheme(true, "48;5;238")
	found := false
	for _, l := range strings.Split(render(s, tabCards, "", styledOpts(th)), "\n") {
		if strings.Contains(ansi.Strip(l), "working 1m") {
			found = true
			if !strings.Contains(l, sgrOf(th.RecentWord)+"working 1m") {
				t.Errorf("the state of a recent card is not in its colour: %q", l)
			}
		}
	}
	if !found {
		t.Fatal("no recent card on screen")
	}
}

func TestAFadedBarIsWeakerThanAFreshOneInEveryTheme(t *testing.T) {
	hex := func(st lipgloss.Style) string {
		r, g, b, _ := st.GetForeground().RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	for _, name := range ThemeNames {
		if name == "mono" {
			continue
		}
		for _, dark := range []bool{true, false} {
			bg := "#ffffff"
			if dark {
				bg = "#1e1e1e"
			}
			th := themeFor(dark, Settings{Theme: name, HighlightSGR: "48;5;238"})
			if need, fade := contrastRatio(hex(th.Need), bg), contrastRatio(hex(th.Fade), bg); fade >= need || fade < 2.95 {
				t.Errorf("%s, dark=%v: the faded bar (%s, %.1f:1) is not weaker than the fresh one (%s, %.1f:1), or it is below 3:1", name, dark, hex(th.Fade), fade, hex(th.Need), need)
			}
		}
	}
}

func TestACardIsAHeavyBracket(t *testing.T) {
	s := loadFixture(t)
	lines := strings.Split(render(s, tabCards, "", styledOpts(newTheme(true, "48;5;238"))), "\n")
	var tops, bottoms int
	open := false
	for _, l := range lines {
		cols := []rune(ansi.Strip(l))
		if len(cols) < 4 {
			continue
		}
		switch cols[0] {
		case '┏':
			if open {
				t.Errorf("a card starts before the one above it ends: %q", string(cols))
			}
			tops, open = tops+1, true
			if cols[3] == ' ' {
				t.Errorf("a top corner on a line that is not a header: %q", string(cols))
			}
		case '┃':
			if !open {
				t.Errorf("a bar outside a card: %q", string(cols))
			}
		case '┗':
			if !open {
				t.Errorf("a bottom corner with no top: %q", string(cols))
			}
			bottoms, open = bottoms+1, false
		case '▌', '▖', '▘':
			t.Errorf("a block character where a bracket belongs: %q", string(cols))
		}
	}
	if tops == 0 || tops != bottoms {
		t.Errorf("each marked card has one top corner and one bottom corner: %d and %d", tops, bottoms)
	}
}

func TestTheThreeBarsAreToldApartInEveryTheme(t *testing.T) {
	hex := func(st lipgloss.Style) string {
		r, g, b, _ := st.GetForeground().RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	for _, name := range ThemeNames {
		if name == "mono" {
			continue
		}
		for _, dark := range []bool{true, false} {
			bg := "#ffffff"
			if dark {
				bg = "#1e1e1e"
			}
			th := themeFor(dark, Settings{Theme: name, HighlightSGR: "48;5;238"})
			need, fade, rec := hex(th.Need), hex(th.Fade), hex(th.Recent)
			if need == fade || need == rec || fade == rec {
				t.Errorf("%s dark=%v: two bars have the same colour: needs you %s, faded %s, recent %s", name, dark, need, fade, rec)
			}
			if d := contrastRatio(need, bg) - contrastRatio(fade, bg); d < 0.8 {
				t.Errorf("%s dark=%v: the faded bar %s is hardly weaker than the fresh one %s (%.1f)", name, dark, fade, need, d)
			}
		}
	}
}

func TestTheRailStopsHalfALineShortOfTheBar(t *testing.T) {
	s := loadFixture(t)
	o := viewOpts{Width: 80, Height: 40, Theme: newTheme(true, "48;5;238"), Sel: s.Sessions[0].ID}
	var rails, ends int
	for _, l := range strings.Split(render(s, tabCards, "", o), "\n") {
		switch cols := []rune(ansi.Strip(l)); {
		case len(cols) > 3 && cols[1] == '│':
			rails++
		case len(cols) > 3 && cols[1] == '╵':
			ends++
			if cols[0] != '┗' {
				t.Errorf("the rail ends on a line that is not the bottom corner: %q", string(cols))
			}
		}
	}
	if rails < 3 || ends != 1 {
		t.Errorf("%d rail lines and %d ends, want at least 3 and 1", rails, ends)
	}
}

func TestEveryCardHasABracketAndTheQuietOneIsTheDimmest(t *testing.T) {
	s := loadFixture(t)
	s.Sessions = append(s.Sessions, Session{ID: "plain001", Short: "plai", Dir: "x", Running: true, State: "working", StateLabel: "working 2h", Objective: "quiet", Queue: Queue{Items: []string{}}})
	th := newTheme(true, "48;5;238")
	tops, bottoms, cards := 0, 0, 0
	for _, l := range strings.Split(render(s, tabCards, "", styledOpts(th)), "\n") {
		cols := []rune(ansi.Strip(l))
		if len(cols) < 4 {
			continue
		}
		switch cols[0] {
		case '┏':
			tops++
		case '┗':
			bottoms++
		}
		if cols[3] != ' ' && cols[0] != '·' && !strings.HasPrefix(string(cols), "Inbox") && !strings.HasPrefix(string(cols), "+") && !strings.HasPrefix(string(cols), "[") {
			cards++
		}
	}
	if tops == 0 || tops != bottoms {
		t.Errorf("each card has a top and a bottom corner: %d and %d", tops, bottoms)
	}
	hex := func(st lipgloss.Style) string {
		r, g, b, _ := st.GetForeground().RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	for _, name := range ThemeNames {
		if name == "mono" {
			continue
		}
		for _, dark := range []bool{true, false} {
			bg := "#ffffff"
			if dark {
				bg = "#1e1e1e"
			}
			th := themeFor(dark, Settings{Theme: name, HighlightSGR: "48;5;238"})
			quiet, recent := contrastRatio(hex(th.Quiet), bg), contrastRatio(hex(th.Recent), bg)
			if quiet < 1.9 || recent-quiet < 0.5 {
				t.Errorf("%s dark=%v: the quiet bracket (%.1f:1) is not dimmer than the recent one (%.1f:1), or it is below 2:1", name, dark, quiet, recent)
			}
		}
	}
}
