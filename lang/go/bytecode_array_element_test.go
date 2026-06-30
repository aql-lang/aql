package lang

import (
	"fmt"
	"testing"
)

// Off-corpus regressions for the parameterised mutable-Array carrier (Array<T>)
// soundness gate — design/ARRAY-ELEMENT-CARRIER{,-ARCH}.0.md. The langspec
// differential is BLIND to these shapes, so per the soundness-gate discipline
// they are pinned here as RunCompiled==Run (compile MUST equal interpret) plus
// a force-compile proof for the admit path. The NEGATIVE cases (a non-conforming
// set, an escape, an alias) matter more than the positive: each MUST stay sound
// by declining the strict element to gradual Any, never miscompiling.

// arrSound asserts the soundness invariant for one program: the byte compiler's
// result (taking the VM path when compilable, else falling back) EQUALS the
// interpreter's, and the two agree on error-vs-success. Returns whether the VM
// path was actually taken.
func arrSound(t *testing.T, src string) bool {
	t.Helper()
	a, _ := New()
	want, werr := a.Run(src)
	b, _ := New()
	got, compiled, gerr := b.RunCompiled(src)
	if (werr == nil) != (gerr == nil) {
		t.Fatalf("error disagreement (compile != interpret):\n  src: %s\n  interp err: %v\n  compiled err: %v", src, werr, gerr)
	}
	if werr == nil && fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("SILENT MISCOMPILE (compile != interpret):\n  src: %s\n  interp:   %v\n  compiled: %v", src, want, got)
	}
	return compiled
}

// A monomorphic-Integer local array whose gets feed arithmetic: the admit path
// must produce a strict Integer element AND stay sound (== interpret), and the
// program must force-compile (not refuse). radix-msd (the sort corpus) is the
// definitive proof the gate FIRES on a shape that only compiles with strict
// element typing; this pins the minimal admit-path soundness.
func TestArrayElement_MonomorphicIntegerCompiles(t *testing.T) {
	const src = `def h fn [[] [Integer] [
		def c (make Array [5 6 7])
		(c get 0) add (c get 1)
	]] h`
	arrSound(t, src)
	a, _ := New()
	got, err := a.RunCompiledStrict(src)
	if err != nil {
		t.Fatalf("monomorphic Integer array must force-compile, got refusal: %v", err)
	}
	if fmt.Sprint(got) != "[11]" {
		t.Fatalf("result = %v, want [11]", got)
	}
}

// NEGATIVE battery: each writes/escapes a non-Integer so the element CANNOT be a
// sound strict Integer; the gate must decline (get → Any) and the program must
// still compile==interpret. A regression that drops the gate would MISCOMPILE
// these (strict integer-add on a String value), which arrSound catches.
func TestArrayElement_NegativeStaysSound(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// a non-conforming set BEFORE the arithmetic get
			name: "set-before-get",
			src: `def f fn [[] [Scalar] [
				def a (make Array [0 0 0])
				def _ (a set 0 "x")
				(a get 0) add 1
			]] f`,
		},
		{
			// get typed BEFORE the poisoning set in source, runtime-divergent
			// across iterations (a[0] is Integer then String)
			name: "forward-pass-loop",
			src: `def g fn [[] [List] [
				def a (make Array [0])
				def out (make Array [0 0])
				def _ (iota 2 each [var [[i]
					out set i ((a get 0) add 1) end
					a set 0 "x" end
					0
				]])
				convert List out
			]] g`,
		},
		{
			// alias shares the make-site identity; a non-conforming set through
			// the alias must taint the original
			name: "alias",
			src: `def f fn [[] [Scalar] [
				def a (make Array [0 0 0])
				def b a
				def _ (b set 0 "x")
				(a get 0) add 1
			]] f`,
		},
		{
			// an array minted in one fn, returned, mutated non-conformingly in
			// the caller — the make-site identity carries across the return
			name: "return-escape",
			src: `def mk fn [[] [Array] [make Array [0 0 0]]]
			def f fn [[] [Scalar] [
				def a (mk)
				def _ (a set 0 "x")
				(a get 0) add 1
			]] f`,
		},
		{
			// HARDENING (residual #2): a tracked array passed to a fn that
			// mutates it non-conformingly THROUGH ITS PARAM. The param carrier
			// shares the make-site identity, so the inner set taints the
			// caller's array — a get in the caller must decline. Pins that the
			// forward-pass ordering hole is closed across the fn boundary.
			name: "escape-arg-mutation",
			src: `def taint fn [[x:Array] [Array] [ x set 0 "x" end x ]]
			def f fn [[] [Scalar] [
				def a (make Array [0 0 0])
				def _ (a taint)
				(a get 0) add 1
			]] f`,
		},
		{
			// HARDENING: a tracked array CAPTURED into a closure that mutates it
			// non-conformingly. Capture preserves the carrier identity (Spike A),
			// so the closure's set taints the captured array.
			name: "escape-capture-mutation",
			src: `def f fn [[] [Scalar] [
				def a (make Array [0 0 0])
				def mut ([] => [a set 0 "q"])
				def _ (mut)
				(a get 0) add 1
			]] f`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { arrSound(t, c.src) })
	}
}

// An untyped (param) array, and a non-make array, must behave EXACTLY as before
// the feature: get → gradual Any, no strict element. Pinned as soundness; the
// param case also exercises that a non-tracked receiver never admits.
func TestArrayElement_UntypedStaysSound(t *testing.T) {
	const src = `def f fn [[a:Array] [Scalar] [ (a get 0) add 1 ]]
	def arr (make Array [10 20 30])
	arr f`
	arrSound(t, src)
}
