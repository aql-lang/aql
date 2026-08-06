package check

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// boruPredSig builds a single boru-bodied signature for a predicate fn.
func boruPredSig(params []core.FnParam, returns []*core.Type, body []core.Value) core.Signature {
	return core.Signature{Params: params, Returns: returns, Impl: &core.BoruImpl{Body: body}}
}

func installPred(r *core.Registry, name string, sigs []core.Signature) {
	r.Defs.Push(name, core.NewFunction(core.FnDefInfo{Name: name, Signatures: sigs}))
}

// predicateImpliedType's signature-shape gates: only a single-overload,
// single-named-param, Boolean-returning boru fn with a recognisable is-T body
// qualifies. Every rejection path is pinned.
func TestPredicateImpliedTypeGates(t *testing.T) {
	if _, ok := predicateImpliedType(nil, "p"); ok {
		t.Error("nil registry must decline")
	}
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := predicateImpliedType(r, ""); ok {
		t.Error("empty name must decline")
	}
	if _, ok := predicateImpliedType(r, "nosuchfn"); ok {
		t.Error("unknown fn must decline")
	}

	xParam := []core.FnParam{{Name: "x", Type: core.TAny}}
	bodyA := []core.Value{core.NewWord("x"), core.NewWord("is"), core.NewWord("List")}

	// POSITIVE: the canonical is-List predicate.
	installPred(r, "islist", []core.Signature{boruPredSig(xParam, []*core.Type{core.TBoolean}, bodyA)})
	if tt, ok := predicateImpliedType(r, "islist"); !ok || !tt.Equal(core.TList) {
		t.Errorf("islist = %v/%v, want List/true", tt, ok)
	}

	// Two real (non-fallback) overloads → decline.
	installPred(r, "multi", []core.Signature{
		boruPredSig(xParam, []*core.Type{core.TBoolean}, bodyA),
		boruPredSig([]core.FnParam{{Name: "y", Type: core.TAny}}, []*core.Type{core.TBoolean},
			[]core.Value{core.NewWord("y"), core.NewWord("is"), core.NewWord("Map")}),
	})
	if _, ok := predicateImpliedType(r, "multi"); ok {
		t.Error("multi-overload must decline")
	}

	// Non-Boru implementation (a native GoImpl) → decline.
	installPred(r, "native", []core.Signature{{Params: xParam, Returns: []*core.Type{core.TBoolean}, Impl: &core.GoImpl{}}})
	if _, ok := predicateImpliedType(r, "native"); ok {
		t.Error("non-Boru impl must decline")
	}

	// Only a fallback signature (no real overload) → decline.
	installPred(r, "onlyfb", []core.Signature{{Fallback: true, Impl: &core.GoImpl{}}})
	if _, ok := predicateImpliedType(r, "onlyfb"); ok {
		t.Error("fallback-only predicate must decline")
	}

	// Two params → decline.
	installPred(r, "twop", []core.Signature{boruPredSig(
		[]core.FnParam{{Name: "x", Type: core.TAny}, {Name: "y", Type: core.TAny}}, []*core.Type{core.TBoolean}, bodyA)})
	if _, ok := predicateImpliedType(r, "twop"); ok {
		t.Error("two-param predicate must decline")
	}

	// Unnamed param → decline.
	installPred(r, "unnamed", []core.Signature{boruPredSig(
		[]core.FnParam{{Name: "", Type: core.TAny}}, []*core.Type{core.TBoolean}, bodyA)})
	if _, ok := predicateImpliedType(r, "unnamed"); ok {
		t.Error("unnamed-param predicate must decline")
	}

	// Non-Boolean return → decline.
	installPred(r, "intret", []core.Signature{boruPredSig(xParam, []*core.Type{core.TInteger}, bodyA)})
	if _, ok := predicateImpliedType(r, "intret"); ok {
		t.Error("non-Boolean-return predicate must decline")
	}
}

