// Package textsafe makes text from prompts, transcripts, models and settings safe to store and to print (invariant 1 in AGENTS.md).
// A Text can only be made here, and the render and encode code accepts nothing else, so unfiltered text cannot reach a terminal by accident.
package textsafe

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// Text has had its control (Cc, including C1) and format (Cf, including bidi) characters removed.
type Text string

func (t Text) String() string { return string(t) }

// unsafe reports whether r is a control or format character.
func unsafe(r rune) bool { return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) }

// lineBreak reports whether r ends a line or a paragraph (tab and newline included).
func lineBreak(r rune) bool {
	return r == '\n' || r == '\t' || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
}

func valid(s string) string { return strings.ToValidUTF8(s, "�") }

// cut keeps at most max runes; a max below 1 means no limit.
func cut(s string, max int) string {
	if max < 1 {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}

// Clean makes text for storing: one flat line, no control or format characters, no trailing spaces, at most max characters.
func Clean(s string, max int) Text {
	var b strings.Builder
	for _, r := range valid(s) {
		switch {
		case lineBreak(r):
			b.WriteByte(' ')
		case unsafe(r):
		default:
			b.WriteRune(r)
		}
	}
	return Text(cut(strings.TrimRight(b.String(), " "), max))
}

// Sane is Clean without the trailing-space trim, for the hook's own fields.
func Sane(s string, max int) Text {
	var b strings.Builder
	for _, r := range valid(s) {
		switch {
		case lineBreak(r):
			b.WriteByte(' ')
		case unsafe(r):
		default:
			b.WriteRune(r)
		}
	}
	return Text(cut(b.String(), max))
}

// Strip makes text for showing: it removes the same characters but keeps line breaks and tabs.
func Strip(s string) Text {
	var b strings.Builder
	for _, r := range valid(s) {
		if r != '\n' && r != '\t' && unsafe(r) {
			continue
		}
		b.WriteRune(r)
	}
	return Text(b.String())
}

// Width is how many terminal columns t takes.
func Width(t Text) int { return runewidth.StringWidth(string(t)) }

// Fit cuts t to at most cols columns, ending in an ellipsis when it cut anything.
func Fit(t Text, cols int) Text {
	return Text(runewidth.Truncate(string(t), cols, "…"))
}
