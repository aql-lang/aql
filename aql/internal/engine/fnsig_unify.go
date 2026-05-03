package engine

// Function-signature unification helpers.
//
// A FnUndef value (Type/Word/__UF) carries a list of FnSigSpec entries
// — each one a (Params, Returns) pair without a body. It is produced
// by `fn [[input] [output]]` (and registered as a type via `type Foo
// fn [...]`) and acts as a structural function-shape constraint.
//
// The first slice uses **exact-match** semantics: every FnSigSpec in
// the FnUndef must be matched by some FnSig in the candidate
// FnDefInfo's Sigs slice (or one of the compiled Signatures, for
// Go-implemented words), where "match" means same arity and pairwise
// Type.Equal on each param type and each return type. The follow-up
// will add variance / overload-subset / pattern-aware rules — those
// are sketched in the commit message accompanying this file.

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
// signature that matches `want` exactly. Both AQL-defined Sigs (with
// FnParam payload) and compiled Signatures (with raw Type payload)
// are considered so Go-implemented words can also satisfy a FnUndef
// type. The exact-match shape is delegated to fnSigMatchesSpec
// (defined alongside `undef`'s targeted-signature removal).
func fnDefHasSig(fnDef FnDefInfo, want FnSigSpec) bool {
	for _, s := range fnDef.Sigs {
		if fnSigMatchesSpec(s, want) {
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
		if fnSigMatchesSpec(FnSig{Params: params, Returns: sig.Returns}, want) {
			return true
		}
	}
	return false
}
