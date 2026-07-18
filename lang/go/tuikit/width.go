package tuikit

// The single display-width source (TUI.0.md §4.2): every path that
// measures text — the P2 layout/painters here, the P3 termback, the P4
// attach client — asks these functions, so widths can never disagree
// between measurement and painting.
//
// Hand-rolled wcwidth-class tables, no dependency: combining marks and
// zero-width code points are 0, East-Asian wide/fullwidth ranges and
// emoji presentation are 2, everything else is 1. Full grapheme-cluster
// segmentation (ZWJ sequences, flags) is future precision work — the
// tables below cover the widths terminals actually honour; goldens pin
// CJK and emoji rows.

type widthRange struct{ lo, hi rune }

// zeroWidth: combining marks, zero-width joiners/spaces, variation
// selectors — code points terminals render at width 0.
var zeroWidth = []widthRange{
	{0x0300, 0x036F},   // combining diacritical marks
	{0x0483, 0x0489},   // combining cyrillic
	{0x0591, 0x05BD},   // hebrew points
	{0x0610, 0x061A},   // arabic marks
	{0x064B, 0x065F},   // arabic marks
	{0x0670, 0x0670},   // arabic letter superscript alef
	{0x06D6, 0x06DC},   // arabic small high marks
	{0x0E31, 0x0E31},   // thai mai han-akat
	{0x0E34, 0x0E3A},   // thai vowels/tones
	{0x0E47, 0x0E4E},   // thai tones
	{0x200B, 0x200F},   // zero-width space/joiners, directional marks
	{0x1AB0, 0x1AFF},   // combining extended
	{0x1DC0, 0x1DFF},   // combining supplement
	{0x20D0, 0x20FF},   // combining for symbols
	{0xFE00, 0xFE0F},   // variation selectors
	{0xFE20, 0xFE2F},   // combining half marks
	{0xE0100, 0xE01EF}, // variation selectors supplement
}

// wide: East-Asian Wide/Fullwidth plus emoji presentation ranges —
// code points terminals render at width 2.
var wide = []widthRange{
	{0x1100, 0x115F},   // hangul jamo
	{0x2E80, 0x303E},   // CJK radicals … CJK symbols/punctuation
	{0x3041, 0x33FF},   // hiragana … CJK compatibility
	{0x3400, 0x4DBF},   // CJK ext A
	{0x4E00, 0x9FFF},   // CJK unified
	{0xA000, 0xA4CF},   // yi
	{0xAC00, 0xD7A3},   // hangul syllables
	{0xF900, 0xFAFF},   // CJK compatibility ideographs
	{0xFE30, 0xFE4F},   // CJK compatibility forms
	{0xFF00, 0xFF60},   // fullwidth forms
	{0xFFE0, 0xFFE6},   // fullwidth signs
	{0x1F300, 0x1F64F}, // emoji: symbols/pictographs, emoticons
	{0x1F680, 0x1F6FF}, // emoji: transport
	{0x1F900, 0x1F9FF}, // emoji: supplemental
	{0x20000, 0x2FFFD}, // CJK ext B…
	{0x30000, 0x3FFFD}, // CJK ext G…
}

func inRanges(r rune, table []widthRange) bool {
	for _, w := range table {
		if r >= w.lo && r <= w.hi {
			return true
		}
	}
	return false
}

// RuneWidth reports the display-cell width of one code point: 0, 1 or 2.
func RuneWidth(r rune) int {
	if r == 0 || inRanges(r, zeroWidth) {
		return 0
	}
	if inRanges(r, wide) {
		return 2
	}
	return 1
}

// StringWidth reports the display-cell width of a string.
func StringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// truncateToWidth returns the longest prefix of s whose display width
// fits limit, and that prefix's width.
func truncateToWidth(s string, limit int) (string, int) {
	w := 0
	for i, r := range s {
		rw := RuneWidth(r)
		if w+rw > limit {
			return s[:i], w
		}
		w += rw
	}
	return s, w
}
