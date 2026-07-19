package formatter

import (
	"strings"
	"testing"
)

// TestScanBacktick exercises every branch of scanBacktick and (through
// it) scanInterpHole: escapes, plain close, interpolation holes with
// nested braces, nested strings, nested templates, and the two
// unterminated tails.
func TestScanBacktick(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // full literal text returned
		n    int    // bytes consumed
	}{
		{"plain", "`hello`", "`hello`", 7},
		{"escaped backtick", "`a\\`b`", "`a\\`b`", 6},
		{"trailing backslash unterminated", "`a\\", "`a\\", 3},
		{"unterminated", "`abc", "`abc", 4},
		{"simple interp", "`a${x}b`", "`a${x}b`", 8},
		{"escaped brace in hole", "`${\\}}`", "`${\\}}`", 7},
		{"nested braces in hole", "`${{}}`", "`${{}}`", 7},
		{"nested template in hole", "`${`x`}`", "`${`x`}`", 8},
		{"string in hole", "`${\"}\"}`", "`${\"}\"}`", 8},
		{"unterminated hole", "`${abc", "`${abc", 6},
		{"trailing text after hole", "`a${x}`", "`a${x}`", 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n := scanBacktick(tt.in)
			if got != tt.want || n != tt.n {
				t.Errorf("scanBacktick(%q) = (%q, %d), want (%q, %d)",
					tt.in, got, n, tt.want, tt.n)
			}
		})
	}
}

// TestFormatBacktickVerbatim pins the "backtick content is left verbatim"
// rule: a template literal is one atomic token, its interior spacing is
// untouched, and it is never rewrapped even when the line exceeds the
// width. The surrounding statement is still normalised.
func TestFormatBacktickVerbatim(t *testing.T) {
	// Interior double spaces and ${...} survive untouched; the outer
	// `def x` spacing is collapsed.
	src := "def  x  `a  ${ 1 add 2 }  b`\n"
	want := "def x `a  ${ 1 add 2 }  b`\n"
	if got := Format(src); got != want {
		t.Errorf("Format(%q)\n  got:  %q\n  want: %q", src, got, want)
	}

	// A backtick literal longer than the width is not split: it is one
	// token, so the only break points are around it, and its content is
	// emitted exactly as written.
	long := "`" + strings.Repeat("x ", 50) + "`"
	out := Format("def y " + long + "\n")
	if !strings.Contains(out, long) {
		t.Errorf("long backtick literal was altered:\n%s", out)
	}
}
