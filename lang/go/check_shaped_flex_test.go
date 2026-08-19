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

// A `/v` reference over a DYNAMIC (Any-typed) local that plausibly holds
// a Function at runtime must model as a Function carrier and check clean
// (the apply path). A `/v` over a binding that PROVABLY cannot be a
// function — a concrete plain value, or a carrier of a disjoint concrete
// type — must still raise illegal_ref.
func TestCheckDynamicRefApply(t *testing.T) {
	// POSITIVE: fw is Any, f = (fw get 0) is dynamic → f/v apply is fine.
	pos := `def g fn [[x:Integer] [Integer] [x add 1]]
def use fn [[fw:Any] [Any] [
  def f (fw get 0)
  def r (5 f/v apply)
  r
]]
use [g/v]`
	if hasErr, codes := checkDiags(t, pos); hasErr {
		t.Errorf("dynamic /v apply flagged: %s\n%q", codes, pos)
	}
	if _, codes := checkDiags(t, pos); strings.Contains(codes, "illegal_ref") {
		t.Errorf("dynamic /v apply raised illegal_ref: %s", codes)
	}

	// `/v` is TOTAL over binding kinds, so a concrete non-fn binding is
	// NOT a diagnostic any more — it is the identity. These two stay as
	// the guard that the old illegal_ref never comes back.
	pos1 := `def use2 fn [[x:Any] [Any] [
  def f 5
  f/v
]]
use2 1`
	if _, codes := checkDiags(t, pos1); strings.Contains(codes, "illegal_ref") {
		t.Errorf("concrete-value /v wrongly flagged illegal_ref: %s", codes)
	}

	pos2 := `def use3 fn [[x:Integer] [Any] [
  def f (x add 1)
  f/v
]]
use3 1`
	if _, codes := checkDiags(t, pos2); strings.Contains(codes, "illegal_ref") {
		t.Errorf("computed-Integer /v wrongly flagged illegal_ref: %s", codes)
	}
}
