package native

import (
	"math"
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/eng/go/parser"
)

// Coverage for the opt-in Float → BigDecimal conversion: the pure
// floatToBigDecimal helper (every accuracy arm + validation) and the
// convert3Handler branch that wires the `accuracy` / `places` options in.
// Positive results are paired with the rejected shapes throughout.

// --- floatToBigDecimal (pure helper) ---

func TestFloatToBigDecimalExact(t *testing.T) {
	// 0.5 is binary-exact; its exact decimal terminates at one place.
	out, err := floatToBigDecimal(0.5, "exact", 0, false)
	if err != nil {
		t.Fatalf("exact 0.5: %v", err)
	}
	if got := mustDec(t, out); got != "0d0.5" {
		t.Fatalf("exact 0.5 = %q, want 0d0.5", got)
	}
	// A whole-number float renders with no fractional part (k == 0).
	out, err = floatToBigDecimal(3.0, "exact", 0, false)
	if err != nil {
		t.Fatalf("exact 3.0: %v", err)
	}
	if got := mustDec(t, out); got != "0d3" {
		t.Fatalf("exact 3.0 = %q, want 0d3", got)
	}
	// 0.1 is NOT binary-exact — the exact value is the long dyadic tail.
	out, err = floatToBigDecimal(0.1, "exact", 0, false)
	if err != nil {
		t.Fatalf("exact 0.1: %v", err)
	}
	if got := mustDec(t, out); !strings.HasPrefix(got, "0d0.1000000000000000055511151231") {
		t.Fatalf("exact 0.1 = %q, want the long dyadic expansion", got)
	}
}

func TestFloatToBigDecimalShortest(t *testing.T) {
	out, err := floatToBigDecimal(3.14, "shortest", 0, false)
	if err != nil {
		t.Fatalf("shortest 3.14: %v", err)
	}
	if got := mustDec(t, out); got != "0d3.14" {
		t.Fatalf("shortest 3.14 = %q, want 0d3.14", got)
	}
}

func TestFloatToBigDecimalRound(t *testing.T) {
	out, err := floatToBigDecimal(3.14159, "round", 2, true)
	if err != nil {
		t.Fatalf("round 3.14159 places 2: %v", err)
	}
	if got := mustDec(t, out); got != "0d3.14" {
		t.Fatalf("round 3.14159 places 2 = %q, want 0d3.14", got)
	}
	// Rounding is half away from zero: 2.5 → 3 at 0 places.
	out, err = floatToBigDecimal(2.5, "round", 0, true)
	if err != nil {
		t.Fatalf("round 2.5 places 0: %v", err)
	}
	if got := mustDec(t, out); got != "0d3" {
		t.Fatalf("round 2.5 places 0 = %q, want 0d3", got)
	}
}

func TestFloatToBigDecimalRejections(t *testing.T) {
	cases := []struct {
		name             string
		f                float64
		accuracy         string
		places           int64
		placesSet        bool
		wantErrSubstring string
	}{
		{"nan", math.NaN(), "shortest", 0, false, "non-finite"},
		{"posinf", math.Inf(1), "exact", 0, false, "non-finite"},
		{"neginf", math.Inf(-1), "round", 2, true, "non-finite"},
		{"unknown-mode", 1.0, "bogus", 0, false, "unknown accuracy"},
		{"exact-with-places", 1.0, "exact", 2, true, "does not take a places option"},
		{"shortest-with-places", 1.0, "shortest", 2, true, "does not take a places option"},
		{"round-without-places", 1.0, "round", 0, false, "requires a places option"},
		{"round-negative-places", 1.0, "round", -1, true, "must be non-negative"},
		// The `places` cap: an impractically large request is rejected up
		// front rather than materialising a billion-digit string (DoS).
		{"round-places-too-large", 1.0, "round", maxFloatFractionDigits + 1, true, "exceeds the maximum"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := floatToBigDecimal(c.f, c.accuracy, c.places, c.placesSet)
			if err == nil {
				t.Fatalf("%s: expected error, got none", c.name)
			}
			if !strings.Contains(err.Error(), c.wantErrSubstring) {
				t.Fatalf("%s: error %q missing %q", c.name, err.Error(), c.wantErrSubstring)
			}
		})
	}
}

// TestFloatToBigDecimalSignedZero: exact and round preserve a negative
// sign bit that big.Rat drops for zero, staying consistent with shortest.
func TestFloatToBigDecimalSignedZero(t *testing.T) {
	negZero := math.Copysign(0, -1)
	cases := []struct {
		name      string
		f         float64
		accuracy  string
		places    int64
		placesSet bool
		want      string
	}{
		{"exact-neg-zero", negZero, "exact", 0, false, "-0d0"},
		{"round-neg-zero", negZero, "round", 2, true, "-0d0.00"},
		{"exact-pos-zero", 0.0, "exact", 0, false, "0d0"},
		// A negative value that rounds to zero keeps the IEEE sign too.
		{"round-neg-to-zero", -0.4, "round", 0, true, "-0d0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := floatToBigDecimal(c.f, c.accuracy, c.places, c.placesSet)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := mustDec(t, out); got != c.want {
				t.Fatalf("%s = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func mustDec(t *testing.T, v Value) string {
	t.Helper()
	d, err := AsBigDecimal(v)
	if err != nil {
		t.Fatalf("value is not a BigDecimal: %v", err)
	}
	return FormatBigDecimal(d)
}

// --- convert3Handler wiring (through the engine) ---

// runConvertAccuracy parses and runs src, returning the canon of the
// result or the error text (prefixed "ERR:").
func runConvertAccuracy(t *testing.T, src string) string {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	toks, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := NewTop(r).Run(toks)
	if err != nil {
		return "ERR:" + err.Error()
	}
	return eng.Canon(out)
}

func TestConvertAccuracyHandlerPaths(t *testing.T) {
	cases := []struct {
		src, want string
	}{
		// Positive: each accuracy mode reaches floatToBigDecimal.
		{"convert BigDecimal {accuracy:'shortest'} 3.14", "0d3.14"},
		{"convert BigDecimal {accuracy:'exact'} 0.5", "0d0.5"},
		{"convert BigDecimal {accuracy:'round' places:2} 3.14159", "0d3.14"},
		// Inert: a non-Float source keeps the ordinary widening.
		{"convert BigDecimal {accuracy:'exact'} 42", "0d42"},
		// Inert: a non-BigDecimal target ignores accuracy entirely.
		{"convert String {accuracy:'exact'} 42", "'42'"},
		// Negative: a dependent-type Float constraint is not concrete.
		{"convert BigDecimal {accuracy:'exact'} (Float gt 0.0)", "ERR:convert: Float source must be a concrete value"},
		// Negative: accuracy enables BigDecimal only, not BigInteger.
		{"convert BigInteger {accuracy:'exact'} 3.14", "ERR:convert: cannot convert Float to BigInteger"},
		// Negative: an unrecognised mode is rejected by the helper.
		{"convert BigDecimal {accuracy:'nope'} 3.14", "ERR:convert: unknown accuracy"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			if got := runConvertAccuracy(t, c.src); got != c.want {
				if !strings.HasPrefix(c.want, "ERR:") || !strings.HasPrefix(got, c.want) {
					t.Fatalf("%q = %q, want %q", c.src, got, c.want)
				}
			}
		})
	}
}
