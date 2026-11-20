package main

import "testing"

// r builds a string from code points, so this file holds no invisible characters.
func r(cps ...rune) string { return string(cps) }

func TestClean(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "hello world", "hello world"},
		{"escape sequence loses its ESC", "a" + r(0x1b) + "[31mb", "a[31mb"},
		{"osc loses ESC and BEL", "a" + r(0x1b) + "]0;title" + r(0x07) + "b", "a]0;titleb"},
		{"c1 controls", "a" + r(0x9b) + "b" + r(0x85) + "c", "abc"},
		{"bidi override and isolate", "a" + r(0x202e) + "b" + r(0x2066) + "c", "abc"},
		{"zero width and byte order mark", "a" + r(0x200b) + "b" + r(0xfeff) + "c", "abc"},
		{"line and paragraph separators", "a" + r(0x2028) + "b" + r(0x2029) + "c", "abc"},
		{"newline and carriage return", "a\nb\rc", "abc"},
		{"tab becomes a space", "a\tb", "a b"},
		{"trailing spaces go", "abc   ", "abc"},
		{"nul", "a" + r(0) + "b", "ab"},
		{"invalid utf-8 becomes the replacement character", "a\xffb", "a" + r(0xfffd) + "b"},
		{"wide text is kept", "\u65e5\u672c\u8a9e text", "\u65e5\u672c\u8a9e text"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := clean(c.in); got != c.want {
			t.Errorf("%s: clean(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
