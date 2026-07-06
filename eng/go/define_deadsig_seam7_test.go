package eng

// Seam-7 (cluster A4): in-package unit tests for previously-unreached
// blocks in define_type.go, sigimpl.go and deadsig.go. Direct calls to the
// guard / error / marker arms per design/TEST-SEAMS.10.md.

import "testing"

// --- define_type.go -------------------------------------------------------

func TestS7DefineMemberTypeMicronNaming(t *testing.T) {
	r := newTestRegistry(t)
	// A member type minted under the Micron branch must carry the -on
	// suffix; a plain capitalised name is rejected.
	_, err := r.DefineMemberType("S7Bad", TMicron, func(Value) bool { return true })
	if err == nil {
		t.Error("a Micron-parented member type without the -on suffix must fail")
	}
}

// --- sigimpl.go -----------------------------------------------------------

func TestS7SigImplMarkersAndNilImpl(t *testing.T) {
	(&GoImpl{}).sigImpl()  // zero-statement seal marker
	(&AQLImpl{}).sigImpl() // zero-statement seal marker
	// A signature with no implementation returns a nil dispatch handler.
	var s Signature
	if s.DispatchHandler() != nil {
		t.Error("a nil-Impl signature must have a nil dispatch handler")
	}
}

// --- deadsig.go -----------------------------------------------------------

func TestS7DeadSignaturesShort(t *testing.T) {
	if got := DeadSignatures(nil); got != nil {
		t.Errorf("DeadSignatures(nil) = %v, want nil", got)
	}
	if got := DeadSignatures([]Signature{{Args: []*Type{TInteger}, BarrierPos: -1}}); got != nil {
		t.Errorf("DeadSignatures(single) = %v, want nil", got)
	}
}

func TestS7DeadSignaturesDuplicateAndFallback(t *testing.T) {
	sigA := Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(nil)}
	sigDup := Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(nil)}
	// A trailing fallback sig must be skipped, and the duplicate detected.
	fb := Signature{Fallback: true, BarrierPos: -1, Impl: Go(nil)}
	dead := DeadSignatures([]Signature{sigA, sigDup, fb})
	if len(dead) == 0 {
		t.Error("a duplicate signature should be flagged dead")
	}
	for _, d := range dead {
		if d.Sig.Fallback {
			t.Error("a fallback signature must never be reported dead")
		}
	}
}

func TestS7DeadSignaturesFallbackFirst(t *testing.T) {
	// A fallback with MORE args sorts BEFORE a normal sig (CompareSignatures
	// ignores Fallback, ranks by arg count) — so the inner loop encounters a
	// fallback at an earlier index than a non-fallback and must skip it.
	fb2 := Signature{Fallback: true, Args: []*Type{TInteger, TInteger}, BarrierPos: -1, Impl: Go(nil)}
	norm := Signature{Args: []*Type{TInteger}, BarrierPos: -1, Impl: Go(nil)}
	if got := DeadSignatures([]Signature{norm, fb2}); len(got) != 0 {
		t.Errorf("no sig should be dead here, got %v", got)
	}
}

func TestS7SigSubsumesNilType(t *testing.T) {
	s1 := &Signature{Args: []*Type{nil}, BarrierPos: -1}
	s2 := &Signature{Args: []*Type{nil}, BarrierPos: -1}
	if sigSubsumes(s1, s2) {
		t.Error("a nil arg type must make sigSubsumes decline")
	}
}
