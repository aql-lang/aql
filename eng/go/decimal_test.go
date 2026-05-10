package eng

import (
	"strconv"
	"testing"
)

// TestDecimalFormat pins down formatDecimal's contract: every TDecimal
// value renders with at least one '.' or an exponent, so the type stays
// visually distinct from TInteger. Whole-valued floats receive a ".0"
// suffix; fractional values pass through 'f' formatting unchanged.
func TestDecimalFormat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.0, "0.0"},
		{1.0, "1.0"},
		{2.0, "2.0"},
		{-3.0, "-3.0"},
		{3.14, "3.14"},
		{-0.5, "-0.5"},
		{1e20, "100000000000000000000.0"}, // 'f' expands; formatDecimal then appends ".0" since no fractional/exponent
		{1e-7, "0.0000001"},
		{0.1, "0.1"},
		{0.2, "0.2"},
	}
	for _, c := range cases {
		got := formatDecimal(c.in)
		if got != c.want {
			t.Errorf("formatDecimal(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDecimalArithmeticIsFloat64 documents the float64 imprecision that
// the engine inherits from Go's underlying numeric type. 0.1 + 0.2 is
// the canonical demo: in IEEE 754 binary float, neither 0.1 nor 0.2
// has an exact representation, and their sum lands at 0.30000000000000004,
// not 0.3.
//
// Pins the artefact so a future port to exact decimal arithmetic (e.g.
// cockroachdb/apd, shopspring/decimal — see SPEC_REPORT.md §2 "0.1+0.2
// problem") flips the expected string and the conversion is verified
// by this test passing against the new payload.
//
// Until that port lands, the contract is: aqleng's TDecimal IS a
// float64. The engine does not silently round, does not display-trim,
// and does not pretend the result is 0.3. Honest arithmetic, ugly
// output for this specific input.
func TestDecimalArithmeticIsFloat64(t *testing.T) {
	a, _ := NewDecimal(0.1).AsDecimal()
	b, _ := NewDecimal(0.2).AsDecimal()
	got := a + b

	// Surface form: render via formatDecimal and assert the canonical
	// float artefact string.

	// Pin the surface form: render via formatDecimal and assert the
	// canonical float artefact string. If a future port to exact
	// decimal arithmetic lands, this row flips to "0.3".
	//
	// Note: we deliberately compare against a STRING, not a float
	// expression like `0.1 + 0.2`. Go's compile-time constant folding
	// evaluates `0.1 + 0.2` with arbitrary-precision rationals (yielding
	// exact 0.3), so a literal-arithmetic comparison would mislead the
	// reader into thinking the engine somehow rounds. The runtime
	// float64 sum is 0.30000000000000004; the string pins that.
	const wantStr = "0.30000000000000004"
	if gotStr := formatDecimal(got); gotStr != wantStr {
		t.Errorf("formatDecimal(0.1 + 0.2) = %q, want %q", gotStr, wantStr)
	}

	// Cross-check: parsing 0.30000000000000004 back gives the same
	// float64 — i.e. the rendered form is round-trip safe. This is
	// the critical property that separates "ugly but honest" output
	// from "rounded but lossy" display.
	parsed, err := strconv.ParseFloat(wantStr, 64)
	if err != nil {
		t.Fatalf("ParseFloat: %v", err)
	}
	if parsed != got {
		t.Errorf("round-trip broken: ParseFloat(%q) = %v, want %v", wantStr, parsed, got)
	}

	// And: 0.1 + 0.2 is NOT equal to 0.3 in float64. Pin the inequality
	// so the day someone "fixes" arithmetic this test surfaces the
	// semantic shift loudly.
	if got == 0.3 {
		t.Error("0.1 + 0.2 == 0.3 — float64 arithmetic should NOT produce this; if it does, the engine has changed payload type and the spec/SPEC_REPORT.md plan needs to land")
	}
}
