package lang_test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// A fn body is analysed against its DECLARED param types, not the (possibly
// gradual) arg shapes a call threads in. A mutual-recursion cycle that passes a
// `get`-result `dynamic(Any)` into a `ch:List` param must still see `ch` as a
// List inside the body, so `each` over it matches the LIST overload (→ List)
// rather than the map-each overload (→ Map) — otherwise a downstream
// `all`/`any`/`[List]` consumer fails no_signature against the spurious Map.
// This is the decision client's `eval-pred-all` ↔ `eval-pred` cycle.
func TestParamNarrowingMutualCycle(t *testing.T) {
	errCount := func(src string) int {
		a, _ := lang.New()
		cr, _ := a.Check(src)
		n := 0
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				n++
			}
		}
		return n
	}

	// eval-pred-all calls each+all over eval-pred results; eval-pred recurses
	// back into eval-pred-all, threading a get-result as the children:List arg.
	clean := `
def eval-cond fn [[c:Map input:Map] [Boolean] [true]] end
def eval-pred-all fn [[children:List input:Map] [Boolean] [(children each [input swap eval-pred]) all]] end
def eval-pred-any fn [[children:List input:Map] [Boolean] [(children each [input swap eval-pred]) any]] end
def eval-pred fn [[pred:Map input:Map] [Boolean] [
  if ((pred get "kind") "group" eq)
    [input (pred get "children") eval-pred-all]
    [input pred eval-cond]
]] end`
	if n := errCount(clean); n != 0 {
		t.Errorf("mutual-cycle eval-pred-all/eval-pred: expected 0 errors, got %d", n)
	}

	// NEGATIVE: passing a genuinely wrong concrete type to a List param is still
	// flagged — narrowing only applies to a gradual arg that PASSED the match.
	bad := `def f fn [[xs:List] [Integer] [xs size]] end (f 5)`
	if n := errCount(bad); n == 0 {
		t.Errorf("passing a concrete Integer to a List param must still be flagged")
	}

	// A STRICT Any arg threaded into a declared-`Any` param must be analysed
	// gradually (the param is declared Any → ParamInputCarrier gives a dynamic
	// carrier). The fold-accumulator-as-node shape: an accumulator that started
	// `none` and was rebuilt into a Map is threaded into a `nd:Any` insert
	// receiver, whose body then reads it with `get`/rebuilds it. (tst/radix's
	// `none entries [(acc … ins)] fold`.)
	accAsNode := `
def mk fn [[a:Any b:Any] [Map] [do {x: [a], y: [b]}]] end
def wev fn [[fin:Any nd:Map] [Map] [(nd "x" get) (nd "y" get) mk]] end
def ins fn [[val:Any key:String nd:Any] [Any] [
  def nd (if (nd eq none) [ none none mk ] [nd])
  if ((key size) eq 0) [ (nd val wev) ] [nd]
]] end
def from-entries fn [[entries:List] [Any] [
  none entries [ var [[e acc] (acc (e get 0) (e get 1) ins) ] ] fold
]] end`
	if n := errCount(accAsNode); n != 0 {
		t.Errorf("fold-accumulator-as-node: expected 0 errors, got %d", n)
	}
}
