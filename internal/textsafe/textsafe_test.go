package textsafe

import (
	"math/rand"
	"os/exec"
	"strings"
	"testing"
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

// perl is the oracle: the expressions are the ones bin/sitrep and the hook ran.
func perl(t *testing.T, expr, in string) string {
	t.Helper()
	cmd := exec.Command("perl", "-CSD", "-0777", "-pe", expr)
	cmd.Stdin = strings.NewReader(in)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("perl: %v", err)
	}
	return string(out)
}

const (
	perlClean = `s/[\n\t\p{Zl}\p{Zp}]/ /g; s/[\p{Cc}\p{Cf}]//g; s/ +$//; $_=substr($_,0,200)`
	perlSane  = `s/[\n\t\p{Zl}\p{Zp}]/ /g; s/[\p{Cc}\p{Cf}]//g; $_=substr($_,0,140)`
	perlStrip = `s/(?!\n|\t)[\p{Cc}\p{Cf}]//g`
)

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

func TestMatchesPerl(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl is not installed")
	}
	for _, in := range corpus() {
		// Invalid UTF-8 is left out: perl keeps the bad bytes and Go replaces them, a deliberate difference.
		if !utf8.ValidString(in) {
			continue
		}
		if got, want := Clean(in, 200).String(), perl(t, perlClean, in); got != want {
			t.Errorf("Clean(%q)\n  go:   %q\n  perl: %q", in, got, want)
		}
		if got, want := Sane(in, 140).String(), perl(t, perlSane, in); got != want {
			t.Errorf("Sane(%q)\n  go:   %q\n  perl: %q", in, got, want)
		}
		if got, want := Strip(in).String(), perl(t, perlStrip, in); got != want {
			t.Errorf("Strip(%q)\n  go:   %q\n  perl: %q", in, got, want)
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

// Every code point, against perl in one run. Perl's Unicode tables can be older than Go's, so Go may be stricter, never looser.
func TestAllCodePointsAgainstPerl(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl is not installed")
	}
	cmd := exec.Command("perl", "-CSD", "-e", `for my $c (0..0x10FFFF) { next if $c >= 0xD800 && $c <= 0xDFFF; my $s = chr($c); print $c, "\n" if $s =~ /[\p{Cc}\p{Cf}]/ }`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	perlUnsafe := map[rune]bool{}
	for _, f := range strings.Fields(string(out)) {
		var c int
		for _, d := range f {
			c = c*10 + int(d-'0')
		}
		perlUnsafe[rune(c)] = true
	}
	for c := rune(0); c <= 0x10FFFF; c++ {
		if c >= 0xD800 && c <= 0xDFFF {
			continue
		}
		if perlUnsafe[c] && !unsafe(c) {
			t.Errorf("U+%04X is unsafe to perl and safe to Go", c)
		}
	}
}
