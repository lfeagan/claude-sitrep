// Package textsafe makes text from prompts, transcripts, models and settings safe to store and to print (invariant 1 in AGENTS.md).
// A Text can only be made here, and the render and encode code accepts nothing else, so unfiltered text cannot reach a terminal by accident.
package textsafe

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// Text has had its control (Cc, including C1) and format (Cf, including bidi) characters removed. The field is private, so no other package
// can make one from raw text; it gets one from Clean, Sane or Strip.
type Text struct{ s string }

func (t Text) String() string { return t.s }

func init() {
	runewidth.DefaultCondition.EastAsianWidth = false // the locale must not change what a frame looks like
}

// unsafe reports whether r is a control or format character.
func unsafe(r rune) bool { return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) }

// lineBreak reports whether r ends a line or a paragraph (tab and newline included).
func lineBreak(r rune) bool {
	return r == '\n' || r == '\t' || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
}

// valid replaces each byte that is not UTF-8 with U+FFFD, one for one.
func valid(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteRune('\uFFFD')
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

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

// head keeps the start of s that can matter for max characters: a character takes at most four bytes, so a longer text is cut before it is
// filtered, and a huge prompt is not scanned in full. Only a start that is mostly control characters can then come out shorter than max.
func head(s string, max int) string {
	if max < 1 || len(s) <= max*4+64 {
		return s
	}
	s = s[:max*4+64]
	for len(s) > 0 && !utf8.RuneStart(s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] >= 0xC0 { // a lead byte with its continuation cut off
		s = s[:len(s)-1]
	}
	return s
}

// flat makes one line without control or format characters.
func flat(s string) string {
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
	return b.String()
}

// Clean makes text for storing: one flat line, no control or format characters, no trailing spaces, at most max characters (a max below 1
// means no limit). Invalid UTF-8 becomes U+FFFD, so state written by sitrep is always valid UTF-8.
func Clean(s string, max int) Text {
	return Text{cut(strings.TrimRight(flat(head(s, max)), " "), max)}
}

// Sane is Clean without the trailing-space trim, for the hook's own fields.
func Sane(s string, max int) Text { return Text{cut(flat(head(s, max)), max)} }

// Strip makes text for showing: it removes the same characters but keeps line breaks and tabs.
func Strip(s string) Text {
	var b strings.Builder
	for _, r := range valid(s) {
		if r != '\n' && r != '\t' && unsafe(r) {
			continue
		}
		b.WriteRune(r)
	}
	return Text{b.String()}
}

// Width is how many terminal columns t takes.
func Width(t Text) int { return runewidth.StringWidth(t.s) }

// Fit cuts t to at most cols columns, ending in an ellipsis when it cut anything.
func Fit(t Text, cols int) Text {
	if cols < 1 {
		return Text{}
	}
	return Text{runewidth.Truncate(t.s, cols, "…")}
}

// MarshalText lets a Text be a JSON string, and nothing else is accepted there.
func (t Text) MarshalText() ([]byte, error) { return []byte(t.s), nil }

// Truncate keeps at most max characters of t (a max below 1 means no limit), with no ellipsis.
func Truncate(t Text, max int) Text { return Text{cut(t.s, max)} }
