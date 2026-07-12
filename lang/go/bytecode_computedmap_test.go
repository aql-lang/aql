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
		// A user-CALL result bound to a value-def and referenced MORE THAN ONCE —
		// here `mv` feeds both a later call (`mv g`) AND a map slot — must promote
		// to a frame local (store once, re-push per use), matching `def`'s
		// evaluate-once semantics. Without that promotion `mv` stayed loose on the
		// single-consume stack and the later call could not seat its operand
		// ("fn arg result is not on top"), the blocker across the bloom/stats unit
		// suites (`def m-val (derive-m …)` read by both derive-k and make-bits).
		{"multi-ref user-call value-def into map", `def g fn [[x:Integer] [Integer] [x add 1]]  def f fn [[x:Integer] [Map] [ def mv (x g)  do {a:(mv add 0), b:(mv g)} ]] (5 f)`},
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

// A `do {…}` value-eval map produces EXACTLY ONE value (the map), so it is not a
// runtime-variable (variadic) result even when it sits in an `if` arm or is the
// return of a RECURSIVE fn. tryRecordDynBody used to flag EVERY dyn-body event
// variadic; the whole-body case was rescued by RecordUserCall's declared-return
// exception, but a do-map inside a branch arm left the branch merge (and any fn
// whose body is that branch) variadic, so a fixed-arity consumer of the fn
// result — `(mk …) get "code"` — refused "consumes loop results (Stage 2 loops
// only feed the program residual)". Marking the concrete-map value-eval overload
// non-variadic at the source fixes both. Each case REFUSED before the fix; the
// RunCompiledStrict success here is also the no-FALLBACK-island proof (force mode
// does not fall back), and the byte-identical interpreter comparison guards the
// arity model. Only the code-body (List) overload and the dynamic-body poly case
// stay variadic.
func TestDoMapVariadicArmCompiles(t *testing.T) {
	positives := []struct {
		name, src string
	}{
		// do-map in BOTH if arms, fn result consumed by get.
		{"do-map both if arms, result get",
			`def mk fn [[i:Integer] [Map] [ if (i lte 0) [do {code:["x"] next:[i]}] [do {code:["y"] next:[i]}] ]]  def use fn [[i:Integer] [String] [ ((mk i) get "code") ]] (use 3)`},
		// RECURSIVE fn returning a do-map, result consumed by get (the gen-program /
		// compile-tagged-seq shape from the voxgig Template library).
		{"recursive do-map return, result get",
			`def mk fn [[i:Integer] [Map] [ if (i lte 0) [do {code:["x"] next:[i]}] [def more (mk (i sub 1)) do {code:[(more get "code")] next:[(more get "next")]}] ]]  def use fn [[i:Integer] [String] [ ((mk i) get "code") ]] (use 3)`},
		// do-map arm whose members read a value bound earlier in the same arm.
		{"do-map arm reads arm-local, result get",
			`def mk fn [[i:Integer] [Map] [ if (i lte 0) [do {code:["x"]}] [def t (i add 100) do {code:[(t 0 add)]}] ]]  def use fn [[i:Integer] [Integer] [ ((mk i) get "code") ]] (mk 3 get "code")`},
	}
	for _, c := range positives {
		t.Run("compiles/"+c.name, func(t *testing.T) {
			a, _ := New()
			got, err := a.RunCompiledStrict(c.src)
			if err != nil {
				t.Fatalf("expected the do-map arm to compile, got refusal: %v", err)
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
}
