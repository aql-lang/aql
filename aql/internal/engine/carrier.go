package engine

// Carrier-based static type-checking support.
//
// A "carrier" is a normal Value with Carrier=true and (typically)
// Data=nil: it carries only type information, not a concrete payload.
// The engine is driven in check mode by Registry.CheckMode. In that
// mode, the same dispatch machinery (matchSignature, forward
// collection, sort order, etc.) runs, but execMatch consults
// Signature.Returns to synthesise carrier results instead of calling
// the handler. This keeps runtime and checker in absolute parity.
//
// This file contains only the minimal helpers needed for the initial
// slice: a conversion from concrete literal values to carriers, and a
// carrier-result builder for a matched signature.

// NewCarrier constructs a carrier Value for the given type. Data is
// nil — the carrier represents "some value of type t", not a specific
// one.
func NewCarrier(t Type) Value {
	v := newValue(t, nil)
	v.Carrier = true
	return v
}

// toCarrier converts a concrete Value to its carrier form. Control /
// structural tokens (words, marks, moves, open-paren, paren-expr,
// interp-string, return-check, def-cleanup, forward) are returned
// unchanged: they drive dispatch and must retain their payloads.
// Lists and maps are returned unchanged for now so that list/map
// signature matching keeps working; carrier-aware list/map handling
// is future work.
func toCarrier(v Value) Value {
	if v.IsWord() || v.IsForward() || v.IsMark() || v.IsMove() ||
		v.IsOpenParen() || v.IsParenExpr() || v.IsInterpString() ||
		v.IsReturnCheck() || v.IsDefCleanup() {
		return v
	}
	// Keep lists and maps concrete for now — matchSignature relies
	// on Data presence for a few compound cases.
	if v.VType.Equal(TList) || v.VType.Equal(TMap) {
		return v
	}
	// Already a carrier.
	if v.Carrier {
		return v
	}
	v.Carrier = true
	v.Data = nil
	return v
}

// StripToCarriers returns a copy of in where every non-structural value
// has been converted to its carrier form. Used at the top-level Run()
// entry to bootstrap check-mode execution.
func StripToCarriers(in []Value) []Value {
	out := make([]Value, len(in))
	for i, v := range in {
		out[i] = toCarrier(v)
	}
	return out
}

// carrierResults returns the carrier Values that a matched signature
// produces in check mode. If Returns is populated, one carrier is
// produced per declared return type. If Returns is missing, a single
// TAny carrier is produced and a diagnostic is recorded against the
// registry, so the user gets actionable feedback about un-annotated
// words.
func carrierResults(r *Registry, word string, sig *Signature) []Value {
	if len(sig.Returns) == 0 {
		r.addCheckDiagnostic(CheckDiagnostic{
			Code:   "missing_returns",
			Detail: "word " + word + " has no declared Returns for matched signature; assuming Any",
			Word:   word,
		})
		return []Value{NewCarrier(TAny)}
	}
	out := make([]Value, len(sig.Returns))
	for i, t := range sig.Returns {
		out[i] = NewCarrier(t)
	}
	return out
}

// addCheckDiagnostic appends a diagnostic to the registry. Safe to call
// outside of check mode — it simply records the finding.
func (r *Registry) addCheckDiagnostic(d CheckDiagnostic) {
	r.CheckDiagnostics = append(r.CheckDiagnostics, d)
}
