package lang_test

import (
	"testing"

	lang "github.com/aql-lang/aql/lang/go"
)

// A value whose static type narrowed to a dynamic List/Map carrier (a fn result
// derived from a dynamic arg) must still dispatch `get`/`size`/etc. cleanly: the
// receiver is gradual, and `get`'s List/Map access goes through its `Node`
// receiver overload (List ⊑ Node), which the dynamic carrier must match. This
// is the trie node-walker shape that drove the `kid-items`/`get`/`build-row`
// no_signature cascade.
func TestDynamicContainerGetChecksClean(t *testing.T) {
	errs := func(src string) int {
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

	clean := []string{
		// fn returns [List]; called with a dynamic arg → dynamic(List); get over it.
		`def ml fn [[x:Any] [List] [[1 2 3]]] end def f fn [[a:Any] [Any] [(a ml) get 0]] end`,
		// dynamic(Map) get by string key.
		`def mm fn [[x:Any] [Map] [{a: 1}]] end def f fn [[a:Any] [Any] [(a mm) get "a"]] end`,
		// size/push over a dynamic(List) (sibling ops, already worked — guard them).
		`def ml fn [[x:Any] [List] [[1 2 3]]] end def f fn [[a:Any] [Integer] [(a ml) size]] end`,
	}
	for _, src := range clean {
		if n := errs(src); n != 0 {
			t.Errorf("expected 0 errors, got %d for: %s", n, src)
		}
	}

	// NEGATIVE: a provably wrong key/receiver is still rejected — gradual
	// looseness must not swallow a real mismatch on a CONCRETE value.
	if n := errs(`(5 get "x")`); n == 0 {
		t.Errorf("get over a concrete Integer must still be flagged")
	}
}
