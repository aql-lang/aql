package tuikit

import "testing"

func TestParseProfile(t *testing.T) {
	if p, ok := ParseProfile(""); !ok || p != ProfileTrueColor {
		t.Errorf("default = %v,%v", p, ok)
	}
	for _, s := range []string{"truecolor", "256", "16", "none"} {
		if _, ok := ParseProfile(s); !ok {
			t.Errorf("%s rejected", s)
		}
	}
	if _, ok := ParseProfile("vga"); ok {
		t.Error("unknown profile accepted")
	}
}

func TestSGRAttributesAndProfiles(t *testing.T) {
	if got := SGR(Style{}, ProfileTrueColor); got != "" {
		t.Errorf("zero style = %q", got)
	}
	full := Style{FG: "#ff0000", BG: "blue", Bold: true, Italic: true, Underline: true, Reverse: true}
	if got := SGR(full, ProfileTrueColor); got != "\x1b[1;3;4;7;38;2;255;0;0;48;5;4m" {
		t.Errorf("truecolor = %q", got)
	}
	// truecolor profile falls through to 256 for named/indexed colors
	if got := SGR(Style{FG: "203"}, ProfileTrueColor); got != "\x1b[38;5;203m" {
		t.Errorf("indexed under truecolor = %q", got)
	}
	if got := SGR(full, Profile256); got != "\x1b[1;3;4;7;38;5;196;48;5;4m" {
		t.Errorf("256 = %q", got)
	}
	// 16-profile: dim fg 30-37 / bright fg 90-97, dim bg 40-47 / bright bg 100-107
	if got := SGR(Style{FG: "red", BG: "bright-red"}, Profile16); got != "\x1b[31;101m" {
		t.Errorf("16 = %q", got)
	}
	if got := SGR(Style{FG: "bright-white", BG: "black"}, Profile16); got != "\x1b[97;40m" {
		t.Errorf("16 bright fg = %q", got)
	}
	// a truecolor RGB background takes the 48;2 form
	if got := SGR(Style{BG: "#010203"}, ProfileTrueColor); got != "\x1b[48;2;1;2;3m" {
		t.Errorf("rgb bg = %q", got)
	}
	// none-profile keeps attributes, drops color
	if got := SGR(Style{FG: "red", Bold: true}, ProfileNone); got != "\x1b[1m" {
		t.Errorf("none = %q", got)
	}
	// unparseable colors are skipped, not fatal
	if got := SGR(Style{FG: "blurple"}, ProfileTrueColor); got != "" {
		t.Errorf("bad color = %q", got)
	}
}

func TestStripANSI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"\x1b[1;31mred\x1b[0m", "red"},
		{"a\x1b]0;title\x07b", "ab"},        // OSC … BEL
		{"a\x1b]0;title\x1b\\b", "ab"},      // OSC … ST
		{"a\x1bcb", "ab"},                   // two-byte escape
		{"tail\x1b", "tail"},                // trailing bare ESC
		{"cut\x1b[31", "cut"},               // truncated CSI
		{"osc\x1b]never-terminated", "osc"}, // unterminated OSC
	}
	for _, c := range cases {
		if got := StripANSI(c.in); got != c.want {
			t.Errorf("StripANSI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if w := StringWidth(StripANSI("\x1b[31m漢\x1b[0m")); w != 2 {
		t.Errorf("strip+width = %d", w)
	}
}
