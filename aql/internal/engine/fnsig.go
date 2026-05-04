package engine

// Function-signature comparison helpers — the single home for FnSig
// shape-matching across the engine. Two distinct rules live side by
// side:
//
//   - **Exact match** (fnSigMatchesSpec) — same arity AND pairwise
//     Type.Equal on params and returns. Used by `undef name fn [spec]`
//     to identify the precise previously-installed signature to remove.
//   - **Structural subtyping** (fnSigSatisfiesSpec) — same arity,
//     contravariant inputs (`spec ⊆ sig`) and covariant returns (`sig
//     ⊆ spec`). Used by `type Foo fn [...]` constraint matching via
//     `fnDefHasSig` / `fnUndefMatchesFnDef`.
//
// A FnUndef value (Type/Word/__UF) carries a list of FnSigSpec entries
// — each one a (Params, Returns) pair without a body. It's produced
// by `fn [[input] [output]]` and acts as a structural function-shape
// constraint. Pattern (FnParam.Pattern) and Optional/BarrierPos
// differences are not yet considered.

// fnSigMatchesSpec returns true if a FnSig matches a FnSigSpec
// exactly: same arity, same param types pairwise, same return types
// pairwise. Variance is intentionally NOT applied — `undef name fn
// [spec]` names a specific shape to discard.
func fnSigMatchesSpec(sig FnSig, spec FnSigSpec) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		if !sig.Params[i].Type.Equal(spec.Params[i].Type) {
			return false
		}
	}
	if len(sig.Returns) != len(spec.Returns) {
		return false
	}
	for i := range sig.Returns {
		if !sig.Returns[i].Equal(spec.Returns[i]) {
			return false
		}
	}
	return true
}

// fnSigSatisfiesSpec returns true if a candidate FnSig satisfies a
// FnSigSpec under structural function subtyping:
//
//   - **Inputs are contravariant.** Each spec param type must be a
//     subtype of the candidate's param type at the same position.
//     Example: spec=[Integer], sig=[Number] — sig accepts Integer
//     (because Integer ⊂ Number), so it satisfies the spec.
//   - **Returns are covariant.** Each candidate return type must be
//     a subtype of the spec's return type at the same position.
//     Example: spec=[Number], sig=[Integer] — sig produces Integer
//     which is also a Number, so it satisfies the spec.
//
// Strictly widens the previous exact-match rule.
func fnSigSatisfiesSpec(sig FnSig, spec FnSigSpec) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		// Contravariant: spec_input must be a subtype of sig_input.
		// `t.Matches(pattern)` is true iff t ⊆ pattern in the type
		// lattice, so spec.Type.Matches(sig.Type) checks spec ⊆ sig.
		if !spec.Params[i].Type.Matches(sig.Params[i].Type) {
			return false
		}
	}
	if len(sig.Returns) != len(spec.Returns) {
		return false
	}
	for i := range sig.Returns {
		// Covariant: sig_return must be a subtype of spec_return.
		if !sig.Returns[i].Matches(spec.Returns[i]) {
			return false
		}
	}
	return true
}

// fnUndefMatchesFnDef reports whether the candidate function value
// (TFnDef or TFunction wrapping FnDefInfo) satisfies every FnSigSpec
// declared by the FnUndef constraint.
func fnUndefMatchesFnDef(undef Value, fnVal Value) bool {
	uInfo, ok := undef.Data.(FnUndefInfo)
	if !ok {
		return false
	}
	fnDef, ok := fnVal.Data.(FnDefInfo)
	if !ok {
		return false
	}
	if len(uInfo.Sigs) == 0 {
		// An empty constraint trivially matches any function. Treat
		// this as an authoring error in practice — but it's well
		// defined.
		return true
	}
	for _, want := range uInfo.Sigs {
		if !fnDefHasSig(fnDef, want) {
			return false
		}
	}
	return true
}

// fnDefHasSig reports whether the candidate has at least one
// signature that satisfies `want` under structural subtyping. Both
// AQL-defined Sigs (with FnParam payload) and compiled Signatures
// (with raw Type payload) are considered so Go-implemented words can
// also satisfy a FnUndef type. The variance rule is delegated to
// fnSigSatisfiesSpec.
func fnDefHasSig(fnDef FnDefInfo, want FnSigSpec) bool {
	for _, s := range fnDef.Sigs {
		if fnSigSatisfiesSpec(s, want) {
			return true
		}
	}
	for _, sig := range fnDef.Signatures {
		if sig.Fallback {
			continue
		}
		// Compiled Signatures store Args as []Type; lift to a FnSig
		// shape so the shared comparison helper applies.
		params := make([]FnParam, len(sig.Args))
		for i, t := range sig.Args {
			params[i] = FnParam{Type: t}
		}
		if fnSigSatisfiesSpec(FnSig{Params: params, Returns: sig.Returns}, want) {
			return true
		}
	}
	return false
}
