package eng

// Function-signature comparison helpers — the single home for FnSig
// shape-matching across the engine. Two distinct rules live side by
// side:
//
//   - **Exact match** (FnSigMatchesSpec) — same arity AND pairwise
//     Type.Equal on params and returns. Used by `undef name fn [spec]`
//     to identify the precise previously-installed signature to remove.
//   - **Structural subtyping** (FnSigSatisfiesSpec) — same arity,
//     contravariant inputs (`spec ⊆ sig`) and covariant returns (`sig
//     ⊆ spec`). Used by `type Foo fn [...]` constraint matching via
//     `FnDefHasSig` / `FnUndefMatchesFnDef`.
//
// A FunctionSignature value (type Type/FunctionSignature) carries a
// list of FnSigSpec entries — each one a (Params, Returns) pair
// without a body. It's produced by `fnsig [[input] [output] …]` and
// acts as a structural function-shape constraint. Pattern
// (FnParam.Pattern) and Optional/BarrierPos
// differences are not yet considered.

// FnSigMatchesSpec returns true if a FnSig matches a FnSigSpec
// exactly: same arity, same param types pairwise, same return types
// pairwise. Variance is intentionally NOT applied — `undef name fn
// [spec]` names a specific shape to discard.
func FnSigMatchesSpec(sig FnSig, spec FnSigSpec) bool {
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

// FnSigSatisfiesSpec returns true if a candidate FnSig satisfies a
// FnSigSpec under structural function subtyping:
//
//   - **Inputs are contravariant on Type.** Each spec param type
//     must be a subtype of the candidate's param type at the same
//     position. Example: spec=[Integer], sig=[Number] — sig accepts
//     Integer (because Integer ⊂ Number), so it satisfies the spec.
//   - **Returns are covariant.** Each candidate return type must be
//     a subtype of the spec's return type at the same position.
//     Example: spec=[Number], sig=[Integer] — sig produces Integer
//     which is also a Number, so it satisfies the spec.
//   - **Optional alignment.** If spec.Params[i].Optional is true the
//     spec may omit arg i; the candidate must therefore also accept
//     omission (sig.Params[i].Optional must be true). The reverse
//     (sig optional, spec required) is fine — the candidate accepts
//     a superset of call shapes.
//   - **Pattern compatibility.** When the spec declares a Pattern
//     for arg i, the candidate's Pattern (if any) must accept every
//     value the spec admits — i.e., the spec's pattern must unify
//     with the candidate's. A spec without a pattern is satisfied
//     by any candidate (pattern absence = no extra constraint).
//
// BarrierPos is intentionally NOT compared — FnSigSpec doesn't
// carry one (it's a body-level collection setting, not part of the
// structural shape), so the type system can't declare a barrier
// requirement. Candidates may have any BarrierPos.
func FnSigSatisfiesSpec(sig FnSig, spec FnSigSpec) bool {
	if len(sig.Params) != len(spec.Params) {
		return false
	}
	for i := range sig.Params {
		sp := spec.Params[i]
		sg := sig.Params[i]
		// Contravariant: spec_input must be a subtype of sig_input.
		// `t.Matches(pattern)` is true iff t ⊆ pattern in the type
		// lattice, so spec.Type.Matches(sig.Type) checks spec ⊆ sig.
		if !sp.Type.Matches(sg.Type) {
			return false
		}
		// Optional alignment: spec-optional → candidate must also be
		// optional. spec-required → candidate may be either.
		if sp.Optional && !sg.Optional {
			return false
		}
		// Pattern compatibility: when the spec demands a pattern,
		// the candidate's pattern must accept everything the spec
		// admits. Spec.Pattern == nil means no extra constraint.
		if sp.Pattern != nil {
			if sg.Pattern == nil {
				// Candidate doesn't constrain the arg at all, but
				// the spec does — the candidate's broader contract
				// still satisfies the spec's narrower demand.
				continue
			}
			if _, ok := Unify(*sp.Pattern, *sg.Pattern); !ok {
				return false
			}
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

// FnUndefMatchesFnDef reports whether the candidate function value
// (TFnDef or TFunction wrapping FnDefInfo) satisfies every FnSigSpec
// declared by the FnUndef constraint.
func FnUndefMatchesFnDef(undef Value, fnVal Value) bool {
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
		if !FnDefHasSig(fnDef, want) {
			return false
		}
	}
	return true
}

// FnDefHasSig reports whether the candidate has at least one
// signature that satisfies `want` under structural subtyping. Both
// AQL-defined Sigs (with FnParam payload) and compiled Signatures
// (with raw Type payload) are considered so Go-implemented words can
// also satisfy a FnUndef type. The variance rule is delegated to
// FnSigSatisfiesSpec.
func FnDefHasSig(fnDef FnDefInfo, want FnSigSpec) bool {
	for _, s := range fnDef.Sigs {
		if FnSigSatisfiesSpec(s, want) {
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
		if FnSigSatisfiesSpec(FnSig{Params: params, Returns: sig.Returns}, want) {
			return true
		}
	}
	return false
}
