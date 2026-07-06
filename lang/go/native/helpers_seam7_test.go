package native

import (
	"strings"
	"testing"
)

// Wave-7 coverage for helpers.go: valueToSliceArg's scalar fallback and
// the full sliceResult / sliceResultItem conversion switches, including
// their error arms (design/TEST-SEAMS.10.md).

func TestSliceArgScalarFallback(t *testing.T) {
	// Non-string, non-list values take the String() fallback branch.
	if got := valueToSliceArg(NewInteger(7)); got != "7" {
		t.Fatalf("valueToSliceArg(7) = %#v, want \"7\"", got)
	}
	// Positive pairs: a string flows through unchanged, a list boxes.
	if got := valueToSliceArg(NewString("hi")); got != "hi" {
		t.Fatalf("valueToSliceArg(\"hi\") = %#v, want \"hi\"", got)
	}
	lst := valueToSliceArg(NewList([]Value{NewInteger(1), NewInteger(2)}))
	boxed, ok := lst.([]interface{})
	if !ok || len(boxed) != 2 {
		t.Fatalf("valueToSliceArg(list) = %#v, want 2-elem []interface{}", lst)
	}
}

func TestSliceResultCases(t *testing.T) {
	// nil → None type literal.
	out, err := sliceResult(nil)
	if err != nil || len(out) != 1 || !out[0].Is(TNone) {
		t.Fatalf("sliceResult(nil) = %v / %v, want [None]", out, err)
	}

	// []interface{} with an unsupported element errors (propagated from
	// sliceResultItem's default arm).
	if _, err := sliceResult([]interface{}{uint(3)}); err == nil ||
		!strings.Contains(err.Error(), "unsupported element type") {
		t.Fatalf("sliceResult(unsupported elem) err = %v, want unsupported element", err)
	}

	// Unexpected top-level result type errors.
	if _, err := sliceResult(42); err == nil ||
		!strings.Contains(err.Error(), "unexpected result type") {
		t.Fatalf("sliceResult(42) err = %v, want unexpected result type", err)
	}

	// Positive pair: a valid []interface{} converts.
	out, err = sliceResult([]interface{}{"a", "b"})
	if err != nil || len(out) != 1 {
		t.Fatalf("sliceResult([a b]) = %v / %v", out, err)
	}
}

func TestSliceResultItemScalarCases(t *testing.T) {
	// A []interface{} that recurses through every scalar leaf case.
	v, err := sliceResultItem([]interface{}{"s", 5, int64(6), 3.14, true, nil})
	if err != nil {
		t.Fatalf("sliceResultItem(scalars) err = %v", err)
	}
	rl, aerr := AsList(v)
	if aerr != nil || rl.Len() != 6 {
		t.Fatalf("sliceResultItem(scalars) = %v, want 6-elem list", v)
	}
	got := rl.Slice()
	if !got[0].Is(TString) || !got[1].Is(TInteger) || !got[2].Is(TInteger) ||
		!got[3].Is(TFloat) || !got[4].Is(TBoolean) || !got[5].Is(TNone) {
		t.Fatalf("sliceResultItem scalar types wrong: %v", got)
	}
}

func TestSliceResultItemNestedAndErrors(t *testing.T) {
	// Nested []interface{} succeeds.
	v, err := sliceResultItem([]interface{}{[]interface{}{1, 2}})
	if err != nil {
		t.Fatalf("nested slice err = %v", err)
	}
	rl, aerr := AsList(v)
	if aerr != nil || rl.Len() != 1 {
		t.Fatalf("nested slice = %v", v)
	}

	// Error propagation up through the nested []interface{} arm.
	if _, err := sliceResultItem([]interface{}{[]interface{}{uint(9)}}); err == nil ||
		!strings.Contains(err.Error(), "unsupported element type") {
		t.Fatalf("nested unsupported err = %v", err)
	}

	// Top-level unsupported element hits the default arm directly.
	if _, err := sliceResultItem(uint(1)); err == nil ||
		!strings.Contains(err.Error(), "unsupported element type") {
		t.Fatalf("unsupported item err = %v", err)
	}
}
