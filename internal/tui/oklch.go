package tui

import (
	"fmt"
	"math"
)

// OKLCH is the colour model the colour tool steps through: lightness, chroma and hue, made so that an equal step looks like an equal step.
// Turning the hue keeps the lightness, which HSL does not (a yellow and a blue of the same HSL lightness are far apart to the eye), and the
// text colour sitrep picks for a background stays right as the hue turns. The matrices are Björn Ottosson's (https://bottosson.github.io/posts/oklab/).

type oklch struct{ l, c, h float64 } // l 0 to 1, c about 0 to 0.4, h in degrees

func toLinear(v int) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func fromLinear(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

func hexToOKLCH(c string) oklch {
	ri, gi, bi := rgbOf(c)
	r, g, b := toLinear(ri), toLinear(gi), toLinear(bi)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	a := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	bb := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	h := math.Atan2(bb, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return oklch{L, math.Hypot(a, bb), h}
}

// linearRGB is the colour as linear red, green and blue, which may lie outside 0 to 1 when it is outside the screen's range.
func (k oklch) linearRGB() (r, g, b float64) {
	a, bb := k.c*math.Cos(k.h*math.Pi/180), k.c*math.Sin(k.h*math.Pi/180)
	l := k.l + 0.3963377774*a + 0.2158037573*bb
	m := k.l - 0.1055613458*a - 0.0638541728*bb
	s := k.l - 0.0894841775*a - 1.2914855480*bb
	l, m, s = l*l*l, m*m*m, s*s*s
	return 4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s
}

func inGamut(r, g, b float64) bool {
	const eps = 1e-6
	return r >= -eps && r <= 1+eps && g >= -eps && g <= 1+eps && b >= -eps && b <= 1+eps
}

// hex is the colour as #rrggbb. A colour the screen cannot show loses chroma, never lightness or hue, until it fits.
func (k oklch) hex() string {
	k.l = math.Min(math.Max(k.l, 0), 1)
	k.c = math.Max(k.c, 0)
	if r, g, b := k.linearRGB(); !inGamut(r, g, b) {
		lo, hi := 0.0, k.c
		for i := 0; i < 24; i++ {
			mid := (lo + hi) / 2
			if r, g, b := (oklch{k.l, mid, k.h}).linearRGB(); inGamut(r, g, b) {
				lo = mid
			} else {
				hi = mid
			}
		}
		k.c = lo
	}
	r, g, b := k.linearRGB()
	ch := func(v float64) int {
		return int(math.Round(math.Min(math.Max(fromLinear(math.Min(math.Max(v, 0), 1)), 0), 1) * 255))
	}
	return fmt.Sprintf("#%02x%02x%02x", ch(r), ch(g), ch(b))
}

// Step sizes: ten degrees of hue, three points of lightness, two hundredths of chroma.
const (
	stepHue    = 10.0
	stepLight  = 0.03
	stepChroma = 0.02
	greyChroma = 0.02 // below this a colour is grey and has no hue to turn; the first turn gives it a little, so that a grey stays calm
)

// stepped moves a colour by whole steps of hue, lightness and chroma. It keeps what the screen cannot show (a chroma too big for its
// hue), so that turning the hue round and back gives the colour it began with; hex() is where it is cut to fit.
func (k oklch) stepped(hue, light, chroma int) oklch {
	if hue != 0 && k.c < greyChroma {
		k.c = 0.04
	}
	k.h = math.Mod(k.h+float64(hue)*stepHue+360, 360)
	k.l = math.Min(math.Max(k.l+float64(light)*stepLight, 0), 1)
	k.c = math.Max(k.c+float64(chroma)*stepChroma, 0)
	return k
}

// stepColor moves a colour (#rrggbb or a terminal colour number) by whole steps and returns #rrggbb.
func stepColor(c string, hue, light, chroma int) string {
	return hexToOKLCH(c).stepped(hue, light, chroma).hex()
}
