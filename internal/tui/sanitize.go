package tui

import (
	"strings"
	"unicode"
)

// maxFieldRunes is the most characters of one field the dashboard keeps: far more than any field sitrep writes (a path is cut at 300, a
// note at 200), so only a hostile or damaged document reaches it.
const maxFieldRunes = 4096

// clean makes text from sitrep safe to draw. Control, format and line or paragraph
// separator characters are dropped, because a terminal obeys some of them, and a tab
// becomes a space. Trailing spaces go too, as in the shell.
func clean(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxFieldRunes {
			break // no field sitrep writes is this long: a document with a megabyte in one is not drawn in full
		}
		n++
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Zl, r), unicode.Is(unicode.Zp, r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), " ")
}
