package eng

import (
	"math"
	"strconv"
	"testing"
)

// FormatFloat keeps the everyday range in plain decimal and only switches
// to scientific notation for genuinely extreme magnitudes (>= 1e21 or
// < 1e-10). Special values render as the parseable inf/-inf/nan literals.
// See design/IEEE-754-COMPLIANCE.8.md.
func TestFormatFloat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		// Special values.
		{math.NaN(), "nan"},
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
		// Everyday range — plain decimal, with the guaranteed ".0".
		{0.1, "0.1"},
		{2.5, "2.5"},
		{150.0, "150.0"},
		{1000000.0, "1000000.0"},
		{0.0001, "0.0001"},
		{0.00001, "0.00001"},
		{0.0000001, "0.0000001"},
		{0.30000000000000004, "0.30000000000000004"},
		{-3.5, "-3.5"},
		{0.0, "0.0"},
		// Boundaries — still plain (just inside the cutoffs).
		{1e20, "100000000000000000000.0"},
		{1e-10, "0.0000000001"},
		// Extremes — scientific.
		{1e21, "1e+21"},
		{1.5e30, "1.5e+30"},
		{1e-11, "1e-11"},
		{5e-324, "5e-324"},
		{math.MaxFloat64, "1.7976931348623157e+308"},
	}
	for _, c := range cases {
		if got := FormatFloat(c.in); got != c.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFormatFloatRoundTrips checks that every FormatFloat output re-parses
// (via ParseFloat, the same routine the literal path uses) to the original
// bits — both the plain and the scientific forms.
func TestFormatFloatRoundTrips(t *testing.T) {
	for _, f := range []float64{
		0.1, 2.5, 0.00001, 1e20, 1e-10, 1e21, 1.5e30, 1e-11, 5e-324, math.MaxFloat64,
	} {
		s := FormatFloat(f)
		g, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Errorf("FormatFloat(%v)=%q does not re-parse: %v", f, s, err)
			continue
		}
		if g != f {
			t.Errorf("round-trip %v → %q → %v", f, s, g)
		}
	}
}
