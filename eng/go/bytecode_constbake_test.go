package eng

import "testing"

// The const pool is the bytecode compiler's single sharpest soundness edge:
// isInertConst (emit.go) decides what may live in Program.Consts, and a pooled
// const is pushed by the SAME pointer-backed Value on every execution —
// including each iteration of a loop. So the whitelist MUST NEVER admit a
// MUTABLE instance type (Array / Object / Store): if one slipped in, an
// in-place mutator (`set` on an Array/Object/Store receiver) would corrupt the
// baked const across iterations with no type error. The invariant is documented
// exhaustively at isInertConst's MUTATION-SAFETY note; this test PINS it so a
// well-meaning "just add this type to the switch" change fails loudly here
// rather than silently in a loop.
//
// Paired positive/negative per the repo's test discipline: the immutable shapes
// the whitelist is FOR stay bakeable, the mutable instances it must exclude do
// not.

func TestIsInertConstRejectsMutableInstances(t *testing.T) {
	// NEGATIVE — mutable instance types must never bake as consts.
	mutable := []struct {
		name string
		v    Value
	}{
		{"array", NewArray([]Value{NewInteger(1), NewInteger(2)})},
		{"empty-array", NewArrayEmpty()},
		{"store", NewStore(TStore)},
		{"object-instance", NewObjectInstance(TObject, ObjectInstanceInfo{})},
	}
	for _, m := range mutable {
		if isInertConst(m.v) {
			t.Errorf("isInertConst(%s) = true; a mutable instance type must NOT be const-bakeable "+
				"(a pooled const is pushed by the same pointer every iteration — an in-place "+
				"mutator would corrupt it). See isInertConst's MUTATION-SAFETY note before "+
				"changing the whitelist.", m.name)
		}
		// And as a MEMBER of a const compound — a mutable instance nested in a
		// list/map must keep the whole compound off the const path.
		if isInertConstMember(m.v) {
			t.Errorf("isInertConstMember(%s) = true; a mutable instance must not ride inside a const compound", m.name)
		}
		if isInertConst(NewList([]Value{m.v})) {
			t.Errorf("isInertConst([%s]) = true; a list holding a mutable instance must not bake", m.name)
		}
	}

	// POSITIVE — the immutable scalars/compounds the whitelist exists for stay
	// bakeable, so the negative assertions above are not vacuously passing on a
	// blanket "everything is false".
	inert := []struct {
		name string
		v    Value
	}{
		{"integer", NewInteger(7)},
		{"float", NewFloat(1.5)},
		{"string", NewString("x")},
		{"boolean", NewBoolean(true)},
		{"int-list", NewList([]Value{NewInteger(1), NewInteger(2)})},
	}
	for _, c := range inert {
		if !isInertConst(c.v) {
			t.Errorf("isInertConst(%s) = false; an immutable scalar/compound must stay const-bakeable", c.name)
		}
	}
}
