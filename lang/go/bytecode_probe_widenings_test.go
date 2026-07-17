package lang

// Three probe-verified gate widenings (the REFUSAL-CLOSURE §9.4 open-site
// sweep, 2026-07-17), each turning a sound refusal into a compiled shape:
//
//   - a computed range START/STEP that resolves to a frame LOCAL lowers
//     (computedRangeBounds passes bounds as-is; RecordLoop admits const +
//     local operands and keeps refusing event-produced ones — opForSetup
//     already enforces the runtime Integer/zero-step taxonomy);
//   - an ALL-0-ARG member's shaped landing skips the statement-window scan
//     (the member never forward-collects, so following tokens belong to the
//     next dispatch — `r get "bool" eq false` declined on the `eq`);
//   - `behave` carries CompileStoresFn (it stores its fn for later Behavior
//     dispatch, never re-stepping it on the tape — the log/patrun/service
//     store-fn pattern), so a capture-free comparator bakes as a const.

import "testing"

func TestProbeWideningComputedRangeStartStep(t *testing.T) {
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Integer] [def acc 0 for [n 5] [def acc (acc add i)] end acc]] (f 1)`, "[10]")
	mustCompileWithParity(t,
		`def f fn [[n:Integer] [Integer] [def acc 0 for [0 6 n] [def acc (acc add i)] end acc]] f 2`, "[6]")
	mustCompileWithParity(t,
		`def s 2 def acc 0 for [s 4] [def acc (acc add i)] end acc`, "[5]")
	// An EVENT-produced step keeps the refusal (no re-pushable home).
	mustRefuseWithParity(t,
		`def f fn [[n:Integer] [Integer] [def acc 0 for [5 1 (0 sub n)] [def acc (acc add i)] end acc]] (f 1)`,
		"computed range start/step")
	// A 4-element range is a runtime for_error in BOTH engines (parseRange
	// arity 1-3; computedRangeBounds falls through the same arity gate).
	{
		src := `for [1 2 3 4] [ 5 drop ] 0`
		a, _ := New()
		_, errC := a.Run(src)
		b, _ := New()
		gotC, _, errC2 := b.RunCompiled(src)
		_ = gotC
		if codeOf(errC) != "for_error" || codeOf(errC2) != "for_error" {
			t.Errorf("4-elem range: want for_error both, got interp [%s] compiled [%s]", codeOf(errC), codeOf(errC2))
		}
	}
}

func TestProbeWideningZeroArgMethodLanding(t *testing.T) {
	mustCompileWithParity(t,
		`import "aql:rand"  def r (Rand.with-seed 7)  r get "bool" eq false`, "[true]")
}

func TestProbeWideningBehaveStoresFn(t *testing.T) {
	mustCompileWithParity(t, `def Person class {age:Integer}
behave "compare" ((fn [[a:Person b:Person] [Integer] [(a get "age") (b get "age") sub]])/r)
def alice (make Person {age:30})
def bob (make Person {age:25})
alice bob lt`, "[false]")
}

// Both-computed `if` over a NON-event condition (the S9.4 probe sweep):
// lowerBothComputedMatCond materialises the condFrag / const / local cond
// above the two eager arm values (the single-computed lowerComputedCond
// shapes) and JMP_IF_FALSE selects — parens evaluate eagerly in BOTH
// engines, so selection-only lowering is the event-cond arm's identical
// semantics. Both branch directions, both cond forms.
func TestProbeWideningBothComputedNonEventCond(t *testing.T) {
	mustCompileWithParity(t,
		`def f fn [[b:Boolean x:Integer] [Integer] [if b (x add 1) (x add 2)]] f true 5`, "[6]")
	mustCompileWithParity(t,
		`def f fn [[b:Boolean x:Integer] [Integer] [if b (x add 1) (x add 2)]] f false 5`, "[7]")
	mustCompileWithParity(t,
		`def f fn [[x:Integer] [Integer] [if [x lt 9] (x add 1) (x add 2)]] f 5`, "[6]")
	mustCompileWithParity(t,
		`def f fn [[x:Integer] [Integer] [if [x lt 9] (x add 1) (x add 2)]] f 50`, "[52]")
	// The event-cond regression control.
	mustCompileWithParity(t,
		`def f fn [[x:Integer] [Integer] [if (x lt 9) (x add 1) (x add 2)]] f 5`, "[6]")
}
