package core

// Named function-SHAPE types — `def IntToStr fnsig Integer String`,
// then `def h fn g:IntToStr String [(g 5)]`.
//
// This closes the last gap in "Function is opaque": the structural
// subtyping rule already existed (FnSigSatisfiesSpec — contravariant
// params, covariant returns) and Unify already routed an anonymous
// constraint through it (unify_fnsig.go), so `f/v is (fnsig …)` worked.
// What did not work was DISPATCH against a NAMED fn-shape type, for
// exactly the reason the disjunct / negation / DepScalar branches of
// InstallType document: a minted lattice node is on no value's parent
// chain, so the plain lattice walk rejects every function. Those three
// branches each install a Behavior to bridge it; this is the fourth,
// and the last metatype that was missing one.
//
// Two matching paths, mirroring unify_disjunct.go and unify_negation.go:
//
//  1. Anonymous constraint value (`f/v is (fnsig Integer String)`):
//     the FunctionSignature value is the RHS and Unify's FnUndef fold
//     dispatches to unifyFnUndefShape.
//  2. Named fn-shape type (`def IntToStr fnsig Integer String` then
//     `g:IntToStr`): InstallType attaches a FnUndefUnifier carrying the
//     signature specs, so every Is/Match call site — `is`, parameter
//     dispatch, and the checker's carrier test alike — consults the
//     same structural rule.

// FnUndefUnifier is the kernel-installed Behavior for named fn-shape
// types. Like DisjunctUnifier and NegationUnifier it lives on the
// minted lattice node, so "is this value an IntToStr?" resolves
// structurally rather than by ancestry.
type FnUndefUnifier struct {
	behaviorWrapper
	sigs     []FnSigSpec
	typeName string
}

// ContentMembership marks membership as a property of the VALUE's
// content (its signatures), not of its lattice position — the same
// declaration the disjunct and negation unifiers make.
func (*FnUndefUnifier) ContentMembership() {}

// Match admits v iff v is a function value with at least one overload
// satisfying every declared signature. A bare type literal (the type
// itself, not an inhabitant) passes through to the prev/Default walk,
// mirroring DisjunctUnifier — `IntToStr is Type` must stay true.
func (f *FnUndefUnifier) Match(v Value, t *Type) bool {
	if IsBareTypeNode(v) {
		return baseBehavior(f.prev).Match(v, t)
	}
	// A carrier (abstract value) carries no signatures to inspect. Admit
	// it when it could still be a function, so the checker does not
	// reject a program the runtime accepts — the sound over-approximation
	// the negation unifier makes for the same reason.
	if v.Carrier {
		return v.Parent.ConformsTo(TFunction) || v.Parent.Equal(TAny)
	}
	return FnUndefMatchesFnDef(NewValueRaw(TFnUndef, FnUndefInfo{Sigs: f.sigs}), v)
}

// installFnUndefUnifier attaches a FnUndefUnifier to def, wrapping any
// existing Behavior. Called by InstallType when minting a named
// fn-shape type so the signature specs drive every Is/Match call site.
func installFnUndefUnifier(def *Type, sigs []FnSigSpec, name string) {
	def.ensureTMeta().Behavior = &FnUndefUnifier{
		behaviorWrapper: behaviorWrapper{prev: def.Behavior()},
		sigs:            sigs,
		typeName:        name,
	}
}
