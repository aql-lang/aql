package eng

import (
	"math"
	"sort"
	"testing"
)

// Integers above 2^53 cannot be represented exactly as float64. The
// comparison path used to project both operands through float64, which
// rounded distinct large integers to the same value and reported them
// equal — silently corrupting cmp / eq / sort. These tests pin the
// exact int64 comparison and guard the cross-leaf magnitude equivalence
// (1 == 1.0) that must NOT regress.

func TestLargeIntegerExactCompare(t *testing.T) {
	a := NewInteger(9007199254740993) // 2^53 + 1
	b := NewInteger(9007199254740992) // 2^53
	if float64(9007199254740993) != float64(9007199254740992) {
		t.Fatal("test premise wrong: these should collide as float64")
	}

	got, err := CompareValues(a, b)
	if err != nil {
		t.Fatalf("CompareValues: %v", err)
	}
	if got != 1 {
		t.Errorf("cmp(2^53+1, 2^53) = %d, want 1 (must not collapse via float64)", got)
	}

	if ExactEqual(a, b) {
		t.Error("ExactEqual(2^53+1, 2^53) = true, want false")
	}
	if !ExactEqual(a, NewInteger(9007199254740993)) {
		t.Error("ExactEqual(x, x) = false for large int, want true")
	}
}

func TestLargeIntegerSortOrder(t *testing.T) {
	vals := []Value{
		NewInteger(9007199254740993),
		NewInteger(9007199254740992),
		NewInteger(9007199254740991),
	}
	sort.SliceStable(vals, func(i, j int) bool {
		c, _ := CompareValues(vals[i], vals[j])
		return c < 0
	})
	want := []int64{9007199254740991, 9007199254740992, 9007199254740993}
	for i, w := range want {
		got, _ := AsInteger(vals[i])
		if got != w {
			t.Errorf("sorted[%d] = %d, want %d", i, got, w)
		}
	}
}

func TestCrossLeafMagnitudeEqualityPreserved(t *testing.T) {
	// The int64-exact fix must not break Integer/Float magnitude equality.
	c, err := CompareValues(NewInteger(1), NewFloat(1.0))
	if err != nil {
		t.Fatalf("CompareValues: %v", err)
	}
	if c != 0 {
		t.Errorf("cmp(1, 1.0) = %d, want 0", c)
	}
	if !ExactEqual(NewInteger(1), NewFloat(1.0)) {
		t.Error("ExactEqual(1, 1.0) = false, want true")
	}
}

func TestNumberSizeNonFinite(t *testing.T) {
	// Size of a non-finite Float must not be int(math.Floor(NaN/Inf)),
	// which is implementation-defined garbage.
	beh := numberCompareBehavior{}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := beh.Size(NewFloat(f)); got != 0 {
			t.Errorf("Size(%v) = %d, want 0", f, got)
		}
	}
	if got := beh.Size(NewFloat(7.9)); got != 7 {
		t.Errorf("Size(7.9) = %d, want 7", got)
	}
	if got := beh.Size(NewInteger(42)); got != 42 {
		t.Errorf("Size(42) = %d, want 42", got)
	}
}
