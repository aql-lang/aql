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
// produces in check mode. Resolution order:
//
//  1. If sig.ReturnsFn is set, it is invoked with the carrier-typed
//     args; the results are coerced to carriers (Carrier=true, Data
//     stripped for scalar types) and returned.
//  2. Otherwise, if sig.Returns is non-empty, one fresh carrier is
//     produced per declared Returns type.
//  3. Otherwise a diagnostic is recorded and a single TAny carrier is
//     returned so the checker can keep making progress.
//
// args are the carrier-typed input values in signature order (same
// args that would be passed to the runtime handler).
func carrierResults(r *Registry, word string, sig *Signature, args []Value) []Value {
	if sig.ReturnsFn != nil {
		raw := sig.ReturnsFn(args)
		out := make([]Value, len(raw))
		for i, v := range raw {
			out[i] = toCarrier(v)
		}
		return out
	}
	// Explicit nil (no annotation) triggers the fallback. An empty but
	// non-nil slice is a valid "returns nothing" declaration.
	if sig.Returns == nil {
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

// ReturnsIdentity is a ReturnsFunc helper that returns its inputs
// unchanged (as carriers). Use for stack operations that preserve
// their inputs — dup, swap, over, rot, etc. — where the output types
// are directly expressible in terms of the input types.
//
// The mapping is a permutation-description slice: result[i] = args[mapping[i]].
// Example: swap is ReturnsIdentity(1, 0); over is ReturnsIdentity(0, 1, 0).
func ReturnsIdentity(mapping ...int) ReturnsFunc {
	return func(args []Value) []Value {
		out := make([]Value, len(mapping))
		for i, m := range mapping {
			if m < 0 || m >= len(args) {
				out[i] = NewCarrier(TAny)
				continue
			}
			out[i] = args[m]
		}
		return out
	}
}

// ReturnsStatic builds a ReturnsFunc that always produces a fixed list
// of carrier types, independent of args. Equivalent to setting Returns
// directly; provided so ReturnsFn call sites can be uniform.
func ReturnsStatic(types ...Type) ReturnsFunc {
	return func(_ []Value) []Value {
		out := make([]Value, len(types))
		for i, t := range types {
			out[i] = NewCarrier(t)
		}
		return out
	}
}

// ReturnsNumericBinary models the common arithmetic pattern: when
// both args are integers the result is an integer, otherwise the
// result is a decimal. Applies to add, sub, mul, div, mod, pow when
// the matched signature is [TNumber, TNumber].
func ReturnsNumericBinary() ReturnsFunc {
	return func(args []Value) []Value {
		if len(args) == 2 &&
			args[0].VType.Matches(TInteger) &&
			args[1].VType.Matches(TInteger) {
			return []Value{NewCarrier(TInteger)}
		}
		return []Value{NewCarrier(TDecimal)}
	}
}

// commonAncestorType returns the longest common prefix of two type
// paths, as a new Type. For example, given Number/Integer/42 and
// Number/Integer/99, returns Number/Integer. Returns TAny if there is
// no shared prefix.
func commonAncestorType(a, b Type) Type {
	n := len(a.Parts)
	if len(b.Parts) < n {
		n = len(b.Parts)
	}
	shared := 0
	for shared < n && a.Parts[shared] == b.Parts[shared] {
		shared++
	}
	if shared == 0 {
		return TAny
	}
	parts := make([]string, shared)
	copy(parts, a.Parts[:shared])
	return Type{Parts: parts}
}

// JoinCarriers folds two carriers into a single carrier that
// represents the disjunction of both. Applies a few simple
// normalisations:
//
//   - Identical VTypes collapse to one carrier.
//   - If one side is a strict subtype of the other, the parent wins.
//   - Sibling literal types (e.g. Number/Integer/42 vs Number/Integer/99)
//     collapse to their nearest common ancestor (Number/Integer).
//   - Otherwise a TDisjunct carrier is returned whose Data is a
//     DisjunctInfo listing the unique alternative type literals.
//
// This is the primary join used when the checker needs to combine
// two branch outcomes (e.g. `if` then/else).
func JoinCarriers(a, b Value) Value {
	if a.VType.Equal(b.VType) {
		out := a
		out.Carrier = true
		out.Data = nil
		return out
	}
	if a.VType.Matches(b.VType) {
		// a is subtype of b → widen to b
		return NewCarrier(b.VType)
	}
	if b.VType.Matches(a.VType) {
		return NewCarrier(a.VType)
	}
	// Check for a non-trivial common ancestor (shared prefix of at
	// least one part). This collapses value-tagged literals (e.g.
	// Number/Integer/42 vs Number/Integer/99 → Number/Integer).
	anc := commonAncestorType(a.VType, b.VType)
	if len(anc.Parts) > 0 && !anc.Equal(TAny) {
		return NewCarrier(anc)
	}
	// No subtype relation and no useful ancestor — build a disjunction carrier.
	alts := []Value{NewTypeLiteral(a.VType), NewTypeLiteral(b.VType)}
	v := NewDisjunct(alts)
	v.Carrier = true
	return v
}

// RunCarrierBody runs a list body (a Value with VType=TList) through a
// fresh sub-engine in check mode and returns the residual carrier
// stack. Returns nil if the body is not a concrete list. Requires
// that the registry is already in CheckMode (callers set it).
//
// Used by branch-aware words (e.g. `if`) to analyse each branch
// symbolically.
func RunCarrierBody(r *Registry, body Value) []Value {
	if body.Data == nil {
		return nil
	}
	elems := body.AsList()
	if elems.IsNil() {
		return nil
	}
	tokens := make([]Value, elems.Len())
	copy(tokens, elems.Slice())
	sub := New(r)
	result, err := sub.Run(tokens)
	if err != nil {
		r.addCheckDiagnostic(CheckDiagnostic{
			Code:   "branch_error",
			Detail: "branch analysis error: " + err.Error(),
		})
		return nil
	}
	return result
}

// JoinCarrierStacks folds two carrier result stacks (e.g. produced by
// two branches of an `if`) into a single stack. The shorter stack is
// padded out with TNone carriers; per-position join uses JoinCarriers.
func JoinCarrierStacks(a, b []Value) []Value {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		var ai, bi Value
		if i < len(a) {
			ai = a[i]
		} else {
			ai = NewCarrier(TNone)
		}
		if i < len(b) {
			bi = b[i]
		} else {
			bi = NewCarrier(TNone)
		}
		out[i] = JoinCarriers(ai, bi)
	}
	return out
}

// addCheckDiagnostic appends a diagnostic to the registry. Safe to call
// outside of check mode — it simply records the finding.
func (r *Registry) addCheckDiagnostic(d CheckDiagnostic) {
	r.CheckDiagnostics = append(r.CheckDiagnostics, d)
}
