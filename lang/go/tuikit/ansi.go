package tuikit

import (
	"strconv"
	"strings"
)

// SGR emission — the one place a Style becomes escape codes, shared by
// the tty backend, the attach client, and the standalone colorize
// word. Profiles degrade truecolor through the §3.3 tables.

// Profile names a terminal's color capability.
type Profile string

const (
	ProfileTrueColor Profile = "truecolor" // 24-bit SGR 38;2;r;g;b
	Profile256       Profile = "256"       // SGR 38;5;n via To256
	Profile16        Profile = "16"        // classic 30-37/90-97 via To16
	ProfileNone      Profile = "none"      // attributes only, no color
)

// ParseProfile validates a profile name ("" defaults to truecolor).
func ParseProfile(s string) (Profile, bool) {
	switch Profile(s) {
	case "":
		return ProfileTrueColor, true
	case ProfileTrueColor, Profile256, Profile16, ProfileNone:
		return Profile(s), true
	default:
		return "", false
	}
}

// SGRReset clears all attributes.
const SGRReset = "\x1b[0m"

// SGR renders a style as one Select-Graphic-Rendition sequence, "" for
// the zero style. Unparseable color strings are skipped (style data is
// author input; painting must not fail on it).
func SGR(st Style, profile Profile) string {
	var codes []string
	if st.Bold {
		codes = append(codes, "1")
	}
	if st.Italic {
		codes = append(codes, "3")
	}
	if st.Underline {
		codes = append(codes, "4")
	}
	if st.Reverse {
		codes = append(codes, "7")
	}
	codes = appendColor(codes, st.FG, profile, false)
	codes = appendColor(codes, st.BG, profile, true)
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

func appendColor(codes []string, s string, profile Profile, background bool) []string {
	if profile == ProfileNone {
		return codes
	}
	c, ok := ParseColor(s)
	if !ok || c.Kind == ColorNone {
		return codes
	}
	switch profile {
	case ProfileTrueColor:
		if c.Kind == ColorRGB {
			base := "38"
			if background {
				base = "48"
			}
			return append(codes, base, "2",
				strconv.Itoa(c.R), strconv.Itoa(c.G), strconv.Itoa(c.B))
		}
		fallthrough
	case Profile256:
		n := c.To256()
		base := "38"
		if background {
			base = "48"
		}
		return append(codes, base, "5", strconv.Itoa(n))
	default: // Profile16
		n := c.To16()
		if n < 8 {
			if background {
				return append(codes, strconv.Itoa(40+n))
			}
			return append(codes, strconv.Itoa(30+n))
		}
		if background {
			return append(codes, strconv.Itoa(100+n-8))
		}
		return append(codes, strconv.Itoa(90+n-8))
	}
}

// StripANSI removes CSI/OSC escape sequences from a string — the
// inverse convenience for width math and log scraping.
func StripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			i++
			continue
		}
		i++ // consume ESC
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '[': // CSI: parameters then a final byte in 0x40-0x7e
			i++
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
			if i < len(s) {
				i++ // the final byte
			}
		case ']': // OSC: until BEL or ESC\
			i++
			for i < len(s) {
				if s[i] == 0x07 {
					i++
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default: // two-byte escape (e.g. ESC c)
			i++
		}
	}
	return b.String()
}
