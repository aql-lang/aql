package eng

import "testing"

// interpBodyInert / interpMemberInert widen the const-bake whitelist so a
// property-harness body (test-check-prop) containing an interpolated string
// bakes as code-as-data at MODULE scope. These pins fix the soundness boundary:
//   - an InterpString is admitted only as a MEMBER (interpMemberInert), never as
//     a standalone top-level const, and never when its holes are non-inert;
//   - interpBodyInert's TOP LEVEL stays exactly isInertConst — a standalone
//     ParenExpr is NOT bakeable (baking `(loopy 1)` wrongly compiled a nested
//     too-deep macroexpand that must refuse);
//   - isInertConst itself is UNCHANGED — still strict — which is what keeps an
//     InterpString body refused inside a compiled fn frame (the fn-scope
//     miscompile guard rests on this).
func TestInterpBodyInertBoundary(t *testing.T) {
	// `v${k}` — an InterpString with an inert hole (the Word k).
	interp := NewInterpString([]InterpPart{{Lit: "v"}, {Expr: []Value{NewWord("k")}}})

	// Admitted as a MEMBER, and inside a List body.
	if !interpMemberInert(interp) {
		t.Error("interpMemberInert(`v${k}`) = false; an InterpString with inert holes must be admitted as a member")
	}
	if !interpBodyInert(NewList([]Value{interp})) {
		t.Error("interpBodyInert([`v${k}`]) = false; a List body with an InterpString member must be inert")
	}
	// Nested two deep — the recursion must reach it.
	if !interpBodyInert(NewList([]Value{NewList([]Value{interp})})) {
		t.Error("interpBodyInert([[`v${k}`]]) = false; a deeply nested InterpString member must be reached")
	}

	// NEGATIVE: a carrier/dynamic InterpString (render only known at run time)
	// must NOT bake — a stale check-mode render would diverge.
	cInterp := interp
	cInterp.Carrier = true
	if interpMemberInert(cInterp) {
		t.Error("interpMemberInert(carrier `v${k}`) = true; a carrier InterpString must not bake")
	}
	// NEGATIVE: a hole that is a carrier (non-inert) rejects the whole InterpString.
	badHole := NewInterpString([]InterpPart{{Expr: []Value{NewCarrier(TInteger)}}})
	if interpMemberInert(badHole) {
		t.Error("interpMemberInert(`${carrier}`) = true; a non-inert hole must reject")
	}

	// TOP-LEVEL strictness: a standalone InterpString and a standalone ParenExpr
	// are NOT bakeable bodies (interpBodyInert defers to isInertConst there).
	if interpBodyInert(interp) {
		t.Error("interpBodyInert(`v${k}`) = true; a standalone InterpString must not bake (member-only)")
	}
	if interpBodyInert(NewParenExpr([]Value{NewWord("loopy"), NewInteger(1)})) {
		t.Error("interpBodyInert((loopy 1)) = true; a standalone ParenExpr is deferred code, not a bakeable body")
	}

	// INVARIANT: the strict whitelist must stay strict — it must NOT admit the
	// InterpString. This is the foundation of the fn-scope miscompile guard: at
	// fn scope noEvalBodiesInertScoped uses isInertConst, which keeps the body
	// refused so it falls back to the interpreter instead of baking a
	// frame-local ${name} that would resolve against the registry.
	if isInertConst(interp) {
		t.Error("isInertConst(`v${k}`) = true; the strict whitelist must NOT admit an InterpString")
	}
	if isInertConst(NewList([]Value{interp})) {
		t.Error("isInertConst([`v${k}`]) = true; the strict whitelist must keep an InterpString body refused")
	}
}
