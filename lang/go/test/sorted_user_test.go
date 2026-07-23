package test

import (
	"strings"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestPureAQLSortedFlexMap is the acceptance pin for the three
// pure-AQL sorted-node enablers (design/OPEN-WORDS.1.md follow-ups,
// FLEX-ATTRS.1 D4): a USER module shape — nominal refine of FlexMap,
// an anchored `set` override, and `super` delegation — maintains
// sorted key order across out-of-order in-place writes, entirely in
// AQL source. Runs on the interpreter: the resort body's get-result
// (a dynamic value) feeding the delegated call is outside today's
// compiled fn-value boundary, so the compiled twin is deferred with
// that boundary (noted in OPEN-WORDS.1 §7).
func TestPureAQLSortedFlexMap(t *testing.T) {
	const src = `
def SortedFlexMap (refine FlexMap)
def bset (super set)
def bdel (super del)
def reslot fn [[m2:SortedFlexMap k2:String] [SortedFlexMap] [
  def tmp (m2 get k2)
  (bdel k2 m2) drop
  (bset k2 tmp m2) drop
  m2
]]
def set fn [[k:Atom/q v:Any m:SortedFlexMap] [SortedFlexMap] [
  (bset k v m) drop
  ((sort (keys m)) each [m reslot]) drop
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
