package eng

import "testing"

// W9 word_extend.go coverage: sigTupleEqual's nil-type / pattern-presence
// arms, sigTupleString's nil-type default, and TransplantExtension's
// all-forward barrier resolution and empty-incoming short-circuit.

func TestW9SigTupleEqual(t *testing.T) {
	// Both arg types nil → both default to Any and compare equal (no patterns).
	an := &Signature{Args: []*Type{nil}}
	bn := &Signature{Args: []*Type{nil}}
	if !sigTupleEqual(an, bn) {
		t.Error("two nil-typed single-arg tuples should be equal")
	}

	// One side has a pattern, the other doesn't → unequal (aok != bok).
	pa := NewInteger(1)
	withPat := &Signature{Params: []FnParam{{Type: TInteger, Pattern: &pa}}}
	noPat := &Signature{Args: []*Type{TInteger}}
	if sigTupleEqual(withPat, noPat) {
		t.Error("a pattern vs no pattern should be unequal")
	}

	// Both have patterns but they differ → unequal.
	pb := NewInteger(2)
	withPat2 := &Signature{Params: []FnParam{{Type: TInteger, Pattern: &pb}}}
	if sigTupleEqual(withPat, withPat2) {
		t.Error("different patterns should be unequal")
	}
}

func TestW9SigTupleStringNilType(t *testing.T) {
	// A nil arg type renders as the Any leaf.
	if got := sigTupleString(&Signature{Args: []*Type{nil}}); got == "" {
		t.Error("sigTupleString should render a nil arg type as Any")
	}
}

func TestW9TransplantExtensionEmptyIncoming(t *testing.T) {
	r := newTestRegistry(t)
	InstallFnDef(r, "w9base", FnDefInfo{
		Signatures: []Signature{{
			Params: []FnParam{{Name: "a", Type: TInteger}},
			Impl:   AQL([]Value{NewWord("a")}),
		}},
	})
	// An extension whose only sig is a fallback: nothing to transplant.
	err := TransplantExtension(r, FnDefInfo{
		Extends:    "w9base",
		Signatures: []Signature{{Fallback: true, BarrierPos: 0}},
	}, "w9origin", "")
	if err != nil {
		t.Errorf("empty-incoming transplant should be a no-op, got %v", err)
	}
}

func TestW9TransplantExtensionAllForwardBarrier(t *testing.T) {
	r := newTestRegistry(t)
	InstallFnDef(r, "w9base2", FnDefInfo{
		Signatures: []Signature{{
			Params: []FnParam{{Name: "a", Type: TInteger}},
			Impl:   AQL([]Value{NewWord("a")}),
		}},
	})
	// A host-authored clone with an all-forward barrier: TransplantExtension
	// resolves BarrierAllForward to the sig's arg count. We only require it to
	// run without panicking (merge outcome is incidental to this arm).
	_ = TransplantExtension(r, FnDefInfo{
		Extends: "w9base2",
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "b", Type: TString}},
			BarrierPos: BarrierAllForward,
			Impl:       AQL([]Value{NewWord("b")}),
		}},
	}, "w9origin2", "")
}