// predicateBodyImpliedType matches shape A (`x is T`) and shape B
// (`if (x is T) [_] [false]`), the latter both marker-expanded (6 tokens) and
// with a raw ParenExpr cond (4 tokens). Every non-matching body declines.
func TestPredicateBodyImpliedType(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	list := func(v ...core.Value) core.Value { return core.NewList(v) }

	// Shape A.
	if tt, ok := predicateBodyImpliedType(r, "x",
		[]core.Value{core.NewWord("x"), core.NewWord("is"), core.NewWord("List")}); !ok || !tt.Equal(core.TList) {
		t.Errorf("shape A = %v/%v, want List/true", tt, ok)
	}
	// Shape A that fails the triple (wrong var) → decline.
	if _, ok := predicateBodyImpliedType(r, "x",
		[]core.Value{core.NewWord("y"), core.NewWord("is"), core.NewWord("List")}); ok {
		t.Error("shape A wrong-var must decline")
	}

	// Shape B, 6-token, concrete `false` else-arm.
	sixFalse := []core.Value{core.NewWord("if"), core.NewWord("x"), core.NewWord("is"), core.NewWord("Map"),
		list(core.NewBoolean(true)), list(core.NewBoolean(false))}
	if tt, ok := predicateBodyImpliedType(r, "x", sixFalse); !ok || !tt.Equal(core.TMap) {
		t.Errorf("shape B (bool false) = %v/%v, want Map/true", tt, ok)
	}
	// Shape B with the `false` WORD else-arm.
	sixWordFalse := []core.Value{core.NewWord("if"), core.NewWord("x"), core.NewWord("is"), core.NewWord("Map"),
		list(core.NewBoolean(true)), list(core.NewWord("false"))}
	if tt, ok := predicateBodyImpliedType(r, "x", sixWordFalse); !ok || !tt.Equal(core.TMap) {
		t.Errorf("shape B (word false) = %v/%v, want Map/true", tt, ok)
	}
	// Shape B, 4-token raw ParenExpr cond → normalized.
	fourParen := []core.Value{core.NewWord("if"),
		core.NewParenExpr([]core.Value{core.NewWord("x"), core.NewWord("is"), core.NewWord("Map")}),
		list(core.NewBoolean(true)), list(core.NewBoolean(false))}
	if tt, ok := predicateBodyImpliedType(r, "x", fourParen); !ok || !tt.Equal(core.TMap) {
		t.Errorf("shape B (paren cond) = %v/%v, want Map/true", tt, ok)
	}

	// Shape B whose else-arm is not `false` → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []core.Value{core.NewWord("if"), core.NewWord("x"),
		core.NewWord("is"), core.NewWord("Map"), list(core.NewBoolean(true)), list(core.NewBoolean(true))}); ok {
		t.Error("shape B non-false else must decline")
	}
	// Shape B whose triple fails → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []core.Value{core.NewWord("if"), core.NewWord("y"),
		core.NewWord("is"), core.NewWord("Map"), list(core.NewBoolean(true)), list(core.NewBoolean(false))}); ok {
		t.Error("shape B bad-triple must decline")
	}
	// Shape B whose arms are not lists → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []core.Value{core.NewWord("if"), core.NewWord("x"),
		core.NewWord("is"), core.NewWord("Map"), core.NewInteger(1), core.NewInteger(2)}); ok {
		t.Error("shape B non-list arms must decline")
	}
	// Shape B whose else-arm is not a single element → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []core.Value{core.NewWord("if"), core.NewWord("x"),
		core.NewWord("is"), core.NewWord("Map"), list(core.NewBoolean(true)), list(core.NewBoolean(false), core.NewBoolean(false))}); ok {
		t.Error("shape B multi-element else must decline")
	}
	// 4-token, `if`, but cond is NOT a ParenExpr → falls through (not 6) → decline.
	if _, ok := predicateBodyImpliedType(r, "x",
		[]core.Value{core.NewWord("if"), core.NewWord("x"), list(core.NewBoolean(true)), list(core.NewBoolean(false))}); ok {
		t.Error("shape B non-paren 4-token must decline")
	}
	// Unrecognised length → decline.
	if _, ok := predicateBodyImpliedType(r, "x", []core.Value{core.NewWord("x"), core.NewWord("is")}); ok {
		t.Error("2-token body must decline")
	}
}

