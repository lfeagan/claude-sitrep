package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func styledOpts(theme Theme) viewOpts { return viewOpts{Width: 80, Height: 60, Theme: theme} }

func TestLabelsAndNamesStandOut(t *testing.T) {
	s := loadFixture(t)
	frame := render(s, tabCards, "", styledOpts(newTheme(true, "48;5;238")))
	var painted, plain int
	for _, l := range strings.Split(frame, "\n") {
		switch {
		case strings.Contains(l, "\x1b[1mObjective:\x1b[22m"): // a row with a background: bold, and the colour behind it is not reset
			painted++
			after := l[strings.Index(l, "\x1b[22m")+len("\x1b[22m"):] // the text that follows the label, still on the row's colour
			if i := strings.Index(after, "\x1b[m"); i <= 1 || strings.Contains(l[:strings.Index(l, "Objective:")], "\x1b[m") {
				t.Errorf("a reset right after the label ends the row's colour: %q", l)
			}
		case strings.Contains(l, "Objective:\x1b[22;39m"): // a row with no background: bold and the accent colour
			plain++
		}
	}
	if painted == 0 || plain == 0 {
		t.Errorf("labels on painted rows %d, on plain rows %d", painted, plain)
	}
	if !strings.Contains(frame, "\x1b[1m") || !strings.Contains(frame, "Now:\x1b[22") {
		t.Errorf("the Now label is not set off")
	}
}

func TestEmphasisAddsNoText(t *testing.T) {
	s := loadFixture(t)
	for name, th := range map[string]Theme{"default": newTheme(true, "48;5;238"), "light": newTheme(false, "48;5;238")} {
		got := ansi.Strip(render(s, tabCards, "", styledOpts(th)))
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

func TestAPlainThemeAndABoldRowGetNoEmphasis(t *testing.T) {
	s := loadFixture(t)
	if strings.ContainsRune(render(s, tabCards, "", styledOpts(plainTheme())), 0x1b) {
		t.Error("the plain theme has no escape codes")
	}
	bold := 0
	for _, l := range strings.Split(render(s, tabCards, "", styledOpts(monoTheme())), "\n") {
		if strings.HasPrefix(l, "\x1b[1m") { // a recently active row is bold all through: a bold label inside it would end the bold
			bold++
			if strings.Contains(l, "\x1b[22m") {
				t.Errorf("emphasis inside a bold row: %q", l)
			}
		}
	}
	if bold == 0 {
		t.Error("the fixture has no recent session in the mono theme")
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
