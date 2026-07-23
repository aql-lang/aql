package lang

import (
	"strings"
	"testing"
)

// A `flex {}` local carried into a narrowed branch used to crash the
// checker: the store-shaped carrier reached valuesEqualDefault's Map
// branch, which walked a nil ReadMap (see equal_shape_test.go). The
// checker must now survive it — clean, no panic.
func TestCheckShapedFlexGuardNarrowingNoPanic(t *testing.T) {
	src := `def f fn [[x:Any] [Any] [
  def m (flex {})
  if (m is None) [1] [2]
]]
f 1`
	if hasErr, codes := checkDiags(t, src); hasErr {
		t.Errorf("shaped-flex narrowing flagged: %s\n%q", codes, src)
	}
}

// A `/r` reference over a DYNAMIC (Any-typed) local that plausibly holds
// a Function at runtime must model as a Function carrier and check clean
// (the apply path). A `/r` over a binding that PROVABLY cannot be a
// function — a concrete plain value, or a carrier of a disjoint concrete
// type — must still raise illegal_ref.
func TestCheckDynamicRefApply(t *testing.T) {
	// POSITIVE: fw is Any, f = (fw get 0) is dynamic → f/r apply is fine.
	pos := `def g fn [[x:Integer] [Integer] [x add 1]]
def use fn [[fw:Any] [Any] [
  def f (fw get 0)
  def r (5 f/r apply)
  r
]]
use [g/r]`
	if hasErr, codes := checkDiags(t, pos); hasErr {
		t.Errorf("dynamic /r apply flagged: %s\n%q", codes, pos)
	}
	if _, codes := checkDiags(t, pos); strings.Contains(codes, "illegal_ref") {
		t.Errorf("dynamic /r apply raised illegal_ref: %s", codes)
	}

	// NEGATIVE: f is a concrete Integer → f/r is provably illegal.
	neg1 := `def use2 fn [[x:Any] [Any] [
  def f 5
  f/r
]]
use2 1`
	if _, codes := checkDiags(t, neg1); !strings.Contains(codes, "illegal_ref") {
		t.Errorf("concrete-value /r not flagged illegal_ref: %s", codes)
	}

	// NEGATIVE: f is a concrete Integer (computed) → still illegal.
	neg2 := `def use3 fn [[x:Integer] [Any] [
  def f (x add 1)
  f/r
]]
use3 1`
	if _, codes := checkDiags(t, neg2); !strings.Contains(codes, "illegal_ref") {
		t.Errorf("computed-Integer /r not flagged illegal_ref: %s", codes)
	}
}
