package lang_test

import (
	"fmt"
	"strings"
	"testing"

	lang "github.com/boru-lang/boru/lang/go"
)

// `set` over a DYNAMIC receiver has runtime-dependent return arity: the
// in-place mutators (Array/Object/Store/Class) return 0 values, the value-
// returning twins (Map/List/Flex) return the updated node. When the receiver
// is statically `Any` (a node pulled out with `get`) and the result is
// CONSUMED, modelling 0 values poisons the consumer — a false undefined_word on
// its bound param plus a no_signature on the consuming call. This is the
// trie client's `((nd "kids" get) set ch child) … mk-node` shape. In pure
// check the result is modelled as one dynamic value so the consuming use
// type-checks; the code runs correctly because the runtime receiver is a Map.
func TestSetOverDynamicReceiverChecksClean(t *testing.T) {
	errCodes := func(t *testing.T, src string) []string {
		t.Helper()
		a, err := lang.New()
		if err != nil {
			t.Fatal(err)
		}
		cr, _ := a.Check(src)
		var codes []string
		for _, d := range cr.Diagnostics {
			if d.Severity == "error" {
				codes = append(codes, d.Code)
			}
		}
		return codes
	}

	// The trie put-kid/mk-node shape: a set over a dynamic node, result consumed.
	clean := `
def mk fn [[fin:Any val:Any kids:Any] [Map] [do {e: [fin], v: [val], k: [kids]}]] end
def pk fn [[child:Map ch:String nd:Map] [Map] [
  ((nd "kids" get) set (ch) child) (nd "val" get) (nd "end" get) mk
]] end`
	if codes := errCodes(t, clean); len(codes) != 0 {
		t.Errorf("set-over-dynamic consumed: expected no errors, got %v", codes)
	}

	// And it RUNS correctly (the runtime receiver is a concrete Map).
	a, _ := lang.New()
	got, err := a.RunInterp(clean + `  ({kids: {}, val: 1, end: false} "a" {x: 9} pk)`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(fmt.Sprint(got), "k:") {
		t.Errorf("expected the rebuilt node, got %v", got)
	}

	// NEGATIVE: a CONCRETE-receiver in-place set (Object) genuinely returns 0
	// values; the relaxation must not fabricate a value there. Consuming the
	// (absent) result is a real error in both check and run.
	a2, _ := lang.New()
	_, runErr := a2.RunInterp(`def o (object {a: Integer}) end def mk1 fn [[x:Any] [Integer] [1]] end ((make o {a: 1}) "a" 5 set mk1)`)
	if runErr == nil {
		t.Errorf("consuming a 0-return Object set should error at runtime, got nil")
	}
}
