package core

import "testing"

// The Ideal root carries exactly ONE Behavior struct composing every
// root capability (NUR017). Before this pin, two init()s in different
// files each assigned TIdeal.Behavior a single-capability struct, so
// lexical filename order silently decided whether the root offered
// Sizer or IdealConverter — and renaming a file flipped the winner.
// This test fails the moment either capability drops off the RESOLVED
// behavior, whatever the init order.
func TestIdealRootBehaviorComposesCapabilities(t *testing.T) {
	b := TIdeal.Behavior()
	if _, ok := b.(idealRootBehavior); !ok {
		t.Fatalf("TIdeal.Behavior() = %T, want the single idealRootBehavior", b)
	}
	if _, ok := b.(Sizer); !ok {
		t.Error("the Ideal root Behavior lost the Sizer capability (size on Class/Store/Table/Error instances would silently return 0)")
	}
	if _, ok := b.(IdealConverter); !ok {
		t.Error("the Ideal root Behavior lost the IdealConverter capability (the {}/[] convert fallback would vanish)")
	}
}

// The composed root must behave identically through both capabilities:
// the converter fallback yields empty containers, and the payload-switch
// Sizer reaches an Ideal instance through the parent walk.
func TestIdealRootBehaviorFallbacks(t *testing.T) {
	b := TIdeal.Behavior().(idealRootBehavior)
	m, err := b.ToMap(NewInteger(1))
	if err != nil {
		t.Fatalf("ToMap: %v", err)
	}
	if rm, e := AsMap(m); e != nil || rm.Len() != 0 {
		t.Errorf("root ToMap fallback = %v, want empty map", m)
	}
	l, err := b.ToList(NewInteger(1))
	if err != nil {
		t.Fatalf("ToList: %v", err)
	}
	if rl, e := AsList(l); e != nil || rl.Len() != 0 {
		t.Errorf("root ToList fallback = %v, want empty list", l)
	}
	if got := b.Size(NewInteger(1)); got != 0 {
		t.Errorf("root Size of a non-Ideal payload = %d, want 0", got)
	}
}
