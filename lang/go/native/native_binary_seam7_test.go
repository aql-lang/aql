package native

import (
	"strings"
	"testing"
)

// Wave-7 coverage for native_binary.go: intPair's two accessor-error
// arms and every bitwise handler's error propagation, plus the
// shift-count guards (design/TEST-SEAMS.10.md). A bare type literal
// (Data==nil) is the panic-guard input the AsConcreteInteger accessor
// must reject rather than crash on.

func binHandlers() map[string]Handler {
	return map[string]Handler{
		"band": bandHandler,
		"bor":  borHandler,
		"bxor": bxorHandler,
		"bsl":  bslHandler,
		"bsr":  bsrHandler,
		"busr": busrHandler,
	}
}

func TestBinaryHandlersRejectNonInteger(t *testing.T) {
	r := b2Reg(t)
	lit := NewTypeLiteral(TInteger) // Data==nil panic guard
	for name, h := range binHandlers() {
		// Bad rhs (args[0]) — intPair's first accessor arm.
		if _, err := h([]Value{lit, NewInteger(2)}, nil, nil, r); err == nil {
			t.Fatalf("%s with type-literal rhs: expected error", name)
		}
		// Bad lhs (args[1]) — intPair's second accessor arm.
		if _, err := h([]Value{NewInteger(2), NewString("x")}, nil, nil, r); err == nil {
			t.Fatalf("%s with string lhs: expected error", name)
		}
		// Positive pair: two integers succeed.
		out, err := h([]Value{NewInteger(2), NewInteger(8)}, nil, nil, r)
		if err != nil || len(out) != 1 || !out[0].Is(TInteger) {
			t.Fatalf("%s(8,2) = %v / %v, want one Integer", name, out, err)
		}
	}
}

func TestBnotHandlerRejectsNonInteger(t *testing.T) {
	r := b2Reg(t)
	if _, err := bnotHandler([]Value{NewTypeLiteral(TInteger)}, nil, nil, r); err == nil {
		t.Fatal("bnot with type literal: expected error")
	}
	// Positive pair.
	out, err := bnotHandler([]Value{NewInteger(0)}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("bnot(0) = %v / %v", out, err)
	}
	if n, _ := AsInteger(out[0]); n != -1 {
		t.Fatalf("bnot(0) = %d, want -1", n)
	}
}

func TestIntPairAccessorArms(t *testing.T) {
	// rhs (args[0]) error arm.
	if _, _, err := intPair([]Value{NewString("x"), NewInteger(1)}); err == nil {
		t.Fatal("intPair(bad rhs): expected error")
	}
	// lhs (args[1]) error arm.
	if _, _, err := intPair([]Value{NewInteger(1), NewTypeLiteral(TInteger)}); err == nil {
		t.Fatal("intPair(bad lhs): expected error")
	}
	// Positive pair: sig order is [count, value] → lhs=value, rhs=count.
	lhs, rhs, err := intPair([]Value{NewInteger(2), NewInteger(8)})
	if err != nil || lhs != 8 || rhs != 2 {
		t.Fatalf("intPair(2,8) = %d,%d / %v, want lhs=8 rhs=2", lhs, rhs, err)
	}
}

func TestShiftCountGuards(t *testing.T) {
	r := b2Reg(t)
	// Negative counts error on every shift word (args[0] is the count).
	for _, name := range []string{"bsl", "bsr", "busr"} {
		h := binHandlers()[name]
		if _, err := h([]Value{NewInteger(-1), NewInteger(4)}, nil, nil, r); err == nil ||
			!strings.Contains(err.Error(), "non-negative") {
			t.Fatalf("%s(-1 count): expected non-negative error, got %v", name, err)
		}
	}
	// n >= 64 saturation. bsl/busr → 0; bsr → 0 for non-negative, -1 for negative.
	if out, _ := bslHandler([]Value{NewInteger(64), NewInteger(1)}, nil, nil, r); mustInt(t, out) != 0 {
		t.Fatal("bsl by 64 should saturate to 0")
	}
	if out, _ := busrHandler([]Value{NewInteger(64), NewInteger(-1)}, nil, nil, r); mustInt(t, out) != 0 {
		t.Fatal("busr by 64 should be 0")
	}
	if out, _ := bsrHandler([]Value{NewInteger(64), NewInteger(-1)}, nil, nil, r); mustInt(t, out) != -1 {
		t.Fatal("bsr of negative by 64 should be -1")
	}
	if out, _ := bsrHandler([]Value{NewInteger(64), NewInteger(8)}, nil, nil, r); mustInt(t, out) != 0 {
		t.Fatal("bsr of positive by 64 should be 0")
	}
}

func mustInt(t *testing.T, out []Value) int64 {
	t.Helper()
	if len(out) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out))
	}
	n, err := AsInteger(out[0])
	if err != nil {
		t.Fatalf("not an integer: %v", err)
	}
	return n
}
