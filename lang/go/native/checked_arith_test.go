package native

import (
	"math"
	"testing"
)

// TestCheckedIntArithmetic pins the boundary behaviour of the checked
// integer helpers used by add/sub/mul/pow (WAT Exhibit K, Phase 0b).
// Each helper must return ok=false exactly on int64 overflow and the
// correct result otherwise — never a silent two's-complement wrap.
func TestCheckedAddInt(t *testing.T) {
	cases := []struct {
		a, b    int64
		want    int64
		wantOk  bool
		comment string
	}{
		{1, 2, 3, true, "in range"},
		{-5, 2, -3, true, "mixed signs never overflow"},
		{math.MaxInt64, 0, math.MaxInt64, true, "max + 0"},
		{math.MaxInt64 - 1, 1, math.MaxInt64, true, "lands exactly on max"},
		{math.MaxInt64, 1, 0, false, "max + 1 overflows"},
		{math.MinInt64, -1, 0, false, "min + -1 underflows"},
		{math.MinInt64, 0, math.MinInt64, true, "min + 0"},
	}
	for _, c := range cases {
		got, ok := checkedAddInt(c.a, c.b)
		if ok != c.wantOk || (ok && got != c.want) {
			t.Errorf("checkedAddInt(%d,%d)=%d,%v want %d,%v (%s)",
				c.a, c.b, got, ok, c.want, c.wantOk, c.comment)
		}
	}
}

func TestCheckedSubInt(t *testing.T) {
	cases := []struct {
		a, b   int64
		want   int64
		wantOk bool
	}{
		{10, 3, 7, true},
		{3, 10, -7, true},
		{math.MinInt64, 1, 0, false},  // underflow
		{math.MaxInt64, -1, 0, false}, // overflow
		{math.MinInt64, 0, math.MinInt64, true},
		{math.MaxInt64, 0, math.MaxInt64, true},
		{-1, math.MinInt64, math.MaxInt64, true}, // -1 - minint = maxint, in range
	}
	for _, c := range cases {
		got, ok := checkedSubInt(c.a, c.b)
		if ok != c.wantOk || (ok && got != c.want) {
			t.Errorf("checkedSubInt(%d,%d)=%d,%v want %d,%v",
				c.a, c.b, got, ok, c.want, c.wantOk)
		}
	}
}

func TestCheckedMulInt(t *testing.T) {
	cases := []struct {
		a, b   int64
		want   int64
		wantOk bool
	}{
		{4, 5, 20, true},
		{-2, -3, 6, true},
		{0, math.MaxInt64, 0, true},
		{math.MaxInt64, 0, 0, true},
		{4000000000, 4000000000, 0, false}, // the WAT mul case
		{math.MinInt64, -1, 0, false},      // no positive counterpart
		{-1, math.MinInt64, 0, false},      // commuted
		{math.MinInt64, 1, math.MinInt64, true},
		{1000000, 1000000, 1000000000000, true},
	}
	for _, c := range cases {
		got, ok := checkedMulInt(c.a, c.b)
		if ok != c.wantOk || (ok && got != c.want) {
			t.Errorf("checkedMulInt(%d,%d)=%d,%v want %d,%v",
				c.a, c.b, got, ok, c.want, c.wantOk)
		}
	}
}

func TestCheckedPowInt(t *testing.T) {
	cases := []struct {
		base, exp int64
		want      int64
		wantOk    bool
	}{
		{2, 0, 1, true},
		{2, 10, 1024, true},
		{2, 62, 4611686018427387904, true}, // largest in-range power of two
		{2, 63, 0, false},                  // the WAT pow case — overflow
		{10, 18, 1000000000000000000, true},
		{10, 19, 0, false}, // overflow
		{1, 1000, 1, true}, // 1**anything never overflows
		{0, 5, 0, true},
	}
	for _, c := range cases {
		got, ok := checkedPowInt(c.base, c.exp)
		if ok != c.wantOk || (ok && got != c.want) {
			t.Errorf("checkedPowInt(%d,%d)=%d,%v want %d,%v",
				c.base, c.exp, got, ok, c.want, c.wantOk)
		}
	}
}
