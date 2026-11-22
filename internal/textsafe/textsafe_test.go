package textsafe

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var hostile = []string{
	"hi \033[31mRED\033]0;title\a there",
	"hi \u009b31mRED\u009d0;t\u009c \u202eX",
	"héllo 世界",
	"q\"uote back\\slash\ttab \033[31mred\033]0;x\a \u202ebidi \u2028sep \xff\xfe bad",
	"p\x01\x1fp\x7f del",
	"line one\nline two\r\nthree",
	"zero\u200bwidth\u200djoiner\ufeffbom\u00adsoft\U000e0001tag",
	"trailing spaces   ",
	"   \t  \n  ",
	"",
	"\u2029para\u2028line",
	"emoji \U0001F600 and \U0001F468\u200d\U0001F469 family",
	"\xc3\x28 invalid",
	strings.Repeat("x", 250),
	strings.Repeat("世", 250),
	"cut  " + strings.Repeat(" ", 300) + "tail",
}

func TestCleanBasics(t *testing.T) {
	for in, want := range map[string]string{
		"hi \033[31mRED\033]0;title\a there": "hi [31mRED]0;title there",
		"héllo 世界":                           "héllo 世界",
		"a\tb\nc":                            "a b c",
		"keep   ":                            "keep",
		"\u202eX\u009b":                      "X",
	} {
		if got := Clean(in, 200); got.String() != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Clean(strings.Repeat("x", 250), 200); len(got.String()) != 200 {
		t.Errorf("Clean cut to %d, want 200", len(got.String()))
	}
	if got := Clean(strings.Repeat("世", 250), 200); utf8.RuneCountInString(got.String()) != 200 {
		t.Errorf("Clean cut to %d runes, want 200", utf8.RuneCountInString(got.String()))
	}
}

func TestOutputIsValidAndClean(t *testing.T) {
	for _, in := range hostile {
		for _, got := range []Text{Clean(in, 200), Sane(in, 140), Strip(in)} {
			if !utf8.ValidString(got.String()) {
				t.Errorf("%q gives invalid UTF-8 %q", in, got)
			}
			for _, r := range got.String() {
				if r != '\n' && r != '\t' && unsafe(r) {
					t.Errorf("%q leaves %U in %q", in, r, got)
				}
			}
		}
	}
}

func TestStripKeepsBreaks(t *testing.T) {
	if got := Strip("a\n\tb\033[0m\u202e"); got.String() != "a\n\tb[0m" {
		t.Errorf("Strip = %q", got)
	}
}

func TestFit(t *testing.T) {
	for _, c := range []struct {
		in   string
		cols int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"a bit too long", 10, "a bit too…"},
		{"世界世界世界", 7, "世界世…"},
	} {
		got := Fit(Clean(c.in, 0), c.cols)
		if got.String() != c.want || Width(got) > c.cols {
			t.Errorf("Fit(%q, %d) = %q (width %d), want %q", c.in, c.cols, got, Width(got), c.want)
		}
	}
}

// The specification of Clean, Sane and Strip, written plainly and character by character, to check the implementation against.
func spec(s string, max int, trim bool) string {
	var out []rune
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t' || r == '\u2028' || r == '\u2029':
			out = append(out, ' ')
		case unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r):
		default:
			out = append(out, r)
		}
	}
	if trim {
		for len(out) > 0 && out[len(out)-1] == ' ' {
			out = out[:len(out)-1]
		}
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return string(out)
}

func specStrip(s string) string {
	var out []rune
	for _, r := range s {
		if r == '\n' || r == '\t' || !(unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)) {
			out = append(out, r)
		}
	}
	return string(out)
}

