package tui

import (
	"strings"
	"unicode"
)

// clean makes text from sitrep safe to draw. Control, format and line or paragraph
// separator characters are dropped, because a terminal obeys some of them, and a tab
// becomes a space. Trailing spaces go too, as in the shell.
func clean(s string) string {
	var b strings.Builder
	for _, r := range s {
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
