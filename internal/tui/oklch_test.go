package tui

import (
	"math"
	"testing"
)

func channels(c string) [3]int {
	r, g, b := rgbOf(c)
	return [3]int{r, g, b}
}

func TestOKLCHRoundTrip(t *testing.T) {
	for r := 0; r < 256; r += 15 {
		for g := 0; g < 256; g += 15 {
			for b := 0; b < 256; b += 15 {
				c := "#" + hex2(r) + hex2(g) + hex2(b)
				got := hexToOKLCH(c).hex()
				a, d := channels(c), channels(got)
				for i := range a {
					if diff := a[i] - d[i]; diff < -1 || diff > 1 {
						t.Fatalf("%s came back as %s", c, got)
					}
				}
			}
		}
	}
}

func hex2(v int) string { return string("0123456789abcdef"[v>>4]) + string("0123456789abcdef"[v&15]) }

func TestOKLCHKnownColours(t *testing.T) {
	if k := hexToOKLCH("#ffffff"); math.Abs(k.l-1) > 0.001 || k.c > 0.001 {
		t.Errorf("white: %+v", k)
	}
	if k := hexToOKLCH("#000000"); k.l > 0.001 {
		t.Errorf("black: %+v", k)
	}
	if k := hexToOKLCH("#ff0000"); math.Abs(k.l-0.628) > 0.005 || math.Abs(k.c-0.2577) > 0.005 || math.Abs(k.h-29.2) > 0.5 {
		t.Errorf("red is L .628 C .258 h 29.2: %+v", k)
	}
}

func TestOKLCHHueTurnsKeepTheLightness(t *testing.T) {
	start := hexToOKLCH("#7a6fd8")
	k := start
	for i := 0; i < 36; i++ {
		k = k.stepped(1, 0, 0)
		if got := hexToOKLCH(k.hex()); math.Abs(got.l-start.l) > 0.02 {
			t.Fatalf("step %d: lightness moved from %.3f to %.3f", i, start.l, got.l)
		}
	}
	// a full turn comes back to the colour it began with, even though some hues in between could not show all its chroma
	if got := k.hex(); got != "#7a6fd8" && got != "#7a6fd9" && got != "#7a6ed8" && got != "#7b6fd8" {
		a, d := channels("#7a6fd8"), channels(got)
		for i := range a {
			if diff := a[i] - d[i]; diff < -1 || diff > 1 {
				t.Errorf("a full turn gave %s", got)
			}
		}
	}
}

func TestOKLCHLightnessAndChromaStayOnTheScreen(t *testing.T) {
	c := "#5b3fd1"
	prev := hexToOKLCH(c).l
	for i := 0; i < 40; i++ {
		c = stepColor(c, 0, 1, 0)
		if l := hexToOKLCH(c).l; l < prev-0.001 {
			t.Fatalf("lighter steps got darker at %d: %.3f then %.3f", i, prev, l)
		} else {
			prev = l
		}
	}
	if c != "#ffffff" {
		t.Errorf("many lighter steps end at white: %s", c)
	}
	for i := 0; i < 40; i++ {
		c = stepColor(c, 0, -1, 0)
	}
	if c != "#000000" {
		t.Errorf("many darker steps end at black: %s", c)
	}
	// far more chroma than the screen has is cut back, keeping the hue
	if got := (oklch{0.6, 0.9, 140}).hex(); len(got) != 7 || got[0] != '#' {
		t.Errorf("out of range: %q", got)
	}
	if k := hexToOKLCH((oklch{0.6, 0.9, 140}).hex()); math.Abs(k.h-140) > 5 || math.Abs(k.l-0.6) > 0.02 {
		t.Errorf("the hue and the lightness are kept: %+v", k)
	}
}

func TestOKLCHGreyGainsAHueOnTheFirstTurn(t *testing.T) {
	grey := "#808080"
	if got := stepColor(grey, 1, 0, 0); got == grey {
		t.Error("a hue step on a grey must show")
	}
	if got := stepColor(grey, 0, 1, 0); channels(got)[0] != channels(got)[1] {
		t.Errorf("lightness alone keeps a grey grey: %s", got)
	}
}

func TestOKLCHTerminalNumbersConvert(t *testing.T) {
	if got := stepColor("196", 1, 0, 0); len(got) != 7 || got[0] != '#' {
		t.Errorf("%q", got)
	}
}
