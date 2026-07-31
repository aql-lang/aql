package test

import (
	"strings"
	"testing"

	"github.com/boru-lang/boru/eng/go/parser"
	"github.com/boru-lang/boru/lang/go/native"
)

// TestPureBORUSortedFlexMap is the acceptance pin for pure-BORU sorted
// nodes (FLEX-ATTRS.1 D4) over the three enablers: nominal refine of
// FlexMap, an anchored `set` override, and `as` dispatch ascription
// (design/OPEN-WORDS.1.md §9) — the override delegates to the base
// overloads in CORE vocabulary (`set k v (m as FlexMap)`), with no
// renamed shadow words, and maintains sorted key order across
// out-of-order in-place writes, entirely in BORU source.
//
// Runs on the interpreter: the resort loop's dynamic list-element key
// feeding the now-multi-overload `set` is outside today's compiled
// dispatch-commit boundary (the emitter statically commits one overload
// or declines — a dynamic operand over a word carrying BOTH native and
// merged user signatures has two live candidates), so the compiled twin
// is deferred with that boundary — the residual documented in
// OPEN-WORDS.1 §9. The compilable delegation shapes (strict-key
// override, helper delegation) are pinned by lang/spec/as.tsv §5.
func TestPureBORUSortedFlexMap(t *testing.T) {
	const src = `
def SortedFlexMap (refine FlexMap)
def resort fn [[ks:List m2:SortedFlexMap] [SortedFlexMap] [
  if (0 eq (size ks)) [m2] [
    def k2 (convert String (ks get 0))
    def tmp (m2 get k2)
    (del (k2) (m2 as FlexMap)) drop
    (set (k2) tmp (m2 as FlexMap)) drop
    resort (ks slice 1 (size ks)) m2
  ]
]]
def set fn [[k:Atom/q v:Any m:SortedFlexMap] [SortedFlexMap] [
  (set (k) v (m as FlexMap)) drop
  (resort (sort (keys m)) m) drop
  m
]]
def w:SortedFlexMap (flex { })
(set b/q 2 w) drop
(set a/q 1 w) drop
(set c/q 3 w) drop
[(keys w) (typeof w) w.b]
`
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	reg.SetParseFunc(parser.Parse)
	vals, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := native.New(reg).Run(vals)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := native.Canon(out[len(out)-1:])
	if !strings.Contains(got, "['a' 'b' 'c']") {
		t.Errorf("keys not maintained sorted: %s", got)
	}
	if !strings.Contains(got, "SortedFlexMap") {
		t.Errorf("nominal tag lost: %s", got)
	}
	if !strings.Contains(got, "2]") {
		t.Errorf("value unreadable after resort: %s", got)
	}
}
