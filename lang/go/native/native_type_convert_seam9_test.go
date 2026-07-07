package native

import (
	"math"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// W9_nativeB — coverage for native_type.go convert* handlers and helpers.
// Drives the scalar-conversion error/edge arms directly (crafted sealed
// values), pairing positive results with the rejected shapes. See
// design/TEST-SEAMS.10.md.

func w9BigDec(t *testing.T, s string) Value {
	t.Helper()
	d, _, err := apd.NewFromString(s)
	if err != nil {
		t.Fatalf("apd parse %q: %v", s, err)
	}
	return NewBigDecimal(d)
}

// --- convertTo direct arms ---

func TestW9ConvertToFloatFromHugeBigDecimal(t *testing.T) {
	// AsFloatApprox errors: a BigDecimal beyond float64 range.
	if _, err := convertTo(w9BigDec(t, "1e100000"), TFloat, ""); err == nil {
		t.Fatal("convert huge BigDecimal to Float must error")
	}
	// Positive pair: a small BigDecimal converts fine.
	out, err := convertTo(w9BigDec(t, "1.5"), TFloat, "")
	if err != nil {
		t.Fatalf("convert 1.5 to Float: %v", err)
	}
	if f, _ := AsFloat(out); f != 1.5 {
		t.Fatalf("want 1.5, got %v", f)
	}
}

func TestW9ConvertToIntegerFromNaNBigDecimal(t *testing.T) {
	// bigDecimalToBigIntTrunc errors on a NaN BigDecimal (SetString fails).
	if _, err := convertTo(w9BigDec(t, "NaN"), TInteger, ""); err == nil {
		t.Fatal("convert NaN BigDecimal to Integer must error")
	}
}

func TestW9ConvertToIntegerFromInteger(t *testing.T) {
	// Integer passthrough (base "" fast path).
	out, err := convertTo(NewInteger(5), TInteger, "")
	if err != nil {
		t.Fatalf("convert Integer→Integer: %v", err)
	}
	if n, _ := AsInteger(out); n != 5 {
		t.Fatalf("want 5, got %d", n)
	}
}

func TestW9ConvertToIntegerFloatOverflow(t *testing.T) {
	for _, f := range []float64{math.NaN(), math.Inf(1), 1e30} {
		if _, err := convertTo(NewFloat(f), TInteger, ""); err == nil {
			t.Fatalf("convert Float %v to Integer must overflow-error", f)
		}
	}
	// Positive pair: an in-range float truncates.
	out, err := convertTo(NewFloat(3.9), TInteger, "")
	if err != nil {
		t.Fatalf("convert 3.9 to Integer: %v", err)
	}
	if n, _ := AsInteger(out); n != 3 {
		t.Fatalf("want 3, got %d", n)
	}
}

func TestW9ConvertToBooleanDefaultArm(t *testing.T) {
	// A non-Boolean/Number/String source hits the CoerceBoolean default.
	out, err := convertTo(NewAtom("x"), TBoolean, "")
	if err != nil {
		t.Fatalf("convert Atom to Boolean: %v", err)
	}
	if !out.Parent.Equal(TBoolean) {
		t.Fatalf("want Boolean, got %s", out.Parent)
	}
}

func TestW9ConvertToUnsupportedTarget(t *testing.T) {
	// A target the switch does not handle → unsupported-target error.
	if _, err := convertTo(NewInteger(1), TFunction, ""); err == nil {
		t.Fatal("convert to Function must be unsupported")
	}
}

// --- convertToBigInteger ---

func TestW9ConvertToBigIntegerBigDecimalTruncError(t *testing.T) {
	if _, err := convertToBigInteger(w9BigDec(t, "NaN"), ""); err == nil {
		t.Fatal("convertToBigInteger NaN must error")
	}
}

func TestW9ConvertToBigIntegerStringBases(t *testing.T) {
	cases := []struct {
		text, base string
		want       int64
	}{
		{"ff", "hex", 255},
		{"101", "bin", 5},
		{"17", "oct", 15},
	}
	for _, c := range cases {
		out, err := convertToBigInteger(NewString(c.text), c.base)
		if err != nil {
			t.Fatalf("convertToBigInteger %q base %q: %v", c.text, c.base, err)
		}
		n, _ := AsBigInteger(out)
		if n.Int64() != c.want {
			t.Fatalf("%q base %q: want %d, got %s", c.text, c.base, c.want, n)
		}
	}
	// Negative: unknown base rejected.
	if _, err := convertToBigInteger(NewString("5"), "zonk"); err == nil {
		t.Fatal("convertToBigInteger unknown base must error")
	}
}

// --- convertToBigDecimal ---

func TestW9ConvertToBigDecimalIdentity(t *testing.T) {
	out, err := convertToBigDecimal(w9BigDec(t, "2.5"), "")
	if err != nil {
		t.Fatalf("convertToBigDecimal identity: %v", err)
	}
	if !out.Parent.ConformsTo(TBigDecimal) {
		t.Fatalf("want BigDecimal, got %s", out.Parent)
	}
}

// --- bigDecimalToBigIntTrunc ---

func TestW9BigDecimalToBigIntTruncNonBigDecimal(t *testing.T) {
	if _, err := bigDecimalToBigIntTrunc(NewInteger(5)); err == nil {
		t.Fatal("bigDecimalToBigIntTrunc of a non-BigDecimal must error")
	}
}

func TestW9BigDecimalToBigIntTruncNegHalf(t *testing.T) {
	// "-0.5" truncates its fraction to "-", normalised to "0".
	n, err := bigDecimalToBigIntTrunc(w9BigDec(t, "-0.5"))
	if err != nil {
		t.Fatalf("bigDecimalToBigIntTrunc -0.5: %v", err)
	}
	if n.Sign() != 0 {
		t.Fatalf("want 0, got %s", n)
	}
}

func TestW9BigDecimalToBigIntTruncNaN(t *testing.T) {
	// NaN text has no integer form → SetString fails.
	if _, err := bigDecimalToBigIntTrunc(w9BigDec(t, "NaN")); err == nil {
		t.Fatal("bigDecimalToBigIntTrunc NaN must error")
	}
}

// --- convertIdealHandler ---

func TestW9ConvertIdealHandlerDefaultTarget(t *testing.T) {
	r := seam5Reg(t)
	// A bare Node literal is neither Map nor List → default rejection.
	_, err := convertIdealHandler([]Value{NewTypeLiteral(TNode), NewMap(NewOrderedMap())}, nil, nil, r)
	if err == nil {
		t.Fatal("convert Node <ideal> must reject (only Map or List)")
	}
	if !strings.Contains(err.Error(), "Map or List") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- convert2Handler / convert3Handler concrete-first-arg guards ---

func TestW9Convert2HandlerConcreteFirstArg(t *testing.T) {
	r := seam5Reg(t)
	if _, err := convert2Handler([]Value{NewInteger(5), NewInteger(6)}, nil, nil, r); err == nil {
		t.Fatal("convert2: concrete first arg (not a type literal) must error")
	}
}

func TestW9Convert3HandlerConcreteFirstArg(t *testing.T) {
	r := seam5Reg(t)
	opts := NewMap(NewOrderedMap())
	if _, err := convert3Handler([]Value{NewInteger(5), opts, NewInteger(6)}, nil, nil, r); err == nil {
		t.Fatal("convert3: concrete first arg (not a type literal) must error")
	}
}