// guardTripleType resolves `param is T` to T, through the def table, the kernel
// name table, and the external-builtin path resolver; every failure path is
// pinned.
func TestGuardTripleType(t *testing.T) {
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	w := core.NewWord
	// w0 not the param.
	if _, ok := guardTripleType(r, "x", w("y"), w("is"), w("List")); ok {
		t.Error("w0-mismatch must decline")
	}
	// w1 not `is`.
	if _, ok := guardTripleType(r, "x", w("x"), w("eq"), w("List")); ok {
		t.Error("w1-not-is must decline")
	}
	// Kernel name-table resolution.
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), w("List")); !ok || !tt.Equal(core.TList) {
		t.Errorf("builtin List = %v/%v", tt, ok)
	}
	// External-builtin path resolution (typeNames misses, ResolveTypePath hits).
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), w(core.TMap.Path())); !ok || !tt.Equal(core.TMap) {
		t.Errorf("path %q = %v/%v, want Map/true", core.TMap.Path(), tt, ok)
	}
	// Def-table alias resolution.
	r.Defs.PushType("Foo", core.TList, core.NewTypeLiteral(core.TList))
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), w("Foo")); !ok || !tt.Equal(core.TList) {
		t.Errorf("def alias Foo = %v/%v, want List/true", tt, ok)
	}
	// Unresolvable type name.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), w("Zzznotatype")); ok {
		t.Error("unresolvable type must decline")
	}
	// None is excluded.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), w("None")); ok {
		t.Error("None must decline")
	}
	// tv already a bare type literal (not a Word).
	if tt, ok := guardTripleType(r, "x", w("x"), w("is"), core.NewTypeLiteral(core.TMap)); !ok || !tt.Equal(core.TMap) {
		t.Errorf("literal tv = %v/%v, want Map/true", tt, ok)
	}
	// tv a concrete non-type value (not a bare node) → decline.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), core.NewInteger(5)); ok {
		t.Error("concrete non-type tv must decline")
	}
	// tv a malformed Word (Parent=TWord, no WordInfo payload): IsWord passes,
	// AsWord fails → the defensive decline.
	if _, ok := guardTripleType(r, "x", w("x"), w("is"), core.Value{Parent: core.TWord}); ok {
		t.Error("malformed-word tv must decline")
	}
}

// isWordNamed, stripGuardMarkers, defsHasType — the small predicates.
func TestGuardPredicateHelpers(t *testing.T) {
	// isWordNamed.
	if isWordNamed(core.NewInteger(5), "x") {
		t.Error("a non-Word is not word-named")
	}
	if isWordNamed(core.Value{Parent: core.TWord}, "x") {
		t.Error("a non-concrete Word is not word-named")
	}
	if isWordNamed(core.NewWord("foo"), "bar") {
		t.Error("wrong name must be false")
	}
	if !isWordNamed(core.NewWord("is"), "is") {
		t.Error("matching name must be true")
	}

	// stripGuardMarkers drops paren/end markers, keeps operands.
	got := stripGuardMarkers([]core.Value{core.NewOpenParen(), core.NewWord("x"), core.NewWord("is"),
		core.NewWord("Map"), core.NewCloseParen(), core.NewEnd()})
	if len(got) != 3 {
		t.Fatalf("stripGuardMarkers kept %d tokens, want 3", len(got))
	}
	// A marker-free slice is returned intact.
	if s := stripGuardMarkers([]core.Value{core.NewWord("x")}); len(s) != 1 {
		t.Errorf("marker-free strip len = %d, want 1", len(s))
	}

	// defsHasType.
	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if defsHasType(r, "Nope") {
		t.Error("absent name must be false")
	}
	r.Defs.PushType("Bar", core.TMap, core.NewTypeLiteral(core.TMap))
	if !defsHasType(r, "Bar") {
		t.Error("bound type name must be true")
	}
}
