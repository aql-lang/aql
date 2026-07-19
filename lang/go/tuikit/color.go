package tuikit

import (
	"strconv"
	"strings"
)

// Color helpers for painting backends (TUI.0.md §3.3): style data stays
// full-fidelity strings everywhere; a backend that can't render
// truecolor degrades AT PAINT TIME with these pure tables. tuikit owns
// them so the P3 termback and any future backend degrade identically.

// ColorKind discriminates a parsed color.
type ColorKind int

const (
	ColorNone  ColorKind = iota // empty string: no color set
	ColorNamed                  // one of the 16 ANSI names
	ColorIndex                  // a 256-palette index
	ColorRGB                    // #rrggbb truecolor
)

// Color is a parsed style color.
type Color struct {
	Kind    ColorKind
	Name    string // ColorNamed: the canonical lowercase name
	Index   int    // ColorNamed: 0-15 ANSI slot; ColorIndex: 0-255
	R, G, B int    // ColorRGB
}

var ansiNames = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3,
	"blue": 4, "magenta": 5, "cyan": 6, "white": 7,
	"bright-black": 8, "bright-red": 9, "bright-green": 10, "bright-yellow": 11,
	"bright-blue": 12, "bright-magenta": 13, "bright-cyan": 14, "bright-white": 15,
	"grey": 8, "gray": 8, // the style-map convention for dim text
}

// ParseColor parses a style-map color string: "", an ANSI name, a
// 256-palette index in decimal, or "#rrggbb". The bool reports whether
// the string was recognised (an empty string parses as ColorNone, true).
func ParseColor(s string) (Color, bool) {
	if s == "" {
		return Color{Kind: ColorNone}, true
	}
	lower := strings.ToLower(s)
	if idx, ok := ansiNames[lower]; ok {
		return Color{Kind: ColorNamed, Name: lower, Index: idx}, true
	}
	if strings.HasPrefix(s, "#") {
		if len(s) != 7 {
			return Color{}, false
		}
		v, err := strconv.ParseUint(s[1:], 16, 32)
		if err != nil {
			return Color{}, false
		}
		n := int(v)
		return Color{Kind: ColorRGB, R: n >> 16 & 0xff, G: n >> 8 & 0xff, B: n & 0xff}, true
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 255 {
		return Color{Kind: ColorIndex, Index: n}, true
	}
	return Color{}, false
}

// To256 degrades a color to the 256-palette: RGB maps onto the 6×6×6
// cube (or the grey ramp when the channels agree closely); named and
// indexed colors pass through.
func (c Color) To256() int {
	switch c.Kind {
	case ColorNamed, ColorIndex:
		return c.Index
	case ColorRGB:
		if isGreyish(c.R, c.G, c.B) {
			// 24-step grey ramp: indices 232-255 cover 8..238
			g := (c.R + c.G + c.B) / 3
			if g < 8 {
				return 16 // cube black
			}
			if g > 238 {
				return 231 // cube white
			}
			return 232 + (g-8)/10
		}
		return 16 + 36*cubeStep(c.R) + 6*cubeStep(c.G) + cubeStep(c.B)
	default:
		return -1 // no color
	}
}

// To16 degrades a color to the 16 ANSI slots.
func (c Color) To16() int {
	switch c.Kind {
	case ColorNamed:
		return c.Index
	case ColorIndex:
		if c.Index < 16 {
			return c.Index
		}
		return ansi16FromRGB(rgbFrom256(c.Index))
	case ColorRGB:
		return ansi16FromRGB([3]int{c.R, c.G, c.B})
	default:
		return -1
	}
}

func isGreyish(r, g, b int) bool {
	max, min := r, r
	for _, v := range []int{g, b} {
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	return max-min < 24
}

// cubeStep maps a 0-255 channel onto the 6-step color-cube axis.
func cubeStep(v int) int {
	if v < 48 {
		return 0
	}
	if v < 115 {
		return 1
	}
	return (v - 35) / 40
}

// rgbFrom256 expands a 256-palette index back to RGB (cube + grey ramp;
// the first 16 use the standard VGA-ish values).
func rgbFrom256(i int) [3]int {
	if i < 16 {
		base := []int{0, 128}
		if i >= 8 {
			base = []int{85, 255}
		}
		v := i & 7
		pick := func(bit int) int {
			if v>>bit&1 == 1 {
				return base[1]
			}
			return base[0]
		}
		return [3]int{pick(0), pick(1), pick(2)}
	}
	if i >= 232 {
		g := 8 + (i-232)*10
		return [3]int{g, g, g}
	}
	i -= 16
	steps := []int{0, 95, 135, 175, 215, 255}
	return [3]int{steps[i/36], steps[i/6%6], steps[i%6]}
}

// ansi16FromRGB maps RGB to the nearest of the 16 ANSI slots by
// channel thresholding: bright when any channel is strong.
func ansi16FromRGB(rgb [3]int) int {
	idx := 0
	if rgb[0] > 127 {
		idx |= 1
	}
	if rgb[1] > 127 {
		idx |= 2
	}
	if rgb[2] > 127 {
		idx |= 4
	}
	if rgb[0] > 191 || rgb[1] > 191 || rgb[2] > 191 {
		idx |= 8
	}
	return idx
}
