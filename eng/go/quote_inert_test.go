package eng

import "testing"

// TestQuoteOperandInertOKFlag pins the CompileQuoteInert contract (review
// cluster 6): a word that DECLARES the flag bakes its INERT quoted operand —
// the quote / codequote / raise / timeout family — via the same bypass that
// exempts get/getr/set, so it compiles as a plain CALL_NATIVE instead of
// refusing. A QuoteArgs word WITHOUT the flag (a meta word like usurp / ref
// whose quoted operand drives a re-stepping result the VM cannot reproduce)
// stays refused, and even a flagged word refuses a NON-inert (carrier) operand.
func TestQuoteOperandInertOKFlag(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	r.RegisterNativeFunc(NativeFunc{
		Name:          "probe-qi",
		CompileEffect: CompileQuoteInert,
		Signatures:    []Signature{{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name:       "probe-plainq",
		Signatures: []Signature{{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}, BarrierPos: -1}},
	})

	ownSig := func(name string) *Signature {
		fn := r.Lookup(name)
		if fn == nil {
			t.Fatalf("%s did not register", name)
		}
		for i := range fn.Signatures {
			if !fn.Signatures[i].Fallback {
				return &fn.Signatures[i]
			}
		}
		t.Fatalf("%s has no own signature", name)
		return nil
	}

	inertAtom := Value{Parent: TAtom, Data: AtomPayload{Name: "k"}}
	carrierArg := NewCarrier(TAtom) // not inert — no concrete payload to bake

	// Flagged + inert operand → bypass the quoted-operand refusal (bakeable).
	if !quoteOperandInertOK(r, "probe-qi", ownSig("probe-qi"), []Value{inertAtom}) {
		t.Error("flagged word + inert atom operand: want bakeable, got refused")
	}
	// Flagged + NON-inert (carrier) operand → still refuses (nothing to bake).
	if quoteOperandInertOK(r, "probe-qi", ownSig("probe-qi"), []Value{carrierArg}) {
		t.Error("flagged word + carrier operand: want refused, got bakeable")
	}
	// NOT flagged → refuses even with an inert operand (and it is not a module
	// inner native, so the other exemption branch does not apply either).
	if quoteOperandInertOK(r, "probe-plainq", ownSig("probe-plainq"), []Value{inertAtom}) {
		t.Error("unflagged QuoteArgs word: want refused, got bakeable")
	}
}
