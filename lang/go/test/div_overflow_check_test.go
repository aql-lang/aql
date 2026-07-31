package test

import (
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// MinInt64 div -1 is the one int64 division overflow (Go wraps it back
// to MinInt64). The runtime guard raises integer_overflow like the
// add/sub/mul/pow overflows (NUR006); the check-mode mirror
// (returnsIntFaultOn + divIntFault) must flag the same program
// statically with the byte-identical detail.

func TestCheckDivOverflowMirror(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := a.Check(`-9223372036854775808 div -1`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Code == "integer_overflow" && strings.Contains(d.Detail, "integer overflow in div") {
			found = true
		}
	}
	if !found {
		t.Errorf("check did not flag minint div -1; diagnostics: %+v", res.Diagnostics)
	}
}

// Negative twins: an in-range division and the mod sibling (MinInt64
// mod -1 is defined as 0 in Go — no overflow case exists for mod) must
// stay clean, so the mirror never over-flags.
func TestCheckDivNoFalseOverflow(t *testing.T) {
	a, err := lang.New()
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, src := range []string{
		`10 div 2`,
		`-9223372036854775808 div 1`,
		`-9223372036854775808 mod -1`,
	} {
		res, err := a.Check(src)
		if err != nil {
			t.Fatalf("check %q: %v", src, err)
		}
		for _, d := range res.Diagnostics {
			if d.Code == "integer_overflow" {
				t.Errorf("check %q wrongly flagged integer_overflow: %+v", src, d)
			}
		}
	}
}
