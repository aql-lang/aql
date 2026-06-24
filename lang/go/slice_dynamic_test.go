package lang_test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// A word with MIXED-ARITY overloads (e.g. `slice`: 1/2/3-arg forms) dispatched
// on a gradual receiver must refine its result to the union of the reachable
// SAME-arity returns, not commit to the single matched overload. Over a dynamic
// receiver `slice` could return a String or a List; committing to one (e.g.
// dynamic(List)) makes a String consumer of the result fail no_signature. The
// sliced value is a get-result of unknown type in radix's edge splitter.
func TestSliceDynamicReceiverRefines(t *testing.T) {
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
	// slice over a dynamic-Any value, result consumed by a String param.
	clean := `def af fn [[x:Any] [Any] [x]] end def g fn [[s:String] [Integer] [1]] end def f fn [[s:String] [Integer] [(((af s) slice 0 1) g)]] end`
	if n := errCount(clean); n != 0 {
		t.Errorf("slice-over-dynamic result -> String param: expected 0 errors, got %d", n)
	}
	// And consumed by a List param (the other reachable return) — also clean.
	clean2 := `def af fn [[x:Any] [Any] [x]] end def g fn [[xs:List] [Integer] [1]] end def f fn [[s:String] [Integer] [(((af s) slice 0 1) g)]] end`
	if n := errCount(clean2); n != 0 {
		t.Errorf("slice-over-dynamic result -> List param: expected 0 errors, got %d", n)
	}
	// NEGATIVE: the union is String|List, so an Integer consumer is still rejected.
	bad := `def af fn [[x:Any] [Any] [x]] end def g fn [[n:Integer] [Integer] [1]] end def f fn [[s:String] [Integer] [(((af s) slice 0 1) g)]] end`
	if n := errCount(bad); n == 0 {
		t.Errorf("slice result (String|List) -> Integer param must still be flagged")
	}
}
