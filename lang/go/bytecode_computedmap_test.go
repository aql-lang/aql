package lang

import (
	"fmt"
	"strings"
	"testing"
)

// A COMPUTED map literal (`{k:(expr)}`) consumed in-frame — bound to a value-def
// local and returned as a fn body result, or read by a downstream word — records
// an OpMakeMap assembly so it resolves as a real per-run event. Before this it
// only recorded for make's construction body (dataMap), so the same map in a
// plain fn body refused "body result of unknown provenance" even though the
// interpreter runs it. The recording is gated on in-frame CONSUMPTION: a DEFERRED
// residual (a bare map tail, auto-evaluated after the frame pops) must still
// refuse, because compiling it in-frame would diverge from the interpreter.
func TestComputedMapInFnBodyCompiles(t *testing.T) {
	// Positive: the compiled result must be byte-identical to the interpreter.
	positives := []struct {
		name, src string
	}{
		{"returned body result", `def f fn [[a:Integer] [Map] [ def m {x:(a 1 add) y:(a 2 mul)} m ]] (f 5)`},
		{"read by a downstream word", `def f fn [[a:Integer] [Integer] [ def m {x:(a 1 add)} (m "x" get) ]] (f 5)`},
		{"const-valued computed map", `def f fn [[a:Integer] [Map] [ def m {x:(1 add 2)} m ]] (f 5)`},
		// LIST-valued entries: a plain map keeps the value AS a list (`{n:[3]}`);
		// `do {map}` evaluates each list value (`{n:3}`). Both need the list
		// WRAPPER recorded as a nested OpMakeList (interleaved per value), which
		// the top-frame guard otherwise refuses inside a fn body.
		{"plain map with list value", `def f fn [[a:Integer] [Map] [ def m {n:[a add 1]} m ]] (f 5)`},
		{"do single list-valued entry", `def f fn [[a:Integer] [Map] [ do {n:[a add 1]} ]] (f 5)`},
		{"do multi list-valued entries", `def f fn [[bf:Map] [Map] [ do {n:[bf "n" get], m:[bf "m" get]} ]] ({n:1 m:2} f)`},
		{"do mixed list and paren values", `def f fn [[bf:Map] [Map] [ do {n:[bf "n" get], m:(bf "m" get)} ]] ({n:1 m:2} f)`},
	}
	for _, c := range positives {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("expected the computed map to compile, got refusal: %v", err)
			}
			b, _ := New()
			want, werr := b.Run(c.src)
			if werr != nil {
				t.Fatalf("interpreter errored on a positive case: %v", werr)
			}
			if len(got) != len(want) {
				t.Fatalf("result arity %d != interpreter %d", len(got), len(want))
			}
			for i := range got {
				if fmt.Sprintf("%v", got[i]) != fmt.Sprintf("%v", want[i]) {
					t.Fatalf("compiled %v != interpreter %v", got[i], want[i])
				}
			}
		})
	}

	// Negative: a DEFERRED residual — a bare computed-map fn-body tail whose value
	// references a fn param — errors in the interpreter (the data-context paren is
	// evaluated after the frame pops, so the param is gone). The compiled path must
	// NOT silently succeed where the interpreter errors: it must refuse and fall
	// back, so RunCompiledStrict surfaces a force-compile refusal rather than a value.
	negatives := []struct {
		name, src string
	}{
		{"paren value", `def f fn [[a:Integer] [Map] [ {x:(a 1 add)} ]] (f 5)`},
		{"list value", `def f fn [[a:Integer] [Map] [ {x:[a add 1]} ]] (f 5)`},
	}
	for _, c := range negatives {
		t.Run("deferred-residual refuses/"+c.name, func(t *testing.T) {
			// The interpreter itself errors on this shape (undefined word a).
			b, _ := New()
			if _, werr := b.Run(c.src); werr == nil {
				t.Fatalf("expected the interpreter to error on the deferred-residual map")
			}
			a, _ := New()
			if _, err := a.RunCompiledStrict(c.src); err == nil {
				t.Fatal("compiled path produced a value where the interpreter errors — divergence")
			} else if !strings.Contains(err.Error(), "force-compile") {
				t.Errorf("expected a force-compile refusal, got %q", err.Error())
			}
		})
	}
}
