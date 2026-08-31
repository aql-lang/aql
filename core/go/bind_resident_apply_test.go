package core

import "testing"

// ApplyResidentBind's two arms against the interpreter's own semantics
// (the parity contract the arm-resident twins carry): the install arm
// goes through InstallDef so per-element repeats STACK — a plain value
// pushes a fresh level each call, never a replace — and the undef arm
// pops the live entry, retiring a minted node only when the popped
// binding minted it. A nil registry is a no-op.
func TestApplyResidentBind(t *testing.T) {
	ApplyResidentBind(nil, "x", false, NewInteger(1)) // must not panic

	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Install arm: two calls stack two levels with their own values —
	// the measured interpreter leak shape (last element's install on top).
	ApplyResidentBind(r, "x", false, NewInteger(10))
	ApplyResidentBind(r, "x", false, NewInteger(20))
	if d := r.Defs.Depth("x"); d != 2 {
		t.Fatalf("two resident installs must stack: depth = %d, want 2", d)
	}
	if v, ok := r.Defs.Top("x"); !ok || v.String() != "20" {
		t.Fatalf("top after two installs = %v, want the second value 20", v)
	}

	// Undef arm: pops exactly one level.
	ApplyResidentBind(r, "x", true, Value{})
	if v, ok := r.Defs.Top("x"); !ok || v.String() != "10" {
		t.Fatalf("top after one resident undef = %v, want the first value 10", v)
	}
	ApplyResidentBind(r, "x", true, Value{})
	if _, ok := r.Defs.Top("x"); ok {
		t.Fatal("the second resident undef must drain the stack")
	}
	ApplyResidentBind(r, "x", true, Value{}) // empty pop: silent no-op, like undef

	// Undef arm retires a minted node — the arms must not drift from
	// ApplyBindTwin's BindUndef even though a var param never mints.
	minted := r.Types.MintType("Rz", TInteger)
	r.Defs.PushType("Rz", minted, NewTypeLiteral(TInteger))
	ApplyResidentBind(r, "Rz", true, Value{})
	if r.Types.LookupByID(minted.ID) != nil {
		t.Fatal("popping a minted type binding must retire its node")
	}
}