// corpus adds seeded random strings built from the characters that matter.
func corpus() []string {
	pool := []rune{'a', 'Z', ' ', '\t', '\n', '\r', 0, 0x1b, 0x7f, 0x80, 0x9b, 0x9f, 0xa0, 0xad, 0x2028, 0x2029, 0x200b, 0x200d, 0x202a, 0x202e, 0x2066, 0x2069, 0xfeff, 0x4e16, 0x1f600, 0xe0001, 0xe0020, 0x061c, 0x180e, 0x3000, 0xfffd}
	rng := rand.New(rand.NewSource(1))
	out := append([]string{}, hostile...)
	for i := 0; i < 300; i++ {
		var b strings.Builder
		for n := rng.Intn(40); n > 0; n-- {
			b.WriteRune(pool[rng.Intn(len(pool))])
		}
		out = append(out, b.String())
	}
	return out
}

func TestMatchesTheSpecification(t *testing.T) {
	for _, in := range corpus() {
		if !utf8.ValidString(in) { // invalid UTF-8 becomes U+FFFD, which the specification has no word for
			continue
		}
		if got, want := Clean(in, 200).String(), spec(in, 200, true); got != want {
			t.Errorf("Clean(%q)\n  got:  %q\n  want: %q", in, got, want)
		}
		if got, want := Sane(in, 140).String(), spec(in, 140, false); got != want {
			t.Errorf("Sane(%q)\n  got:  %q\n  want: %q", in, got, want)
		}
		if got, want := Strip(in).String(), specStrip(in); got != want {
			t.Errorf("Strip(%q)\n  got:  %q\n  want: %q", in, got, want)
		}
	}
}

func TestFitSmallWidths(t *testing.T) {
	for _, cols := range []int{-1, 0} {
		if got := Fit(Clean("abc", 0), cols); got.String() != "" {
			t.Errorf("Fit(abc, %d) = %q", cols, got)
		}
	}
	if got := Fit(Clean("abc", 0), 1); got.String() != "\u2026" || Width(got) != 1 {
		t.Errorf("Fit(abc, 1) = %q", got)
	}
}

// Every code point: none that is a control or a format character in the Unicode tables of Go survives Clean, Sane or Strip, and every other
// one is kept, so that nothing is lost that is safe (a line break becomes a space).
func TestEveryCodePointIsKeptOrRemovedAsTheSpecificationSays(t *testing.T) {
	for c := rune(0); c <= 0x10FFFF; c++ {
		if c >= 0xD800 && c <= 0xDFFF {
			continue
		}
		in := "a" + string(c) + "b"
		bad := unicode.Is(unicode.Cc, c) || unicode.Is(unicode.Cf, c)
		for name, got := range map[string]string{"Clean": Clean(in, 0).String(), "Sane": Sane(in, 0).String(), "Strip": Strip(in).String()} {
			for _, r := range got {
				if r != '\n' && r != '\t' && (unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)) {
					t.Fatalf("%s kept U+%04X", name, r)
				}
			}
			if !bad && c != '\u2028' && c != '\u2029' && got != in {
				t.Fatalf("%s lost the safe character U+%04X: %q", name, c, got)
			}
		}
	}
}

func TestCleanMarkedEndsInAnEllipsisOnlyWhenItCut(t *testing.T) {
	if got := CleanMarked(strings.Repeat("x", 200), 200).String(); got != strings.Repeat("x", 200) {
		t.Errorf("text that fits is kept whole: %q", got)
	}
	got := CleanMarked(strings.Repeat("word ", 100), 200).String()
	if !strings.HasSuffix(got, "…") || utf8.RuneCountInString(got) > 200 || strings.Contains(got, " …") {
		t.Errorf("a cut ends in an ellipsis within 200 characters: %q", got)
	}
	if got := CleanMarked(strings.Repeat("世", 300), 200).String(); utf8.RuneCountInString(got) != 200 || !strings.HasSuffix(got, "…") {
		t.Errorf("counts characters, not bytes: %d", utf8.RuneCountInString(got))
	}
	if got := CleanMarked("a\x1b[31mb"+strings.Repeat("y", 300), 50).String(); strings.Contains(got, "\x1b") {
		t.Errorf("still cleaned: %q", got)
	}
}
