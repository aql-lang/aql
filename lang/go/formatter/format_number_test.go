package formatter

import "testing"

// TestScanNumber pins that scanNumber consumes each boru numeric literal
// whole — plain decimals, 0d BigInteger/BigDecimal, 0x/0o/0b base
// literals, `_` separators, and `e` exponents — and stops correctly so a
// following dot-access or word is left intact.
func TestScanNumber(t *testing.T) {
	tests := []struct{ in, want string }{
		{"42", "42"},
		{"-3", "-3"},
		{"1.5", "1.5"},
		{"1e5", "1e5"},
		{"1.5e3", "1.5e3"},
		{"1E-2", "1E-2"},
		{"0d123", "0d123"},
		{"0d12.5", "0d12.5"},
		{"0d1_000_000", "0d1_000_000"},
		{"0d1.5e3", "0d1.5e3"},
		{"0D5", "0D5"},
		{"-0d5", "-0d5"},
		{"0x1F", "0x1F"},
		{"0XaB_cd", "0XaB_cd"},
		{"0o17", "0o17"},
		{"0O7", "0O7"},
		{"0b1010", "0b1010"},
		{"0B1_1", "0B1_1"},
		// Prefix without a matching digit: `0` is the number, the rest a word.
		{"0dfoo", "0"},
		{"0xzz", "0"},
		{"0o9", "0"},
		{"0b2", "0"},
		// A trailing dot is left for dot-access when no digit follows.
		{"0d5.foo", "0d5"},
		// `e` not starting a real exponent is left alone.
		{"1each", "1"},
		{"1e", "1"},
		// Short inputs exercise the prefix-length guard.
		{"0", "0"},
		{"0d", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := scanNumber(tt.in, 0); got != tt.want {
				t.Errorf("scanNumber(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFormatNumberLiterals is the end-to-end guard: the formatter never
// splits a numeric literal it once corrupted (0d12.5 -> "0 d12.5").
func TestFormatNumberLiterals(t *testing.T) {
	src := "0d12.5 add 0d1_000 0d1.5e3 0xFF 0b1010 0o17 1e5\n"
	if got := Format(src); got != src {
		t.Errorf("Format(%q) = %q, want it unchanged", src, got)
	}
}
