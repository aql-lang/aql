package native

import (
	"strings"
	"testing"
)

// Seam-7 coverage for join.go and pad.go word handlers
// (design/TEST-SEAMS.10.md). Each error arm is driven with a crafted
// argument and paired with a positive call.

func TestSeam7JoinDefaultHandler(t *testing.T) {
	r := seam5Reg(t)

	// Positive: a list joins with the default comma separator.
	lst := NewList([]Value{NewString("a"), NewString("b")})
	out, err := joinDefaultHandler([]Value{lst}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("joinDefaultHandler positive: %v / %v", out, err)
	}
	if s, _ := AsString(out[0]); s != "a,b" {
		t.Fatalf("expected a,b got %q", s)
	}

	// Negative: a non-list arg is rejected.
	if _, err := joinDefaultHandler([]Value{NewInteger(7)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected list") {
		t.Fatalf("expected join list error, got %v", err)
	}
}

func TestSeam7JoinSepHandler(t *testing.T) {
	r := seam5Reg(t)
	lst := NewList([]Value{NewString("a"), NewString("b")})

	// Positive: explicit separator.
	out, err := joinSepHandler([]Value{NewString("-"), lst}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("joinSepHandler positive: %v / %v", out, err)
	}
	if s, _ := AsString(out[0]); s != "a-b" {
		t.Fatalf("expected a-b got %q", s)
	}

	// Negative: separator not a concrete string.
	if _, err := joinSepHandler([]Value{NewInteger(1), lst}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "separator") {
		t.Fatalf("expected separator error, got %v", err)
	}

	// Negative: data not a list.
	if _, err := joinSepHandler([]Value{NewString("-"), NewInteger(9)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "expected list") {
		t.Fatalf("expected join list error, got %v", err)
	}
}

func TestSeam7PadHandlers(t *testing.T) {
	r := seam5Reg(t)
	lst := NewList([]Value{NewString("a")})

	// Positive: default width.
	if _, err := padDefaultHandler([]Value{lst}, nil, nil, r); err != nil {
		t.Fatalf("padDefaultHandler positive: %v", err)
	}

	// Positive: explicit width.
	if _, err := padWidthHandler([]Value{NewInteger(3), lst}, nil, nil, r); err != nil {
		t.Fatalf("padWidthHandler positive: %v", err)
	}

	// Negative: width not a concrete integer.
	if _, err := padWidthHandler([]Value{NewString("x"), lst}, nil, nil, r); err == nil {
		t.Fatal("expected pad width integer error")
	}
}
