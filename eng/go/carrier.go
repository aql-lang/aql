package eng

import (
	core "github.com/boru-lang/boru/core/go"

	"errors"
	"sort"
	"strconv"
	"strings"
)

// Carrier-based static type-checking support.
//
// A "carrier" is a normal Value with Carrier=true and (typically)
// Data=nil: it carries only type information, not a concrete payload.
// The engine is driven in check mode by Registry.Check.Mode. In that
// mode, the same dispatch machinery (matchSignature, forward
// collection, sort order, etc.) runs, but execMatch consults
// Signature.Returns to synthesise carrier results instead of calling
// the handler. This keeps runtime and checker in absolute parity.
//
// This file contains only the minimal helpers needed for the initial
// slice: a conversion from concrete literal values to carriers, and a
// carrier-result builder for a matched signature.

// NewDynamicCarrierValue promotes an existing carrier value (e.g. a
// disjunct carrier for dynamic(A tor B), or a narrowed bound) to the
// dynamic modality, preserving its Parent/Data bound.
func NewDynamicCarrierValue(bound Value) Value {
	bound.Carrier = true
	bound.Dynamic = true
	return bound
}

// SpreadPayload is the payload of a "variadic spread" carrier: a residual
// entry denoting 0-or-more values of element type Elem. It exists ONLY on the
// plain-check surface (a `[]`-declared recursive fn whose body leaks a
// per-frame value — recursion.tsv:53 — cannot be modelled by a fixed-length
// residual because the depth is a runtime value) and is consumed only by the
// soundness oracle. Elem is a Value (a type literal or a disjunct of the
// per-frame leaked types), never Any/Dynamic — a variadic-Any marker would let
// the oracle admit a wrong-typed leak.
type SpreadPayload struct {
	PayloadBase
	Elem Value
}

// NewVariadicCarrier builds a variadic-spread carrier over element `elem`.
// Parent is TAny so no TList/TMap carrier machinery touches it; it is
// discriminated only via IsVariadicSpread. It is also Dynamic so that if a
// variadic result is CONSUMED by a downstream word (`m 3 add 1`) it matches
// optimistically like dynamic(Any) instead of failing dispatch — the soundness
// oracle intercepts it via IsVariadicSpread BEFORE any Dynamic check, so the
// dynamic flag never weakens the element-type coverage.
func NewVariadicCarrier(elem Value) Value {
	v := NewValueRaw(TAny, SpreadPayload{Elem: elem})
	v.Carrier = true
	v.Dynamic = true
	return v
}

// IsVariadicSpread reports whether v is a NewVariadicCarrier, returning its
// element value.
func IsVariadicSpread(v Value) (Value, bool) {
	if sp, ok := v.Data.(SpreadPayload); ok {
		return sp.Elem, true
	}
	return Value{}, false
}

// stackHasVariadic reports whether any entry of a residual stack is a
// variadic-spread carrier.
func stackHasVariadic(stk []Value) bool {
	for _, v := range stk {
		if _, ok := IsVariadicSpread(v); ok {
			return true
		}
	}
	return false
}

// FoldVariadicArms models a plain-check `if` whose arm residual contains a
// variadic-spread carrier — the shape a `[]`-declared recursive fn's body
// produces once its self-call's in-flight bail is seeded with a variadic (see
// AnalyseFnBody). It folds BOTH arms into one variadic spread whose element
// joins every per-frame LEAKED type: the non-None, non-variadic slot types
// (the fixed lead below the recursive tail, e.g. `n mul 2` → Integer) plus the
// variadic arms' own elements, dropping Never (the seed). Returns ok=false when
// neither arm carries a variadic, so the ordinary if-join then applies.
func FoldVariadicArms(then, els []Value) (Value, bool) {
	hasVar := false
	var elems []Value
	for _, stk := range [][]Value{then, els} {
		for _, v := range stk {
			if e, ok := IsVariadicSpread(v); ok {
				hasVar = true
				if e.Parent != nil && !ValueType(e).Equal(TNever) {
					elems = append(elems, e)
				}
				continue
			}
			if v.Parent == nil || v.Parent.Equal(TNone) {
				continue // None padding / base-case emptiness
			}
			elems = append(elems, NewTypeLiteral(v.Parent))
		}
	}
	if !hasVar {
		return Value{}, false
	}
	elem := NewTypeLiteral(TNever)
	if len(elems) > 0 {
		elem = elems[0]
		for _, e := range elems[1:] {
			elem = UnionType(elem, e) // drops Never, dedups, keeps a disjunct for genuine unions
		}
	}
	return NewVariadicCarrier(elem), true
}

// NewCarrierTypedList constructs a typed-list carrier — a list
// carrier whose element type is known. Implemented as a regular
// Value with Parent=TList and Data=ChildTypeInfo{Child: NewCarrier(elem)}.
// The Carrier flag is still set so the rest of the engine treats it
// as abstract. Downstream list-consuming words can recover the
// element carrier via dataListElemType.
func NewCarrierTypedList(elem *Type) Value {
	v := NewTypedList(NewCarrier(elem))
	v.Carrier = true
	return v
}

// NewCarrierTypedListValue constructs a typed-list carrier whose
// element is an arbitrary carrier Value. Use this when the element
// itself is a typed list (nested lists), a disjunct, or otherwise
// needs more structure than a bare Parent.
func NewCarrierTypedListValue(child Value) Value {
	v := NewTypedList(child)
	v.Carrier = true
	return v
}

// NewCarrierTypedListLen constructs a typed-list carrier with a
// statically-known length, so a downstream index check can reason
// about a computed list (e.g. `iota n`). n MUST be the exact length
// or an upper bound — never an underestimate (see ChildTypeInfo.Len).
func NewCarrierTypedListLen(elem *Type, n int) Value {
	v := NewCarrierTypedList(elem)
	if ct, ok := v.Data.(ChildTypeInfo); ok {
		ln := n
		ct.Len = &ln
		v.Data = ct
	}
	return v
}

// ReturnsPreserveListAt builds a ReturnsFunc that returns a typed-
// list carrier whose element type matches the data-list arg at
// index i. Used by list-preserving words like reverse, take, shed,
// unique, at, sortby — they return a list of the same element type
// as their input.
func ReturnsPreserveListAt(i int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		if i < 0 || i >= len(args) {
			return []Value{NewCarrier(TList)}
		}
		elem := DataListElemTypeFromValue(args[i])
		out := NewCarrierTypedList(elem)
		// Copy the source's element constraint onto the residual so the check-mode
		// write mirror fires: d2CheckWrite consults ElemConstraint (the elem
		// pointer), which NewCarrierTypedList sets in ChildTypeInfo.Child but not
		// as the pointer — so `(reverse xs) set 0 "bad"` for xs:[:Integer] is
		// diagnosed at check, mirroring the tagged runtime result (#9, round 9).
		if ec, ok := args[i].ElemConstraint(); ok {
			out.SetElemConstraint(ec)
		}
		return []Value{out}
	}
}

// ReturnsListElemAt builds a ReturnsFunc that returns the element
// type carrier of the data-list arg at index i. Used by words like
// head/first (if added) that pick a single element out of a list.
func ReturnsListElemAt(i int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		if i < 0 || i >= len(args) {
			return []Value{NewCarrier(TAny)}
		}
		elem := DataListElemTypeFromValue(args[i])
		return []Value{NewCarrier(elem)}
	}
}

// NewElementCarrier builds the per-invocation element carrier a higher-order
// body (each / fold / scan / filter) sees. When the element type is UNKNOWN —
// TAny from an UNTYPED list (`Test.results end` declares `[List]`, so there is no
// ChildTypeInfo to read the element from) — the carrier is DYNAMIC, so a
// downstream access in the body (`get`, a field read) matches OPTIMISTICALLY
// instead of failing `no_signature` against the bare Any root, exactly as a
// declared-Any return does (carrierResults). A KNOWN element type stays a strict
// carrier — its real shape is checked normally. Sound under the dynamic-modality
// framework: it only loosens matching, and a guard discharges it back to strict.
func NewElementCarrier(t *Type) Value {
	c := NewCarrier(t)
	if t == nil || t.Equal(TAny) {
		c.Dynamic = true
	}
	return c
}

// ElementCarrierFromValue is the check-mode carrier a higher-order body sees
// for one element of data. For a CONCRETE heterogeneous list (or map, whose
// value-bodies see the values) the element is the lattice JOIN of the element
// types — built via JoinCarriers, the same join branch merges use: direct
// siblings collapse to the shared parent, DISTANT cousins stay a strict
// Disjunct, so the body dispatch distributes per alternative
// (disjunctPartitionReturns) exactly as the runtime dispatches per element —
// `[1 2 "s"] each [1 add]` matches add's Integer AND String overloads instead
// of failing no_signature against the Scalar ancestor. Every other shape
// (typed lists, empty/single-typed collections, carriers) keeps
// NewElementCarrier(DataListElemTypeFromValue(data)) — an untyped element
// stays a dynamic Any.
func ElementCarrierFromValue(data Value) Value {
	if IsConcrete(data) {
		if joined, ok := joinedElementCarrier(data); ok {
			return joined
		}
	}
	return NewElementCarrier(DataListElemTypeFromValue(data))
}

// joinedElementCarrier joins the element types of a concrete plain list (or
// the value types of a concrete map) via JoinCarriers. ok=false when the
// collection is empty, single-typed (the plain-type path is already precise),
// or not a plain list/map payload.
func joinedElementCarrier(data Value) (Value, bool) {
	var elems []Value
	switch p := data.Data.(type) {
	case ListPayload:
		elems = p.Elems
	case MapPayload:
		if p.M == nil {
			return Value{}, false
		}
		for _, k := range p.M.Keys() {
			v, _ := p.M.Get(k)
			elems = append(elems, v)
		}
	default:
		return Value{}, false
	}
	if len(elems) < 2 || elems[0].Parent == nil {
		return Value{}, false
	}
	mixed := false
	for i := 1; i < len(elems); i++ {
		if elems[i].Parent == nil {
			return Value{}, false
		}
		if !elems[i].Parent.Equal(elems[0].Parent) {
			mixed = true
		}
	}
	if !mixed {
		return Value{}, false
	}
	out := NewCarrier(elems[0].Parent)
	for i := 1; i < len(elems); i++ {
		out = JoinCarriers(out, NewCarrier(elems[i].Parent))
	}
	return out, true
}

// ParamInputCarrier builds the check-mode carrier for a fn parameter declared of
// type t. An EXPLICITLY-Any (or untyped) parameter binds a DYNAMIC carrier: the
// author wrote "accepts anything", which is the gradual-dispatch intent — a body
// word over it (`get`, `add`, a user helper) poly-matches at runtime instead of
// failing no_signature against the strict Any top. A concrete declared type
// stays a strict carrier so its real shape is checked normally. This is the
// same treatment NewElementCarrier gives an untyped list element, lifted to the
// parameter boundary (the trie/decision "unify Any with concrete params" gap).
func ParamInputCarrier(t *Type) Value {
	if t == nil || t.Equal(TAny) {
		return NewDynamicCarrier(TAny)
	}
	// A named-union param (`x:T` with `def T (Integer tor String)`) binds
	// the DISTRIBUTING disjunct carrier, not a bare T-tagged one — the same
	// multi-denotation rule as the declared-return side (UnionCarrierForType):
	// install-time body analysis then dispatches `x add x` per alternative.
	// Marked Declared: the param annotation claims every alternative is a
	// valid input, so a body dispatch that fails for one is an ERROR
	// (disjunctPartitionReturns), not the analysis-join partial warning.
	if dv, ok := UnionCarrierForType(t); ok {
		if di, isDi := dv.Data.(DisjunctInfo); isDi {
			di.Declared = true
			dv.Data = di
		}
		return dv
	}
	return NewCarrier(t)
}

// UnionCarrierForType returns the DISTRIBUTING carrier for a user-defined
// union/enum type — a strict Disjunct of the type's alternatives, the exact
// shape a branch join of distant cousins produces (JoinCarriers), so
// sigTypeMatches' strict-disjunct branch and disjunctPartitionReturns treat
// it identically. ok=false for any type without a disjunctUnifier Behavior.
// This is the third multi-denotation carrier shape (after dynamic carriers
// and payload-bearing joins); the distribute-over-dispatch invariant
// (TestDistributeOverDispatchInvariant) pins all of them.
func UnionCarrierForType(t *Type) (Value, bool) {
	if t == nil {
		return Value{}, false
	}
	du, ok := t.Behavior().(*DisjunctUnifier)
	if !ok || len(du.Alternatives) == 0 {
		return Value{}, false
	}
	dv := NewDisjunct(SimplifyDisjunctAlts(du.Alternatives))
	dv.Carrier = true
	return dv, true
}

// DataListElemTypeFromValue is a package-level duplicate of
// dataListElemType that lives in carrier.go so ReturnsFunc helpers
// don't depend on the native_array_higher.go symbol. It reads the
// ChildTypeInfo first, then joins concrete element VTypes.
func DataListElemTypeFromValue(data Value) *Type {
	if data.Data == nil {
		return TAny
	}
	if ct, ok := data.Data.(ChildTypeInfo); ok {
		if ct.Child.Data == nil && !ct.Child.Carrier {
			c := ct.Child // bare type-literal child IS the element type
			return &c
		}
		return ct.Child.Parent
	}
	// A concrete MAP: the higher-order words (each/filter/fold over a map)
	// transform the VALUES (keys are kept), so the element type a value-body
	// closure sees is the common value type, not the list-element type below.
	if mp, ok := data.Data.(MapPayload); ok {
		if mp.M == nil || mp.M.Len() == 0 {
			return TAny
		}
		var t *Type
		for _, k := range mp.M.Keys() {
			v, _ := mp.M.Get(k)
			if t == nil {
				t = v.Parent
			} else {
				t = CommonAncestorType(t, v.Parent)
			}
			if t.Equal(TAny) {
				break
			}
		}
		if t == nil {
			return TAny
		}
		return t
	}
	list, err := AsList(data)
	if err != nil || list.IsNil() || list.Len() == 0 {
		return TAny
	}
	t := list.Get(0).Parent
	for i := 1; i < list.Len(); i++ {
		t = CommonAncestorType(t, list.Get(i).Parent)
		if t.Equal(TAny) {
			break
		}
	}
	return t
}

// toCarrier converts a concrete Value to its carrier form. Control /
// structural tokens (words, marks, moves, open-paren, paren-expr,
// interp-string, return-check, def-cleanup, forward) are returned
// unchanged: they drive dispatch and must retain their payloads.
// Lists and maps are returned unchanged for now so that list/map
// signature matching keeps working; carrier-aware list/map handling
// is future work.
func toCarrier(v Value) Value {
	if IsWord(v) || IsForward(v) || IsMark(v) || IsMove(v) ||
		IsOpenParen(v) || IsParenExpr(v) || IsInterpString(v) || IsXmlInterp(v) ||
		IsReturnCheck(v) || IsDefCleanup(v) || IsDispatchMod(v) {
		// A `/r`/`/q` dispatch-modifier marker (Word/__DM, parser-emitted right
		// after a paren / dotted-path group) is a control token too: stripping
		// it to a payload-less carrier made check mode UNABLE to consume it
		// (execFnDefLiteral's peek reads the DispatchModInfo) or drop it
		// standalone (stepLiteral's IsDispatchMod drop) — the marker then
		// leaked into the check stack as a phantom value the runtime never
		// has (`([x:Any] => [x])/r is T` bound `is` to the marker instead of
		// the lambda). Kept verbatim, check mode parks/drops it exactly as
		// the interpreter does (Stage M2d).
		return v
	}
	// A dynamic carrier already IS a carrier; its Parent/Data is the
	// gradual bound (which may be a disjunct). Return it unchanged so
	// stripping never nulls the bound or clears the Dynamic flag.
	if v.Dynamic {
		return v
	}
	// Keep lists and maps concrete for now — matchSignature relies
	// on Data presence for a few compound cases.
	if v.Parent.Equal(TList) || v.Parent.Equal(TMap) {
		return v
	}
	// Keep concrete inert SCALAR literals concrete so the recorder can recover
	// the value. Two drivers: static index checking (an out-of-bounds literal
	// index like `[10 20] 5 getr` needs the integer), and const-baking a DATA
	// list/map whose interior is scalar (`each $.name [{name:"a"}]`, `size
	// people`, a string-bearing table row). Without this the interior strips to
	// a type-only carrier, so the container is no longer an inert const and the
	// operand has no compiled home. Only GENUINE concrete scalars qualify — a
	// DepScalar CONSTRAINT (`Integer gt 10`, `String len 5`) carries a
	// DepScalarInfo payload, not one of these, so it is naturally excluded and
	// stays a carrier for predicate matching. Precision only increases: a
	// literal stays concrete until a word consumes it and produces a computed
	// carrier, exactly as lists/maps already behave.
	switch v.Data.(type) {
	case IntPayload, StrPayload, BoolPayload, FloatPayload, AtomPayload,
		BigIntPayload, DecimalPayload, TimePayload, DurationPayload, TimezonePayload:
		if IsConcrete(v) {
			return v
		}
	}
	// Keep FnDef / Function payloads (FnDefInfo) concrete. Stripping
	// them loses the body, params, and Captured list — which means a
	// factory call producing a closure (`def add5 (make-adder 5)`) in
	// check mode would otherwise bind add5 to an empty carrier rather
	// than to the inner FnDefInfo, breaking subsequent invocation +
	// inference of `add5 3`.
	if _, ok := v.Data.(FnDefInfo); ok {
		return v
	}
	// Keep Disjunct / Enum values concrete: their DisjunctInfo (the
	// alternatives) IS the type definition. Stripping to a bare TDisjunct /
	// TEnum carrier loses the alternatives, so IsDisjunct / IsTypeBody go false
	// and `def Maybe (String tor None)` / `def Color enum […]` are wrongly
	// rejected in check mode ("body must be a type value or literal"), and
	// `tcmp` / `is` / `typeof` over the type lose their members (compare.tsv).
	// Same rationale as the FnDef / Module / Reach payload preservations above.
	if _, ok := v.Data.(DisjunctInfo); ok {
		return v
	}
	// Keep DEPSCALAR constraints concrete: their DepScalarInfo (the bounds)
	// IS the type definition. Stripping to a bare base-scalar carrier loses
	// the constraint, so a type-algebra meet over named refinements
	// (`A tand B`) degraded to a plain Integer and a typed-def annotation
	// built from a tand/tor expression lost its bounds in check mode. Same
	// rationale as the Disjunct / Enum preservation above.
	if _, ok := v.Data.(DepScalarInfo); ok {
		return v
	}
	// Keep generic SCHEMA values concrete: their *TypeSchemaInfo IS the type
	// definition (the parameters + body `of` instantiates). Stripping it to a
	// bare carrier loses the schema, so IsTypeSchema goes false and `of`
	// rejects it — e.g. a schema exported from a module and read back through
	// `Pkg.Box` (whose namespace-map get returns the stored value, carrier-
	// stripped) could no longer be instantiated `Pkg.Box of [Integer]`. Same
	// rationale as the FnDef / Disjunct / Module payload preservations above.
	if _, ok := v.Data.(*TypeSchemaInfo); ok {
		return v
	}
	// Keep MODULE values concrete, same rationale as FnDefInfo: stripping
	// nulls the Ideal/Module descriptor's ExtensionPayload, so
	// `MathUtil.$module` would become an opaque carrier the get-resolution
	// elision can no longer follow. They are immutable and import-bound, so
	// a pure read of one (`$name`, `$module.name`, `convert Map …`)
	// const-folds (tryFoldModuleConst). A module NAMESPACE needs no arm of
	// its own — it is a plain facet-carrying Map, already preserved by the
	// TMap guard above (facets ride every Value copy).
	if IsModuleFamilyValue(v) {
		return v
	}
	// Keep Reach values (dot-access `m.a`, `Pkg.fn`) concrete. A parsed
	// dot-access is a Reach whose ReachInfo carries the Eval flag and the
	// get/getr segments; the engine expands it in place at step time
	// (isEvalReach → expandReach). Stripping would null the ReachInfo,
	// so isEvalReach goes false and the chain never expands — dot-access
	// would be opaque in check mode, silently dropping module-export type
	// propagation, index checks, and dispatch diagnostics. Same rationale
	// as the FnDefInfo case above.
	if IsReach(v) {
		return v
	}
	// Keep an __SP splice marker concrete: it is a compile-time macro binding
	// (`def x word [body]`), expanded inline at each use site by stepLiteral. A
	// carrier-stripped marker would lose its payload and never splice, so the
	// reference would be an opaque carrier in check mode.
	if IsSplice(v) {
		return v
	}
	// Keep a sugar marker concrete: the parser's structural desugar
	// output (ADR-012 rule 3, 2026-08-04 amendment), lowered at step
	// time by stepSugar. A carrier-stripped marker loses its SugarInfo
	// and could never lower — the sugar would be opaque in check mode.
	if IsSugar(v) {
		return v
	}
	// Type literals (Data already nil) are already in the right
	// shape for sig matching — preserve their Carrier=false marker
	// so sigTypeMatchesAsType can still recognise them as type
	// literals rather than as value-carriers. Without this guard,
	// `Integer gt 10` under check mode loses the Integer
	// type-literal distinction and falls through to the boolean
	// sig instead of the dep-constructor sig. See depscalar.go's
	// MakeDepScalarSig + RunInCheckMode for the matching change.
	if v.Data == nil {
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

// checkModeLiteralWords are words whose adjacent String literal argument
// must survive check-mode carrier-stripping because the handler needs the
// concrete value to do anything useful. `import`/`module` resolve a path
// or module name — without it the checker can't load the target's exports
// and every cross-module reference flags a spurious undefined_word (§4.3).
var checkModeLiteralWords = map[string]bool{
	"import": true,
	"module": true,
	// `unpack 'boru:mod'` / `unpack ExportName 'boru:mod'` resolve a module's
	// (statically-declared) exports and bind them unqualified in check mode;
	// the module-name string must stay concrete for the handler to resolve it.
	// The export-name form puts the string two tokens after `unpack`, so the
	// adjacency window below looks back/forward by two, not one.
	"unpack": true,
}

// StripToCarriers returns a copy of in where every non-structural value
// has been converted to its carrier form. Used at the top-level Run()
// entry to bootstrap check-mode execution.
//
// Exception: a String literal directly adjacent to an `import` / `module`
// word (either order — forward `import "x"` or stack `"x" import`) is kept
// concrete so the import handler can resolve and analyse the target. See
// checkModeLiteralWords and §4.3.
func StripToCarriers(in []Value) []Value {
	out := make([]Value, len(in))
	for i, v := range in {
		if v.Parent.ConformsTo(TString) && IsConcrete(v) && adjacentToLiteralWord(in, i) {
			out[i] = v
			continue
		}
		out[i] = toCarrier(v)
	}
	return out
}

// adjacentToLiteralWord reports whether the token at index i has an
// immediate neighbour that is a checkModeLiteralWords word.
func adjacentToLiteralWord(in []Value, i int) bool {
	// A window of two each way: `import "x"` / `"x" import` are immediate, but
	// `unpack ExportName 'boru:mod'` places the module-name string two tokens
	// after the literal word. Keeping a string concrete is sound regardless
	// (it only adds precision), so the slightly wider window is harmless for
	// the rare non-literal-word case it might also catch.
	for d := 1; d <= 2; d++ {
		if i-d >= 0 && isLiteralWord(in[i-d]) {
			return true
		}
		if i+d < len(in) && isLiteralWord(in[i+d]) {
			return true
		}
	}
	return false
}

func isLiteralWord(v Value) bool {
	if !IsWord(v) {
		return false
	}
	w, _ := AsWord(v)
	return checkModeLiteralWords[w.Name]
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
// args that would be passed to the runtime handler). pos carries the
// word's source location so diagnostics can point at it.
//
// tailConsumed reports that this dispatch sits in PAREN-TAIL position —
// the token immediately after the call is a CloseParen, so the result is
// the group's value, consumed by whatever encloses the group (a `def`
// binding, an outer word's arg). It gates the mixed-arity gradual-arity
// model under a real compile (see the `len(out) == 0` branch): only a
// consumed result may optimistically model one dynamic value, because a
// free statement-position residual would be collected by the NEXT word
// and corrupt its arity. The dynamic-recovery callers pass false.
func carrierResults(r *Registry, word string, sig *Signature, args []Value, pos SrcPos, ownerReg *Registry, tailConsumed bool) []Value {
	if out, ok := specialWordResults(r, word, args, pos); ok {
		return out
	}
	narrowDynamicUses(r, word, sig, args)
	// Per-alternative dispatch for strict disjunct inputs
	// (design/checker-accuracy-review.10.md A1). matchSignature tested
	// the disjunct as a single value, so the matched sig may not be
	// the one runtime dispatch takes for every alternative — e.g.
	// Integer|String reaches add's [Scalar Scalar]→String catch-all
	// although the Integer path takes [Number Number]. Resolve each
	// alternative independently and join the per-alternative returns.
	var partOut []Value
	if out, ok := disjunctPartitionReturns(r, word, args, pos); ok {
		// A strict-disjunct straddle is a runtime-dispatch case, not an
		// inherent refusal: if the word is a safe poly candidate (core builtin,
		// no meta/fn-value/code-body sig) and its operands resolve, lower it to
		// OpCallNativePoly so the VM re-matches the one concrete alternative at
		// run time — e.g. `5 is (tnot (Integer gt 0))`. A USER FN under an
		// ARMED recording instead falls THROUGH to the ordinary dispatch
		// below — one recorded CALL_USER with the ORIGINAL (provenance-
		// carrying) args; the runtime value is one concrete alternative the
		// unit's param contract admits — and the partition-joined carriers
		// are re-IDed onto the recorded results at the tail, keeping the
		// per-alternative type precision without orphaning the residual
		// (the L-JOIN recursive-union refusal). Everything else refuses.
		if r.Check.Recorder().Active() && sig != nil && sig.FnFrame() != nil &&
			disjunctCombosTakeSig(r, word, args, sig) {
			partOut = out
		} else {
			if !dispatchTryRecordPoly(r, word, sig, args, out, pos, true, ownerReg, false, nil) {
				r.Check.Recorder().RecordPoly(word)
			}
			return out
		}
	}
	out := declaredReturnCarriers(r, word, sig, args, pos)
	if folded, ok := dispatchTryFoldScalarConst(r, sig, args); ok && len(out) == 1 {
		// Concrete-condition folding (CompileScalarFold): the comparison ran
		// for real over compile-time-known scalars, so the concrete result
		// replaces the declared-type carrier — `(n eq 0)` with a const n
		// reads as a known false downstream (return-membership conformance,
		// future static branch commitment). Unlike tryFoldModuleConst the
		// dispatch still RECORDS below: the VM re-runs the comparison
		// faithfully at run time, and eliding the event would strand every
		// lowering that anchors on a condition EVENT (computed-arm `if`,
		// variadic-else claims — the emit goldens pin this).
		folded.ID = out[0].ID // keep the recorder's result identity
		out[0] = folded
	}
	out = applyGradualContagion(r, word, args, out, pos, tailConsumed)
	dispatchRecordOutcome(r, word, sig, args, out, pos, ownerReg)
	// The partitioned user-fn dispatch (above): hand back the partition-
	// joined carriers under the RECORDED results' identities, so downstream
	// operand resolution reaches the recorded call while the checker keeps
	// the per-alternative precision. A count mismatch keeps the recorded
	// results verbatim — sound, just wider.
	if partOut != nil {
		if len(partOut) != len(out) {
			return out
		}
		for i := range partOut {
			partOut[i].ID = out[i].ID
		}
		return partOut
	}
	return out
}

// scalarFoldOperand reports whether a check-mode dispatch operand carries a
// compile-time-known SCALAR value a CompileScalarFold dispatch may fold
// over: an inert const, or a check-mode literal — which rides as a
// concrete-PAYLOAD carrier (Carrier=true, Data set; the same shape the
// DepScalar predicate evaluation reads for `f 5`), so the isInertConst
// carrier guard alone would reject it. The carrier-tolerant arm admits only
// value-scalar payloads: a compound carrier's payload is structural
// (ChildTypeInfo) and a dynamic carrier's value is unknown — both decline.
func scalarFoldOperand(v Value) bool {
	if IsInertConst(v) {
		return true
	}
	if v.Dynamic || v.Data == nil {
		return false
	}
	switch v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		BigIntPayload, DecimalPayload:
		return true
	}
	return false
}

// specialWordResults handles the words carrierResults special-cases before
// ordinary return resolution: the `args` frame projection, the `word` splice
// marker, and compile-time `macroexpand` folding. ok=false falls through to
// the normal path (including macroexpand's error/trap fall-through, which
// must still resolve returns and refuse).
func specialWordResults(r *Registry, word string, args []Value, pos SrcPos) ([]Value, bool) {
	// `args` inside a compiled fn body projects the frame's params (pushed by
	// AnalyseFnBody) as a list value with NO recorded event. An `args.N` access
	// then folds to param N — a frame local — via tryFoldStaticIndex; bare
	// `args` has no foldable consumer and refuses at its use site. At top level
	// r.Args is empty so `args` falls through to RecordCall's refusal.
	if word == "args" {
		// Inside a CLOSURE body compile the projection must DECLINE: the
		// analysis args frame there holds the CallableSpec inputs (empty for
		// `do`), while at run time the body executes through InvokeBody in
		// the ENCLOSING call context, so the interpreter's `args` reads the
		// enclosing fn's per-call list. Projecting the closure frame baked
		// that wrong (often empty) list as an inert const — `do [args]`
		// inside a fn compiled to PUSH_CONST [] against the interpreter's
		// [7]. Falling through reaches RecordCall's context-dependent-word
		// refusal, the closure probe declines, and the program takes the
		// interpreter fallback with the correct value. A plain (non-
		// recording) check keeps the projection so diagnostics are unchanged.
		if es := r.Check.Recorder(); es.Active() && es.InClosureUnit() {
			return nil, false
		}
		if top, ok, err := r.Args.Top(); err == nil && ok && IsConcrete(top) {
			// In compile mode `args.N` folds to a frame local (PUSH_LOCAL N)
			// for named AND unnamed params alike: an unnamed param is a
			// local re-pushed onto the operand stack at unit entry, and the
			// copy the body never consumes is discarded by the RET's
			// NUnnamed trim — the exact __RC frame discipline — so the fold
			// no longer strands a divergent count (the former "args over a
			// frame with unnamed params" refusal).
			return []Value{top}, true
		}
	}
	// `word [body]` is a compile-time macro splice: produce the __SP marker as a
	// non-emitting value (no runtime op). At its use site stepLiteral splices the
	// body inline and re-steps it against the live stack, so the expansion
	// compiles in place — late binding and all. (The `def NAME word …` that binds
	// the marker emits nothing either; the marker has no runtime existence.)
	if word == "word" && len(args) == 1 {
		return []Value{NewSplice(args[0])}, true
	}
	// `macroexpand (mac args…)` is Lisp-style compile-time expansion: the macro
	// and its operands are static, so run the expansion NOW and bake the
	// resulting token list as a const (code-as-data). Only when the expansion
	// is fully concrete (isInertConst — no carrier from a runtime operand) and
	// succeeds; a too-deep / erroring expansion falls through to refuse, and the
	// interpreter surfaces the same error.
	if word == "macroexpand" && len(args) == 1 {
		toks, err := ExpandMacroForm(r, args[0])
		if err == nil {
			if lst := NewList(toks); IsInertConst(lst) {
				// Determinism guard (mirrors tryFoldModuleConst /
				// constFoldContainerVal): ExpandMacroForm runs the macro template
				// through a sub-engine, which is NOT guaranteed pure — a template
				// reading now/rand/mutable state expands differently each step, and
				// isInertConst accepting the result does not imply determinism (an
				// unevaluated (now) member is inert). Bake the check-time expansion
				// only when a second expansion agrees; on mismatch fall through to
				// refuse so the interpreter expands at its own step. The probe runs
				// under a def-stack snapshot so it leaves no new side effect beyond
				// the first expansion's (which is preserved, as before).
				snap := r.Defs.Snapshot()
				toks2, err2 := ExpandMacroForm(r, args[0])
				r.Defs.Restore(snap)
				if err2 == nil && ConstFoldAgrees(NewList(toks2), lst) {
					return []Value{lst}, true
				}
			}
		} else {
			// A runaway recursive macro expands until the depth guard
			// (macro_expand.go) raises `macroexpand_error` — the interpreter
			// raises exactly this error at run time. Compile the byte-identical
			// terminal trap (top-level only — RecordTrap declines a nested
			// occurrence, which keeps the lenient fallback) so the compiled
			// program raises it instead of refusing. Mirrors the sibling
			// mini/parse/emit `*_unknown_lang` expansion-time traps. Any OTHER
			// expansion error (a malformed form, a non-macro head) is not a
			// runtime error to reproduce — it falls through to refuse, and the
			// interpreter surfaces it.
			var ae *BoruError
			if errors.As(err, &ae) && ae.Code == "macroexpand_error" {
				r.Check.Recorder().RecordTrap("macroexpand_error", ae.Detail,
					"macroexpand", ae.Hint, pos)
			}
		}
	}
	return nil, false
}

// declaredReturnCarriers is carrierResults' three-way return resolution:
// ReturnsFn (invoked with the carrier args), declared Returns (one fresh
// carrier per type, declared Any riding dynamic), or the missing_returns
// fallback (a dynamic Any so one unannotated word does not cascade false
// no_signature errors downstream).
func declaredReturnCarriers(r *Registry, word string, sig *Signature, args []Value, pos SrcPos) []Value {
	var out []Value
	switch {
	case sig.ReturnsFn != nil:
		r.Check.CurCallPos = pos // expose call site to ReturnsFn (e.g. make Array identity)
		raw := sig.ReturnsFn(resolveTypeNameArgs(args), r)
		out = make([]Value, len(raw))
		for i, v := range raw {
			out[i] = toCarrier(v)
		}
	case sig.Returns == nil:
		// Explicit nil (no annotation) triggers the fallback. An empty but
		// non-nil slice is a valid "returns nothing" declaration.
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "missing_returns",
			Detail: "word " + word + " has no declared Returns for matched signature; assuming Any",
			Word:   word,
			Row:    pos.Row,
			Col:    pos.Col,
		})
		// The assumed result is a DYNAMIC Any carrier — "type unknown",
		// not "type is the Any root". A plain Any carrier fails every
		// typed slot (Any conforms only to Any), so one unannotated
		// word used to cascade false no_signature errors through every
		// downstream consumer (`mod (size s) 4` errored on mod). The
		// dynamic bound matches optimistically, the one real diagnostic
		// (this missing_returns) remains, and a guard discharges the
		// modality back to strict.
		c := NewCarrier(TAny)
		c.Dynamic = true
		out = []Value{c}
	default:
		out = make([]Value, len(sig.Returns))
		for i, t := range sig.Returns {
			c := NewCarrier(t)
			// A declared `Any` return means "statically unknown", not
			// "inhabits only the Any root": a STRICT Any carrier
			// conforms to no typed slot, so accessor words declared
			// `Returns: [Any]` (`get`, dotted field reads) poisoned
			// every typed consumer downstream with false no_signature
			// errors (`set 0 1 f.bits` errored because `f.bits` typed
			// as strict Any). Mark it dynamic: optimistic matching,
			// discharged back to strict by a guard.
			if t.Equal(TAny) {
				c.Dynamic = true
			}
			out[i] = c
		}
	}
	return out
}

// isConcreteContainerReturn reports whether a declared-return carrier is a
// fully-determined List/Map container (not the Any root, not a dynamic
// carrier). Such a return type is fixed by the word's contract independent of
// the runtime input value, so gradual contagion need not mark it dynamic — a
// downstream `each` / `fold` / accessor can then commit on the known container
// and derive an element carrier. Scoped to containers because that is the only
// consumer that needs the concrete type; scalar returns gain nothing and their
// strict form can spuriously trip the forward/stack split detector.
func isConcreteContainerReturn(v Value) bool {
	if v.Dynamic || v.Parent == nil {
		return false
	}
	return v.Parent.ConformsTo(TList) || v.Parent.ConformsTo(TMap)
}

// applyGradualContagion widens results derived from dynamic args: the
// modality flows downstream (a guard discharges it), a single result widens
// to the union of all reachable overload returns (first-match partition),
// and a 0-return mutator over a dynamic receiver optimistically models one
// value where a value-returning sibling overload is reachable (gated to
// consumed results under a real compile — see carrierResults' doc).
func applyGradualContagion(r *Registry, word string, args []Value, out []Value, pos SrcPos, tailConsumed bool) []Value {
	// Gradual contagion (design/dynamic-modality-report.10.md): a result
	// derived from a dynamic carrier is itself dynamic, so the modality
	// flows downstream instead of dying after one dispatch. The bound is
	// the sig's declared return (the first-cut result; the full
	// first-match partition over the bound is a later slice). Sound — it
	// only loosens matching, never tightens — and a guard discharges it
	// back to strict. ReturnsFn results that are already dynamic (e.g.
	// ReturnsIdentity of a dynamic input) stay so via toCarrier.
	if anyDynamicCarrier(args) {
		// STRICT mode (`boru check --strict`): make the gradual frontier
		// loud — every committed dispatch over a dynamic operand is a
		// point the checker matched optimistically and the runtime will
		// re-verify. Non-gating info; deduped by word+position (the same
		// dispatch can be re-analysed under several call shapes).
		if r.Check.Strict {
			dup := false
			for _, d := range r.Check.Diagnostics {
				if d.Code == "dynamic_dispatch" && d.Word == word && d.Row == pos.Row && d.Col == pos.Col {
					dup = true
					break
				}
			}
			if !dup {
				r.Check.AddDiagnostic(CheckDiagnostic{
					Code:   "dynamic_dispatch",
					Detail: "strict: " + word + " dispatched over a dynamic operand — matched optimistically, re-checked at runtime",
					Word:   word,
					Row:    pos.Row,
					Col:    pos.Col,
				})
			}
		}
		// A CONCRETE, fully-determined declared CONTAINER return type does not
		// become statically-unknown just because an input was dynamic: the
		// word's contract fixes the RESULT TYPE regardless of which runtime
		// value flowed in (`StructUtil.items` always returns a `List`). The
		// dynamic input only decides whether the call SUCCEEDS or RAISES — and
		// the call itself is baked as a runtime-faithful guarded dispatch
		// (CALL_NATIVE / poly) that raises exactly as the interpreter on a bad
		// receiver, so a compiled program never observes a wrongly-typed
		// result. Keeping such a List/Map output STRICT lets a downstream
		// `each` / `fold` / accessor commit on the KNOWN container instead of
		// refusing "dynamic input at each" — the cross-module element-typing
		// flip. Scoped to CONTAINER returns on purpose: a downstream
		// higher-order/accessor word needs the concrete container to derive an
		// element carrier, whereas keeping a SCALAR return strict has no such
		// consumer and merely turns a formerly-dynamic if-arm join into a
		// non-dynamic disjunct that trips the conservative forward/stack split
		// detector (the trie/burst `String|Integer` fold-body regression).
		// Guarded to the single-reachable-return case (`keepStrict`): a
		// multi-overload word whose reachable overloads return DIVERGENT types
		// must still ride the dynamic union below (the runtime could take a
		// sibling overload's return), and a genuinely input-dependent return
		// (declared Any / a ReturnsFn that produced a dynamic carrier) stays
		// dynamic — those arrive here already flagged Dynamic and are untouched.
		var reachable []*Type
		if len(out) == 1 {
			reachable = dynamicReachableReturns(r, word, args)
		}
		keepStrict := len(out) == 1 && len(reachable) < 2
		for i := range out {
			out[i].Carrier = true
			if keepStrict && !out[i].Dynamic && isConcreteContainerReturn(out[i]) {
				continue
			}
			out[i].Dynamic = true
		}
		// First-match partition (design/dynamic-modality-report.10.md): a
		// dynamic bound can reach MULTIPLE of the word's overloads, whose
		// returns may differ. The single matched-sig return is then too
		// narrow — it would wrongly reject a downstream use of one of the
		// other reachable returns. Widen the (single) result to the union
		// of all reachable returns. No-op for the common case (one
		// reachable return), so unobservable with return-uniform words.
		// The widening MINTS a fresh identity (NewDynamicCarrierValue) — a
		// user-poly ReturnsFn that already recorded the call under the old
		// out[0].ID is re-linked by recordCallElided's poly-alias arm (the
		// §8.2(3) return-join), which rebinds the rebuilt outs onto the
		// recorded event; preserving the old ID here instead reclassified
		// an unrelated compiling flex-set row's residual ("call result
		// above a literal") — the fresh mint is load-bearing for the
		// residual model.
		if len(out) == 1 {
			if len(reachable) >= 2 {
				alts := make([]Value, len(reachable))
				for i, t := range reachable {
					alts[i] = NewTypeLiteral(t)
				}
				out[0] = NewDynamicCarrierValue(NewDisjunct(alts))
			}
		} else if len(out) == 0 && (!r.Check.Compiling || tailConsumed) {
			// The matched overload returns NOTHING (an in-place mutator) but the
			// receiver is dynamic and a value-returning sibling is also reachable —
			// `set`'s mixed-arity overloads (Array/Object/Store/Class return 0;
			// Map/List/Flex return the updated node). The runtime receiver could be
			// either, so committing to 0 values poisons every downstream CONSUMER of
			// the result (trie's `(nd "kids" get) set ch child` fed to mk-node: 0
			// values → an unbound arg → a false undefined_word on the consumer's
			// param + no_signature on the consuming call). Model ONE dynamic value —
			// the optimistic gradual arity — so a consuming use type-checks.
			//
			// In PURE check mode this always fires (no runtime, so the model is
			// free). Under a REAL compile it fires ONLY in paren-tail position
			// (tailConsumed): there the single value is immediately consumed by the
			// group close, so it can never be collected as the NEXT word's extra arg
			// — the unsoundness that forced the original !Compiling gate (a
			// statement-position `… set mem true  IO.write …`, where modeling 1
			// value made write bind the phantom as a 3rd arg and underflow at run
			// time — TestSetOverDynamicReceiverPolyCompiles). The VM's
			// OpCallNativePoly is runtime-faithful (callPoly pushes the matched
			// overload's ACTUAL results), so a consumed Map-set pushes the 1 value
			// the recorder wired; a non-value receiver would already error the
			// original program (def of 0 values), erroring alike in both engines. A
			// CONCRETE receiver reaches only its own overload, so vrets is empty and
			// the true 0-arity stands either way.
			if vrets := dynamicReachableValueReturns(r, word, args); len(vrets) > 0 {
				if len(vrets) == 1 {
					c := NewCarrier(vrets[0])
					c.Carrier = true
					c.Dynamic = true
					out = []Value{c}
				} else {
					alts := make([]Value, len(vrets))
					for i, t := range vrets {
						alts[i] = NewTypeLiteral(t)
					}
					out = []Value{NewDynamicCarrierValue(NewDisjunct(alts))}
				}
			}
		}
	}
	return out
}

// bodyFreeForFallback reports whether a code body references only words
// the VM's sub-engine can resolve at run time: registered natives /
// fn-defs (r.Lookup non-nil) and the known bare literals. A bare word
// bound by a value `def` resolves via Defs substitution, NOT Lookup, so
// it fails here — correctly, since at VM run time that binding is the
// check pass's CARRIER, not a concrete value.
func bodyFreeForFallback(r *Registry, body Value) bool {
	free := true
	WalkBodyWords([]Value{body}, func(w WordInfo, _ Value) {
		if !free {
			return
		}
		switch w.Name {
		case "true", "false", "none", "null":
			return
		case "break", "continue", "return":
			// A flow-control sentinel inside the body would set the shared
			// registry's FlowCtrl when the island's sub-engine runs it; the
			// VM cannot propagate that to an ENCLOSING compiled loop/frame
			// across the island boundary (the sentinel surfaces as a
			// tape-coupled island result and the VM rejects it). Refuse so
			// the whole program falls back, where the interpreter unwinds
			// it correctly. (Sentinels targeting a loop WITHIN the body are
			// rarer than this conservative rule loses; the gate stays
			// green either way.)
			free = false
			return
		case "args", "__pa":
			// Context-dependent words read the interpreter's per-call args
			// stack, which the VM's CALL_USER frames do not maintain (params
			// are frame locals). An island's sub-engine inside a compiled fn
			// would therefore see an EMPTY args stack where the interpreter
			// sees the enclosing call's list — `do [args]` in a fn islanded
			// to error(args_error) against the interpreter's [7]. Refuse so
			// the whole program falls back (the same divergence RecordCall's
			// context-dependent-word gate refuses for direct dispatches).
			free = false
			return
		}
		if r.Lookup(w.Name) != nil {
			return
		}
		if _, ok := TypeNames[w.Name]; ok {
			return
		}
		free = false
	})
	return free
}

// bodyRefsFnLocalFn reports whether any NoEvalArgs code-body arg of a
// body-consuming dispatch (a CompileFallbackBody word, or one declaring a
// CallableSpec) names a FN-LOCAL FN: a bare body Word whose current r.Defs
// binding both lives INSIDE the enclosing fn being compiled (the
// ComputeCaptures scope rule — Depth(name) > TopFnBaseline()[name]) and
// holds a Function value. Returns the first such name.
//
// The shape is the NUR037 leak: the interpreter resolves the body word per
// run through r.Defs, but a compiled program never executes the enclosing
// body's `def step fn […]` (the fn unit is static), so every record-time
// admission that bakes the NAME — the island span (bodyFreeForFallback
// resolves it via r.Lookup against the CHECK-time registry), the plain
// CALL_NATIVE const-bake (the body list is inert data to isInertConst),
// and the closure probe (whose body compile binds the check-time overload)
// — leaves the VM raising undefined_word on a program the interpreter
// runs. recordDispatchOutcome therefore refuses the whole program up front
// ("slow, not wrong" — design/COMPILABLE-SUBSET.md §5).
//
// Scope-matching is deliberately NARROW, mirroring ComputeCaptures:
//   - module-scope callbacks (TopFnBaseline nil, or Depth ≤ baseline) keep
//     compiling — the utils-corpus house-rule shape;
//   - fn-local VALUE defs (a non-Function binding, e.g. the mini-redis
//     KEYS accumulator) are legitimately handled by the closure path's
//     lexical captures and must NOT match;
//   - structured-lowering words (if / for — NoEvalArgs but neither
//     CompileFallbackBody nor Callable) record their bodies as inline
//     events (a fn-local dispatch lowers to CALL_USER by unit ref, no
//     name bake), so the family gate excludes them.
func bodyRefsFnLocalFn(r *Registry, sig *Signature, args []Value) (string, bool) {
	if sig == nil || len(sig.NoEvalArgs) == 0 ||
		(!sig.CompileEffect.Has(CompileFallbackBody) && sig.Callable == nil) {
		return "", false
	}
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return "", false // module scope: no enclosing fn, nothing is fn-local
	}
	for i := range args {
		if !sig.NoEvalArgs[i] {
			continue
		}
		name := ""
		WalkBodyWords([]Value{args[i]}, func(w WordInfo, _ Value) {
			if name != "" {
				return
			}
			v, bound := r.Defs.Top(w.Name)
			if !bound || r.Defs.Depth(w.Name) <= baseline[w.Name] {
				return
			}
			if _, isFn := v.Data.(FnDefInfo); isFn {
				name = w.Name
			}
		})
		if name != "" {
			return name, true
		}
	}
	return "", false
}

// bodyHasSentinel reports whether a code body contains a flow-control
// sentinel (break/continue/return). Such a body cannot compile to a closure
// (or island): the sentinel targets an ENCLOSING loop/frame the VM cannot
// reach across the call boundary, so the whole program must fall back and let
// the interpreter unwind it. Unlike bodyFreeForFallback this does NOT reject
// def references — the closure compile bakes a concrete def as a const or
// threads an enclosing-fn binding as a capture, and the probe compile refuses
// anything it cannot resolve.
func bodyHasSentinel(body Value) bool {
	return valueHasSentinel(body)
}

// valueHasSentinel reports whether a code-BODY value contains a live
// break/continue/return that would run when the body executes. It differs
// from WalkBodyWords (capture analysis) in one load-bearing way: it does NOT
// honour the .Quoted flag. A `quote`d list handed to `do`/`each`/… runs ALL
// its tokens as CODE — quote marks the LIST as data, not its tokens as
// non-executable — so a quoted `break` inside is a live sentinel (verified:
// `def b (quote [break]) for 5 [do b i]` breaks the loop in the interpreter).
// WalkBodyWords skipped it, letting the const-folded do-body compile past the
// sentinel gate; the escaped break then reached the loop epilogue as "flow
// signal with no enclosing loop" and was caught as a value — a miscompile.
// Nested fn bodies are still skipped: their sentinel targets their own scope.
func valueHasSentinel(v Value) bool {
	if _, ok := v.Data.(FnDefInfo); ok {
		return false
	}
	if IsWord(v) {
		w, _ := AsWord(v)
		return w.Name == "break" || w.Name == "continue" || w.Name == "return"
	}
	if v.Parent != nil && v.Parent.Equal(TList) && v.Data != nil {
		lst, _ := AsList(v)
		for i := 0; i < lst.Len(); i++ {
			if valueHasSentinel(lst.Get(i)) {
				return true
			}
		}
		return false
	}
	return scanBodyContainers(v, valueHasSentinel)
}

// scanBodyContainers applies scan to every code-position element of v's
// container families and reports the first hit: list elements, paren-expr
// tokens, interpolated-string ${...} expression parts, XML-interpolation
// attribute/child holes (recursing into nested child templates), and map
// values (keys are strings, inert). Every one of these runs as CODE when
// the container materialises in a body — verified: an interp-string
// `${break}` / a `{k: break}` map value in a quoted do-body breaks the
// loop in the interpreter. A non-container value returns false — leaf
// classification (words, fn payloads) belongs to the caller's scan.
func scanBodyContainers(v Value, scan func(Value) bool) bool {
	if v.Parent != nil && v.Parent.Equal(TList) && v.Data != nil {
		lst, _ := AsList(v)
		for i := 0; i < lst.Len(); i++ {
			if scan(lst.Get(i)) {
				return true
			}
		}
		return false
	}
	if IsParenExpr(v) {
		toks, _ := AsParenExpr(v)
		for _, t := range toks {
			if scan(t) {
				return true
			}
		}
		return false
	}
	if IsInterpString(v) {
		parts, _ := AsInterpString(v)
		for _, p := range parts {
			for _, t := range p.Expr {
				if scan(t) {
					return true
				}
			}
		}
		return false
	}
	if IsXmlInterp(v) {
		tmpl, _ := AsXmlInterp(v)
		return xmlTmplScan(tmpl, scan)
	}
	if v.Parent != nil && v.Parent.Equal(TMap) && v.Data != nil {
		m, _ := AsMap(v)
		if m == nil {
			return false
		}
		for _, key := range m.Keys() {
			mv, _ := m.Get(key)
			if scan(mv) {
				return true
			}
		}
	}
	return false
}

// xmlTmplScan applies scan to every ${...} expression token in an XML
// interpolation skeleton — attribute values and child holes, recursing
// into nested child templates (mirrors walkXmlTmplExprs).
func xmlTmplScan(t XmlTmpl, scan func(Value) bool) bool {
	for _, a := range t.Attr {
		for _, p := range a.Parts {
			for _, tok := range p.Expr {
				if scan(tok) {
					return true
				}
			}
		}
	}
	for _, c := range t.Cren {
		switch c.Kind {
		case XmlCrenExpr:
			for _, tok := range c.Expr {
				if scan(tok) {
					return true
				}
			}
		case XmlCrenChild:
			if c.Child != nil && xmlTmplScan(*c.Child, scan) {
				return true
			}
		}
	}
	return false
}

// bodyHasSentinelDeep is bodyHasSentinel plus a TRANSITIVE callee scan: a
// body word resolving to a user fn (registry-installed or a def-bound fn
// value) whose body — transitively through further calls — holds a bare
// break/continue leaks that signal through the call into the CALLER's loop
// (probe-verified: the interpreter unwinds `def f fn [.. [break 7]] .. do
// [f 1]` to the enclosing for), while a compiled closure boundary starts a
// fresh loop stack and surfaces "flow signal with no enclosing loop" error
// values — a miscompile. `return` is deliberately NOT counted in callees:
// the fn boundary consumes it. The scan is conservative — a break consumed
// by the callee's OWN loop still declines; the body then falls back with
// parity, so the cost is speed, never correctness.
//
// The closure-compile gates with a Registry in hand ride this deep variant:
// the token-list callable gate (which owns `do <body>` — the proven
// divergence) and the dyn-body bake gate. Where the interpreter does not
// thread a callee's break through the boundary anyway (each/filter callbacks
// raise `break outside loop` — probe-verified), the deep decline is still
// parity-safe: the fallback interpreter run raises identically. The
// const-bake gates (the noEvalBodiesInert twins) and the lambda gates keep
// the syntactic scan — no divergence reproduced there.
func bodyHasSentinelDeep(r *Registry, body Value) bool {
	if bodyHasSentinel(body) {
		return true
	}
	if r == nil {
		return false
	}
	return calleeValueLeaksFlow(r, body, map[string]bool{})
}

// calleeValueLeaksFlow is the transitive-scan walker: bare break/continue
// count as leaks, other words resolve to callee fn bodies (recursively,
// cycle-guarded by seen), and containers recurse. Constructed fn payloads
// scan too — an APPLIED fn value's break escapes to the caller (only raw
// tokens reach the direct scanner; a constructed FnDefInfo in a body is
// treated as potentially applied).
func calleeValueLeaksFlow(r *Registry, v Value, seen map[string]bool) bool {
	if fd, ok := v.Data.(FnDefInfo); ok {
		return fnDefLeaksFlow(r, &fd, seen)
	}
	if IsWord(v) {
		w, _ := AsWord(v)
		if w.Name == "break" || w.Name == "continue" {
			return true
		}
		return calleeLeaksFlow(r, w.Name, seen)
	}
	return scanBodyContainers(v, func(e Value) bool {
		return calleeValueLeaksFlow(r, e, seen)
	})
}

// calleeLeaksFlow resolves name to its aggregated dispatch table (Lookup
// unions every FnDefInfo binding on the name's def stack — exactly what a
// call would dispatch over) and scans the overload bodies. Native (Go-impl)
// sigs have no boru body and contribute nothing.
func calleeLeaksFlow(r *Registry, name string, seen map[string]bool) bool {
	if name == "" || seen[name] {
		return false
	}
	seen[name] = true
	fd := r.Lookup(name)
	return fd != nil && fnDefLeaksFlow(r, fd, seen)
}

// fnDefLeaksFlow scans every overload body of fd for a leaking
// break/continue. A module fn's body words resolve in its OWN registry.
func fnDefLeaksFlow(r *Registry, fd *FnDefInfo, seen map[string]bool) bool {
	if fd.Registry != nil {
		r = fd.Registry
	}
	for i := range fd.Signatures {
		for _, t := range fd.Signatures[i].Body() {
			if calleeValueLeaksFlow(r, t, seen) {
				return true
			}
		}
	}
	return false
}

// disjunctPartitionCap bounds the alternative cross product a
// partitioned dispatch will enumerate; beyond it the analysis falls
// back to the whole-disjunct match (wide but terminating).
const disjunctPartitionCap = 16

// disjunctPartitionReturns dispatches each alternative of strict
// disjunct args independently and joins the resulting return
// carriers — the abstract domain distributing over first-match
// dispatch. Returns ok=false when the partition does not apply and
// the caller should use the whole-disjunct path: no strict disjunct
// arg, an unknown or single-signature word, a concrete list/map arg
// (body-running ReturnsFns must not be re-entered per alternative),
// mismatched return arity across alternatives, or a cross product
// over the cap.
//
// Alternatives that reach NO signature get a partial_dispatch
// warning (that path would fail dispatch at runtime) and do not
// contribute to the join; if no alternative matches anything the
// partition declines and the engine's no_signature handling stands.
func disjunctPartitionReturns(r *Registry, word string, args []Value, pos SrcPos) ([]Value, bool) {
	if r == nil || !r.Check.IsActive() {
		return nil, false
	}
	hasStrictDisjunct := false
	declaredDomain := true // every strict disjunct arg is a declared-union param binding
	for _, a := range args {
		if IsDisjunct(a) && a.Carrier && !a.Dynamic {
			hasStrictDisjunct = true
			if di, err := AsDisjunct(a); err != nil || !di.Declared {
				declaredDomain = false
			}
		}
		// Body-running ReturnsFns (if, each, fold, do, …) take
		// concrete list/map operands; re-running them per alternative
		// would duplicate branch analysis and its diagnostics.
		if IsConcrete(a) && (a.Parent.ConformsTo(TList) || a.Parent.ConformsTo(TMap)) {
			return nil, false
		}
	}
	if !hasStrictDisjunct {
		return nil, false
	}
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) == 0 {
		return nil, false
	}

	combos, ok := disjunctCombos(args, disjunctPartitionCap)
	if !ok {
		return nil, false
	}

	// Per-combination transfer function: dispatch the concrete
	// alternative tuple and gather its return-carrier row. A combo that
	// reaches no overload is dropped with a partial_dispatch warning; a
	// combo whose overload is unannotated declines the whole partition
	// (the whole-disjunct fallback — missing_returns + dynamic Any —
	// beats a partial join). Surviving rows are joined position-wise.
	//
	// The combo runs are TYPE PROBES over per-alternative carrier COPIES
	// (alternativeCarriers mints fresh IDs), so under an ARMED recording
	// they must not record: a user fn's ReturnsFn would RecordUserCall the
	// fresh-ID copies ("fn call operand of unknown provenance" — the
	// L-JOIN recursive-union refusal) and compile one unit per combo.
	// Suspend for the loop (a no-op on a plain check); the caller records
	// the dispatch ONCE with the original args (carrierResults' partition
	// arm). Diagnostics (partial_dispatch) are not gated by suspension.
	rows := make([][]Value, 0, len(combos))
	resume := r.Check.Recorder().Suspend()
	defer resume()
	for _, combo := range combos {
		comboSig := firstMatchingSig(fn, combo)
		if comboSig == nil {
			// Severity by disjunct PROVENANCE: when every disjunct arg is a
			// DECLARED union param (DisjunctInfo.Declared), the failing
			// alternative is a valid input of the fn's own annotation — the
			// body is broken for part of its declared domain, an error. An
			// analysis-join disjunct (branch join, element join) stays a
			// warning: the runtime materialises one alternative, so the
			// failing path may be dead (the fuzz corpus' dead-branch class).
			d := CheckDiagnostic{
				Code: "partial_dispatch",
				Detail: word + " has no overload for alternative (" +
					comboTypeNames(combo) + ") of a disjunct input — that path would fail dispatch at runtime",
				Word: word,
				Row:  pos.Row,
				Col:  pos.Col,
			}
			if declaredDomain {
				d.Severity = SeverityError
				d.Detail = word + " has no overload for alternative (" +
					comboTypeNames(combo) + ") of a declared union parameter — a valid argument of the declared type would fail dispatch at runtime"
			}
			r.Check.AddDiagnostic(d)
			continue
		}
		switch {
		case comboSig.ReturnsFn != nil:
			raw := comboSig.ReturnsFn(resolveTypeNameArgs(combo), r)
			rets := make([]Value, len(raw))
			for i, v := range raw {
				rets[i] = toCarrier(v)
			}
			rows = append(rows, rets)
		case comboSig.Returns != nil:
			rets := make([]Value, len(comboSig.Returns))
			for i, t := range comboSig.Returns {
				rets[i] = NewCarrier(t)
			}
			rows = append(rows, rets)
		default:
			return nil, false
		}
	}
	return joinReturnRows(rows)
}

// disjunctCombosTakeSig reports whether EVERY per-alternative combination of
// the disjunct args first-matches exactly the caller's committed sig — the
// condition under which ONE recorded CALL_USER of that sig's unit is faithful
// for every runtime alternative (carrierResults' partitioned user-fn arm). A
// combo that would first-match a SIBLING overload (a narrow arm ahead of the
// committed wide one) makes the single baked call a miscompile — the
// interpreter dispatches the sibling for that alternative — so the caller
// keeps the refusal. Combo enumeration failure (over the cap) declines.
func disjunctCombosTakeSig(r *Registry, word string, args []Value, sig *Signature) bool {
	fn := r.Lookup(word)
	if fn == nil {
		return false
	}
	combos, ok := disjunctCombos(args, disjunctPartitionCap)
	if !ok {
		return false
	}
	for _, combo := range combos {
		cs := firstMatchingSig(fn, combo)
		// Lookup mints a FRESH aggregate per call in check mode (the
		// pointer-identity contract), so compare by the stable
		// run-implementation identity — Signature.Impl, the same primitive
		// the user-poly drift guard keys on.
		if cs == nil || cs.Impl != sig.Impl {
			return false
		}
	}
	return true
}

// alternativeCarriers expands a strict disjunct carrier into one carrier
// per flattened alternative; any other value yields itself. It is the
// per-argument expansion the disjunct cross product enumerates.
func alternativeCarriers(a Value) []Value {
	if IsDisjunct(a) && a.Carrier && !a.Dynamic {
		lits := flattenAlternatives(a)
		out := make([]Value, 0, len(lits))
		for _, lit := range lits {
			out = append(out, CarrierOfLiteral(lit))
		}
		return out
	}
	return []Value{a}
}

// disjunctCombos enumerates the bounded cross product of the per-argument
// alternative expansions (alternativeCarriers) — the powerset domain a
// union-typed argument list distributes over. Returns ok=false when the
// product would exceed limit, so the caller widens to the whole-disjunct
// path (wide but terminating).
func disjunctCombos(args []Value, limit int) ([][]Value, bool) {
	combos := [][]Value{nil}
	for i, a := range args {
		alts := alternativeCarriers(a)
		if len(combos)*len(alts) > limit {
			return nil, false
		}
		next := make([][]Value, 0, len(combos)*len(alts))
		for _, c := range combos {
			for _, alt := range alts {
				row := make([]Value, i+1)
				copy(row, c)
				row[i] = alt
				next = append(next, row)
			}
		}
		combos = next
	}
	return combos, true
}

// joinReturnRows position-wise JoinCarriers-folds the return-carrier rows
// gathered from each matched alternative — the abstract join at the
// dispatch merge. ok=false when no row survived or the rows disagree on
// return arity (the partition declines; the caller widens). A row set
// whose members are all zero-arity yields an empty slice with ok=true.
func joinReturnRows(rows [][]Value) ([]Value, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	joined := rows[0]
	for _, rets := range rows[1:] {
		if len(rets) != len(joined) {
			return nil, false
		}
		for i := range joined {
			joined[i] = JoinCarriers(joined[i], rets[i])
		}
	}
	return joined, true
}

// firstMatchingSig returns the first signature of fn (registration
// keeps Signatures in SortSignatures match order) whose arity equals
// len(args) and whose every positional type admits the corresponding
// arg, or nil. Mirrors matchSignature's per-arg type test for the
// already-collected case.
func firstMatchingSig(fn *FnDefInfo, args []Value) *Signature {
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) {
			continue
		}
		ok := true
		for j := range args {
			if !SigTypeMatches(args[j], SigArgType(s, j)) {
				ok = false
				break
			}
		}
		if ok {
			return s
		}
	}
	return nil
}

// comboTypeNames renders a combo's types for diagnostics: "Integer, String".
func comboTypeNames(combo []Value) string {
	parts := make([]string, len(combo))
	for i, v := range combo {
		parts[i] = v.Parent.Leaf()
	}
	return strings.Join(parts, ", ")
}

// dynamicReachableReturns returns the distinct single-position return
// types of every signature of `word` that the (dynamic) args reach, but
// only when there are TWO OR MORE distinct returns — the case the
// single matched-sig return would get wrong. Returns nil otherwise (the
// common case: contagion's matched-sig return is already correct).
// Restricted to same-arity, single-static-return sigs; any other shape
// falls back to contagion.
// dynamicReachableValueReturns returns the distinct 1-value return types of the
// word's overloads that the (possibly dynamic) args can reach. Unlike
// dynamicReachableReturns it does NOT bail on a sibling that returns 0 values —
// it is for the MIXED-arity case: a mutator like `set` whose in-place overloads
// (Array/Object/Store/Class) return nothing while the value-returning twins
// (Map/List/Flex) return the updated node. A CONCRETE receiver reaches only its
// own overload (sigTypeMatches is exact for it), so this returns nil there and
// the true 0-arity stands; only a genuinely dynamic receiver reaches both.
// dynamicReachableOverloadCount counts how many of word's same-arity overloads
// the (partly-dynamic) arg list could match. ≥2 means the dispatch is AMBIGUOUS:
// a gradual-Any arg matches every overload optimistically, so the checker's single
// committed overload may not be the one the RUNTIME value needs. Used to refuse a
// higher-order word (each/fold/scan) over a gradual collection — its List-vs-Map
// overloads can't be statically chosen and it has no poly re-match (code body).
// fnPredicateOverloadHazard reports whether word's same-arity overload set
// both (a) contains an fn-PREDICATE-typed param slot (*predicateUnifier —
// the boru fn-body membership path whose check-mode match is LENIENT:
// RunPredicate short-circuits true, registry.go) and (b) leaves more than
// one arm reachable for these args — the combination where a static arm
// commit can diverge from the interpreter's runtime predicate fall-through.
// DepScalar and Go-member types match self-contained in check mode (no
// leniency), so they carry no hazard.
func fnPredicateOverloadHazard(r *Registry, word string, args []Value) bool {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return false
	}
	hasPred, reachable := false, 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) {
			continue
		}
		reach := true
		for j := range args {
			t := SigArgType(s, j)
			if t != nil {
				if _, ok := t.Behavior().(*PredicateUnifier); ok {
					hasPred = true
				}
			}
			if !SigTypeMatches(args[j], t) {
				reach = false
				break
			}
		}
		if reach {
			reachable++
		}
	}
	return hasPred && reachable >= 2
}

func dynamicReachableOverloadCount(r *Registry, word string, args []Value) int {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return 0
	}
	n := 0
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) {
			continue
		}
		reach := true
		for j := range args {
			// A strict OR gradual Any carrier could hold a value of any type at
			// run time, so it reaches EVERY same-arity arm — the dispatch is
			// genuinely runtime-dynamic and must poly re-match, not commit
			// statically to the Any-slot arm. Without this a strict Any (an Any
			// param's generalised arg — core_helpers.go) reached only the Any
			// overload, so a wrapper forwarding a Map through an Any param baked
			// the Any arm and diverged from the interpreter's Map dispatch (the
			// each/fold-body multi-sig degradation).
			if isAnyCarrier(args[j]) {
				continue
			}
			// A GENERIC placeholder slot ((T extends Comparable)) admits by
			// per-call instantiation over the runtime value, which a gradual
			// arg's bound-disjointness probe cannot see (tand(Integer, T-node)
			// is Never even though a runtime Integer instantiates T) — count
			// the arm reachable so the ambiguous dispatch stays a runtime
			// re-matched user poly instead of a wrong static commit.
			if args[j].Dynamic && IsTypeParamNode(SigArgType(s, j)) {
				continue
			}
			if !SigTypeMatches(args[j], SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if reach {
			n++
		}
	}
	return n
}

func dynamicReachableValueReturns(r *Registry, word string, args []Value) []*Type {
	fn := r.Lookup(word)
	if fn == nil {
		return nil
	}
	var rets []*Type
	seen := map[string]bool{}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if s.TotalArgs() != len(args) || len(s.Returns) != 1 || s.Returns[0] == nil {
			continue
		}
		reach := true
		for j := range args {
			if !SigTypeMatches(args[j], SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if !reach {
			continue
		}
		if t := s.Returns[0]; !seen[t.ID] {
			seen[t.ID] = true
			rets = append(rets, t)
		}
	}
	return rets
}

func dynamicReachableReturns(r *Registry, word string, args []Value) []*Type {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return nil
	}
	// All operands fully unknown (dynamic carriers bound to the Any root): the
	// result is genuinely statically-unknown, so an unknown-return (ReturnsFn)
	// reachable overload may safely fold in as the Any alternative below
	// (widening the union to gradual-permissive). When SOME operand is
	// concrete, the reachable concrete returns are a tighter, trustworthy
	// union and an unknown-return overload must still suppress refinement — a
	// partially-known call should not be blanket-widened to Any (that
	// mis-narrowed trie's `child` through a `get` whose module-namespace overload
	// then looked reachable).
	allUnknown := len(args) > 0
	for _, a := range args {
		if !a.Dynamic || a.Parent == nil || !a.Parent.Equal(TAny) {
			allUnknown = false
			break
		}
	}
	var rets []*Type
	seen := map[string]bool{}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		// A different-arity overload is not a candidate for THIS call's arg
		// count — skip it rather than bail. The original code bailed here,
		// which disabled refinement for every word with mixed-arity overloads:
		// `slice`'s 1/2/3-arg forms meant a dynamic-receiver slice committed to
		// the single matched overload (`dynamic(List)` for a String-or-List
		// receiver) instead of the `String|List` disjunct, so a String consumer
		// of the result then failed no_signature (radix's edge splitter, where
		// the sliced label is a get-result of unknown type).
		if s.TotalArgs() != len(args) {
			continue
		}
		reach := true
		for j := range args {
			if !SigTypeMatches(args[j], SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if !reach {
			continue
		}
		// A REACHABLE overload we cannot reduce to a single return type
		// (ReturnsFn / multi- or zero-return): its result is statically
		// unknown, so it contributes the Any alternative to the union rather
		// than disabling refinement. The old code bailed (`return nil`) here,
		// which left carrierResults committed to the single matched overload's
		// return — for a word whose overloads return DISJOINT types over
		// dynamic operands (`add` of two dynamic-Any operands matches the
		// temporal `Instant`/`Date` overloads alongside the numeric ReturnsFn
		// and the String concat), that commitment picked `Instant`, and a
		// downstream String consumer then failed no_signature (radix-delete's
		// `(lab (gpair get 0) add)` merged label fed to `set-edge`'s
		// `newlabel:String`). Folding the unknown in as Any makes the union
		// gradual-permissive, matching any later typed use — sound for dynamic
		// inputs, where the modality is optimistic by construction.
		if len(s.Returns) != 1 || s.Returns[0] == nil {
			if !allUnknown {
				return nil
			}
			if !seen[TAny.ID] {
				seen[TAny.ID] = true
				rets = append(rets, TAny)
			}
			continue
		}
		if t := s.Returns[0]; !seen[t.ID] {
			seen[t.ID] = true
			rets = append(rets, t)
		}
	}
	if len(rets) < 2 {
		return nil
	}
	return rets
}

// narrowDynamicUses implements narrowing-through-use
// (design/dynamic-modality-report.10.md): when a dynamic carrier resolved
// from a binding is consumed by a typed slot, the binding tightens to
// dynamic(bound ∩ slot) for downstream uses, so a later provably-disjoint
// use of the same name fails the match rule and is flagged — no explicit
// guard needed. Scoped via the def stack: branch analysis
// (RunCarrierBodyWithDefs) truncates these pushes, so a then-branch
// narrowing never leaks to the else-branch. Sound — the bound only
// tightens, never widens.
func narrowDynamicUses(r *Registry, word string, sig *Signature, args []Value) {
	if r == nil || sig == nil || !r.Check.IsActive() {
		return
	}
	for i, a := range args {
		if !a.Dynamic || a.DynFrom() == "" {
			continue
		}
		// Only narrow a binding that is itself still dynamic (consistent
		// with this value) — guards against a since-rebound name.
		cur, ok := r.Defs.Top(a.DynFrom())
		if !ok || !cur.Dynamic {
			continue
		}
		slot := SigArgType(sig, i)
		if slot == nil {
			continue
		}
		// Skip a POLYMORPHIC position: when another of the word's overloads
		// is equally reachable for THIS call but constrains position i to a
		// different (disjoint) type, the dynamic value could legitimately
		// dispatch to either overload at run time, so narrowing to the single
		// matched slot is unsound — it would wrongly reject a downstream use of
		// the sibling shape. `slice`'s data arg is String in one 3-arg overload
		// and List in the other; a dynamic-Any label narrowed to List by the
		// first slice then failed every String consumer (radix's edge splitter,
		// where the sliced label is a get-result of unknown type). Leaving the
		// carrier un-narrowed only loosens matching, never tightens — sound.
		if slotIsPolymorphic(r, word, args, i, slot) {
			continue
		}
		bound := cur
		bound.Dynamic = false
		bound.SetDynFrom("")
		narrowed := TandValues(bound, NewCarrier(slot))
		// A successful match guarantees a non-disjoint intersection; skip
		// when the bound did not actually tighten (no-op / avoids
		// unbounded layer growth on repeated same-type uses).
		if IsNeverShape(narrowed) || ValuesEqual(bound, narrowed) {
			continue
		}
		// A narrowed intersection that collapses to a bare ROOT node (nil
		// Parent — None / Any / Never) must not be re-promoted to a dynamic
		// carrier: the resulting carrier would carry a nil bound that
		// matchSignature reads as "matches nothing", cascading false
		// no_signature through every downstream use of the name. None in
		// particular is the builder-pattern sentinel (a `none` accumulator that
		// becomes a node — tst/radix's `none entries […tst-insert] fold`);
		// tightening the binding to None breaks the node-rebuild words (get /
		// with-lo / mk-tnode) that follow. Leaving the binding at its prior,
		// broader dynamic bound only loosens matching — sound, never a new
		// false positive.
		if narrowed.Parent == nil {
			continue
		}
		// Identity-preserving rebind: the narrowing refines the STATIC bound
		// of the SAME runtime value — no new value exists at run time.
		// TandValues assembles the intersection as a fresh Value (fresh ID),
		// which would orphan the binding from its producing event: a
		// `def nodes (tree get "nodes")` narrowed to List at its first typed
		// use lost the get event's provenance, so every LATER read of the
		// name refused ("fn call operand of unknown provenance" — the
		// decision eval-tree walkers). Re-stamp the current binding's ID so
		// the recorder's producedBy still resolves the rebound name to its
		// original producer.
		narrowed.ID = cur.ID
		r.Defs.Push(a.DynFrom(), NewDynamicCarrierValue(narrowed))
	}
}

// slotIsPolymorphic reports whether the word has a second, equally-reachable
// overload that constrains argument position i to a type other than
// matchedSlot. "Reachable" means same arity, every OTHER position matches the
// actual carrier args, and the dynamic value's bound is not provably disjoint
// from that overload's type at i. When true, position i is polymorphic for
// this call (e.g. slice's data arg: String vs List), so narrowing the dynamic
// carrier to matchedSlot alone would be unsound.
func slotIsPolymorphic(r *Registry, word string, args []Value, i int, matchedSlot *Type) bool {
	if word == "" {
		return false
	}
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return false
	}
	for k := range fn.Signatures {
		s := &fn.Signatures[k]
		if s.Fallback || s.TotalArgs() != len(args) || i >= s.TotalArgs() {
			continue
		}
		st := SigArgType(s, i)
		if st == nil || st.Equal(matchedSlot) {
			continue
		}
		reach := true
		for j := range args {
			if j == i {
				// The dynamic value at i: reachable unless provably disjoint
				// from this overload's type there.
				if IsNeverShape(TandValues(NewCarrier(args[j].Parent), NewCarrier(st))) {
					reach = false
					break
				}
				continue
			}
			if !SigTypeMatches(args[j], SigArgType(s, j)) {
				reach = false
				break
			}
		}
		if reach {
			return true
		}
	}
	return false
}

// anyDynamicCarrier reports whether any value is a dynamic carrier — the
// trigger for gradual contagion in carrierResults.
func anyDynamicCarrier(vs []Value) bool {
	for _, v := range vs {
		if v.Dynamic {
			return true
		}
	}
	return false
}

// anyNonConcreteOperand reports whether any value is not a concrete
// payload-bearing value (a typed carrier or a bare type literal) — the
// operand shape under which a static CoreDefault match is not a dispatch
// proof (recordCallRefusal): the runtime tag may be a strict subtype a
// more-specific unlocked overload claims.
func anyNonConcreteOperand(vs []Value) bool {
	for _, v := range vs {
		if !IsConcrete(v) {
			return true
		}
	}
	return false
}

// anyAnyCarrier reports whether any value is an Any-typed carrier — a value
// whose static type is unknown (a List/Map element, an opaque module-fn
// result). Unlike anyDynamicCarrier it does not require the Dynamic flag: a
// strict Any carrier conforms to no concrete container/operand type, so it
// fails matchSignature and reaches the no-signature recovery, where it is the
// signal that the dispatch is genuinely runtime-dynamic (poly), not a concrete
// type error.
func anyAnyCarrier(vs []Value) bool {
	for _, v := range vs {
		if isAnyCarrier(v) {
			return true
		}
	}
	return false
}

// isAnyCarrier reports whether v is an Any-typed carrier (strict or gradual):
// a value whose static type is exactly Any, so at run time it may hold a value
// of any type. Such an arg to a multi-overload user fn makes the dispatch
// genuinely runtime-dynamic — the committed Any-slot arm is not a proof, since
// the runtime value may match a more-specific sibling overload.
func isAnyCarrier(v Value) bool {
	return v.Carrier && v.Parent != nil && v.Parent.Equal(TAny)
}

// anyDisjunctCarrier reports whether any value is a UNION (Disjunct) carrier — a
// receiver whose static type is a union of alternatives (`Map | Any` from an
// if-branch join, the trie/tst node-vs-none shape). Like an Any carrier it
// conforms to no single concrete overload, so matchSignature cannot commit and
// the dispatch reaches the no-signature recovery — but at run time it holds ONE
// concrete alternative, and the SAME first-match the interpreter takes
// dispatches it. So a poly-safe word (get/getr) over a union receiver records a
// runtime-re-matching OpCallNativePoly instead of refusing; a runtime member
// that matches no overload routes to the sound OpFallback island. (A bare
// disjunct carrier carries no DisjunctInfo payload, so IsDisjunct is false here —
// the carrier's Parent IS the union type, which is what we test.)
func anyDisjunctCarrier(vs []Value) bool {
	for _, v := range vs {
		if v.Carrier && v.Parent != nil && v.Parent.ConformsTo(TDisjunct) {
			return true
		}
	}
	return false
}

// anyImpreciseCarrier reports whether any value is a CARRIER of a concrete-but-
// imprecise type — a checker stand-in whose static type is a definite tag
// (Integer, String, List, …) that a multi-branch narrowing / carrier-join
// settled on, but which at run time may hold a value of a DIFFERENT type the
// static analysis could not pin (the deep guard-complement chain in the
// mini-redis codec's `join " " reply` — reply lands as a scalar carrier where
// `join`'s List slot cannot commit). Like an Any / Disjunct carrier it is NOT a
// concrete value (Carrier=true), so per the no-signature-recovery contract
// ("Concrete operands are a genuine type error and refuse; carriers recover")
// it is eligible for the runtime-re-matching OpCallNativePoly: tryRecordPoly's
// own poly-safety gates decide whether recovery is faithful, and a runtime
// value that matches no overload routes to the sound OpFallback island / raises
// the interpreter's byte-identical error. Bare type nodes (a type-literal
// operand of is/typeof) are excluded — they are not runtime values to re-match.
func anyImpreciseCarrier(vs []Value) bool {
	for _, v := range vs {
		if v.Carrier && v.Parent != nil && !IsBareTypeNode(v) {
			return true
		}
	}
	return false
}

// ReturnsIdentity is a ReturnsFunc helper that returns its inputs
// unchanged (as carriers). Use for stack operations that preserve
// their inputs — dup, swap, over, rot, etc. — where the output types
// are directly expressible in terms of the input types.
//
// The mapping is a permutation-description slice: result[i] = args[mapping[i]].
// Example: swap is ReturnsIdentity(1, 0); over is ReturnsIdentity(0, 1, 0).
//
// A DUPLICATED source index (dup `(0, 0)`, over `(0, 1, 0)`) would otherwise
// return the same Value — one Value.ID — for several stack outputs, which
// the bytecode emitter's per-value provenance (emit.go producedBy) cannot
// tell apart: a `dup`-bodied higher-order word (`each [dup add]`) records
// both of add's operands onto the LAST output, so the operand layout refuses
// them as "not adjacent." Each output of a repeated source gets a fresh
// identity (the carrier-identity DUP path) so the N copies stay distinct;
// the source's own provenance is left untouched (no output keeps its ID).
// Identity-only — runtime dispatch is unaffected (ReturnsFn is check-mode).
func ReturnsIdentity(mapping ...int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		counts := make(map[int]int, len(mapping))
		for _, m := range mapping {
			counts[m]++
		}
		out := make([]Value, len(mapping))
		for i, m := range mapping {
			if m < 0 || m >= len(args) {
				out[i] = NewCarrier(TAny)
				continue
			}
			v := args[m] // struct copy: the ID write below is local to v.
			if counts[m] > 1 {
				v.ID = GenerateID(IDPrefixForType(v.Parent))
			}
			out[i] = v
		}
		return out
	}
}

// valueTreeHasCarriers reports whether v or any nested map value / list
// element is a carrier or dynamic — i.e. statically unknown. Used to scope
// the static generic-record construction validation to fully-literal data;
// carrier-bearing data defers to the runtime constructor.
func valueTreeHasCarriers(v Value) bool {
	if v.Carrier || v.Dynamic {
		return true
	}
	if m, err := AsMap(v); err == nil && m.Len() > 0 {
		for _, k := range m.Keys() {
			if mv, ok := m.Get(k); ok && valueTreeHasCarriers(mv) {
				return true
			}
		}
	}
	if l, err := AsList(v); err == nil && l.Len() > 0 {
		for _, e := range l.Slice() {
			if valueTreeHasCarriers(e) {
				return true
			}
		}
	}
	return false
}

// ReturnsFreshInstance is a ReturnsFunc for IMPURE constructors (make):
// each result is a fresh VALUE carrier of the TYPE named by args[mapping[i]],
// mirroring the runtime where every construction mints a new value
// (NewValueRaw). ReturnsIdentity returns args[m] verbatim — for `make` that
// is the requested TYPE LITERAL, which is wrong twice over:
//
//   - Identity. A type literal is dual (eng CLAUDE.md "Canonical *Type
//     Pointers"): its Value.ID IS the type's lattice identity, so every
//     `make P {}` returns the SAME P-literal value (one Value.ID). The
//     bytecode lowerer's per-value provenance (emit.go's producedBy) then
//     cannot tell two instances apart — it resolves both operands of a
//     downstream `(make P {}) (make P {}) eq` onto the LAST make, leaving
//     one simulated-stack slot where the binary op needs two ("stack
//     discipline underflow"). A fresh carrier ID keeps each construction a
//     distinct operand.
//   - Faithfulness. The runtime result is a VALUE of type P, not the type
//     literal P; a carrier of P is the same shape carrierResults gives every
//     other declared return, and conformance sees the instance's type.
//
// (Minting a fresh ID on the type literal itself is NOT viable: it severs
// the literal↔type ID duality, so ValueType can no longer resolve the made
// type and conformance checks fail.)
//
// Only a concrete bare-type-node target is converted; a dynamic/computed
// target (a carrier already) keeps the prior identity behaviour.
func ReturnsFreshInstance(mapping ...int) ReturnsFunc {
	return func(args []Value, r *Registry) []Value {
		out := make([]Value, len(mapping))
		for i, m := range mapping {
			switch {
			case m < 0 || m >= len(args):
				out[i] = NewCarrier(TAny)
			case !args[m].Carrier && !args[m].Dynamic:
				// A concrete constructor target — a bare type node
				// (`make Pathon …`, `make Foo …`) or a structural type body
				// (`make P {}` for a class/record, whose literal carries an
				// ClassTypeInfo/RecordTypeInfo payload). ValueType yields the
				// made *Type in both cases (the node itself, or the body's
				// Parent); a fresh carrier of it is the per-call instance.
				//
				// EXCEPTION — a GENERIC RECORD SCHEMA target (`make Rule {…}`
				// where Rule is `gen [R] refine Record […]`): the schema node
				// is minted in the Ideal branch (parent Ideal/Record), but the
				// runtime instance is built by MakeRecordR and carries the
				// TMap tag — a schema-node carrier fails the Map conformance
				// the runtime accepts (the voxgig decision `make-rule … [Map]`
				// return-check false positive). Type the carrier faithfully to
				// the runtime tag: instantiate statically when the data is
				// concrete (the instantiated record body's ValueType sits in
				// the Map branch and keeps the field schema reachable), else
				// fall back to a plain TMap carrier.
				if info, serr := AsTypeSchema(args[m]); serr == nil && info.Kind == SchemaRecord {
					if r != nil && len(args) == 2 && IsConcrete(args[1-m]) && !valueTreeHasCarriers(args[1-m]) {
						// CONCRETE construction data: validate statically, exactly as the
						// runtime will — infer + instantiate, then run the record
						// constructor. On SUCCESS the carrier takes the instantiated
						// body's Map-branch type. On FAILURE (uninferable params —
						// unbound_param — or a construction error like an unknown field)
						// the runtime RAISES, so fall through to the pre-exception
						// schema-node carrier below: its failed Map conformance is what
						// FLAGS the enclosing [Map] return (Codex PR #218 review — a
						// silent TMap carrier here read clean where the runtime raises;
						// pinned by lang/spec/generics.tsv:81 and the stage0 pins).
						validated := false
						if inst, ierr := InferAndInstantiateSchema(r, args[m], args[1-m]); ierr == nil {
							if rt, rerr := AsRecordType(inst); rerr == nil {
								if _, mkErr := MakeRecordR(rt, args[1-m], false, r); mkErr == nil {
									c := NewCarrier(CanonicalType(r, ValueType(inst)))
									// Ride the instantiated FIELD SCHEMA on the
									// instance carrier (the recordSchemaCarrier
									// shape: dynamic + RecordTypeInfo payload) so a
									// downstream field read narrows via the
									// accessor ReturnsFns instead of ending
									// dynamic(Any) — `(make Test.TestCase {…}) get
									// "out"`. Gradual, guard-discharged.
									if rt.Fields != nil {
										c.Data = rt
										c.Dynamic = true
									}
									out[i] = c
									validated = true
								}
							}
						}
						if validated {
							continue
						}
						// fall through to the schema-node carrier (flags the return)
					} else {
						// Non-concrete data, or a concrete map holding CARRIER members
						// (`{kind:(table.kind) …}` — field values statically unknown):
						// whether construction succeeds is unknowable, so the optimistic
						// Map carrier is the gradual posture — matching the runtime tag
						// on success. The pre-fix schema-node carrier flagged EVERY such
						// make (the voxgig decision make-rule / with-policy FPs).
						c := NewCarrier(TMap)
						// A PARAMETERLESS record schema's field TYPES are static
						// regardless of the construction data — ride them so a
						// module constructor's `make TestCase {name:name …}`
						// (carrier-membered data) still narrows downstream field
						// reads. Generic schemas with unresolved params keep the
						// plain Map carrier.
						if len(info.Params) == 0 {
							if rt, rerr := AsRecordType(info.Body); rerr == nil && rt.Fields != nil {
								c.Data = rt
								c.Dynamic = true
							}
						}
						out[i] = c
						continue
					}
				}
				t := ValueType(args[m])
				if r != nil {
					t = CanonicalType(r, t)
				}
				c := NewCarrier(t)
				// A plain RECORD BODY target (`def TC refine Record […]` —
				// the binding is the body value, Parent TMap, RecordTypeInfo
				// payload): ride the declared field schema on the instance
				// carrier so downstream field reads narrow (same shape and
				// rationale as the schema-record branches above).
				if rt, ok := args[m].Data.(RecordTypeInfo); ok && rt.Fields != nil {
					c.Data = rt
					c.Dynamic = true
				}
				out[i] = c
			default:
				// Dynamic / computed target (a carrier already): keep the
				// prior identity behaviour.
				out[i] = args[m]
			}
		}
		return out
	}
}

// RecordTypedDefMake records the synthetic `make` event a typed-def
// object-instance construction (`def b:Type {map}`) skips. That form is
// exactly `def b (make Type map)`, but the typed-def handler builds the
// instance by calling MakeObject directly, bypassing the make WORD dispatch —
// so the instance never gets the make event that gives an explicit make its
// provenance, and a downstream `b typeof` then refuses with an operand the
// lowerer cannot resolve.
//
// In active emit mode this records make over [typeArg, body] (both inert
// consts — the instantiated type body and the scalar map) and returns the
// fresh instance carrier to bind in place of the concrete result, so the
// binding carries make-equivalent provenance; the VM re-runs make's
// MakeObjHandler at run time, producing the identical instance. Outside emit
// mode it returns (Value{}, false) and the caller binds the concrete value.
func RecordTypedDefMake(r *Registry, typeArg, body Value, pos SrcPos) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	es := r.Check.Recorder()
	if !es.Active() {
		return Value{}, false
	}
	sig := objectMakeSig(r)
	if sig == nil {
		return Value{}, false
	}
	t := CanonicalType(r, ValueType(typeArg))
	carrier := toCarrier(NewCarrier(t))
	es.RecordCall("make", sig, []Value{typeArg, body}, []Value{carrier}, pos, false, false)
	return carrier, true
}

// objectMakeSig returns make's `[Ideal Map]` overload (MakeObjHandler) — the
// one a typed-def object construction would have dispatched. Looked up by
// arg shape so it tracks the registered native rather than a fabricated sig
// (the VM calls Sig.Handler directly at OpCallNative).
func objectMakeSig(r *Registry) *Signature {
	fd := r.Lookup("make")
	if fd == nil {
		return nil
	}
	for i := range fd.Signatures {
		s := &fd.Signatures[i]
		if s.TotalArgs() == 2 && SigArgType(s, 0) != nil && SigArgType(s, 1) != nil &&
			SigArgType(s, 0).Equal(TIdeal) && SigArgType(s, 1).Equal(TMap) {
			return s
		}
	}
	return nil
}

// ReturnsStatic builds a ReturnsFunc that always produces a fixed list
// of carrier types, independent of args. Equivalent to setting Returns
// directly; provided so ReturnsFn call sites can be uniform.
func ReturnsStatic(types ...*Type) ReturnsFunc {
	return func(_ []Value, _ *Registry) []Value {
		out := make([]Value, len(types))
		for i, t := range types {
			out[i] = NewCarrier(t)
		}
		return out
	}
}

// ReturnsNumericBinary models the arithmetic-tower result type for
// add/sub/mul/div/mod/pow on [TNumber, TNumber]: the widest leaf wins
// among the exact types (Integer < BigInteger < BigDecimal); an
// Integer⊕Float mix is Float. A Big⊕Float mix errors at runtime (the
// exact types never silently become Float); statically it is modelled as
// the Big leaf so analysis can continue past it.
func ReturnsNumericBinary() ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		if len(args) != 2 {
			return []Value{NewCarrier(TFloat)}
		}
		a, b := args[0].Parent, args[1].Parent
		switch {
		case a.ConformsTo(TBigDecimal) || b.ConformsTo(TBigDecimal):
			return []Value{NewCarrier(TBigDecimal)}
		case a.ConformsTo(TBigInteger) || b.ConformsTo(TBigInteger):
			return []Value{NewCarrier(TBigInteger)}
		case a.ConformsTo(TFloat) || b.ConformsTo(TFloat):
			return []Value{NewCarrier(TFloat)}
		default:
			return []Value{NewCarrier(TInteger)}
		}
	}
}

// ReturnsAddConcat types the result of add's string-concat overloads
// ([TString TScalar] / [TScalar TString]). Those overloads commit whenever an
// operand fills the String slot — but a GRADUAL (dynamic) operand fills it
// OPTIMISTICALLY: it MIGHT be a String, yet at run time it could be a Number,
// in which case the interpreter takes the NUMERIC overload instead. A definite
// [String] return for that case wrongly rejects a downstream numeric use — the
// sort.boru `msd-go` false positive, where `lo add (Array-get-result)` (Integer
// + a dynamic get) was typed String and then failed the recursive `lo:Integer`
// param. So: return String only when an operand is PROVABLY String (a concrete
// or non-gradual String-typed value); otherwise the result is String-or-Number,
// i.e. a gradual Scalar, which matches a later numeric OR string use and a guard
// discharges back to strict. Runtime concat (addConcatHandler) is unchanged —
// this governs check-mode result typing only.
func ReturnsAddConcat() ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
		for _, a := range args {
			if !a.Dynamic && a.Parent != nil && a.Parent.ConformsTo(TString) {
				return []Value{NewCarrier(TString)}
			}
		}
		return []Value{NewDynamicCarrier(TScalar)}
	}
}

// CommonAncestorType returns the longest common prefix of two type
// paths, as a new Type. For example, given Number/Integer/42 and
// Number/Integer/99, returns Number/Integer. Returns TAny if there is
// no shared prefix.
func CommonAncestorType(a, b *Type) *Type {
	if a == nil || b == nil {
		return TAny
	}
	seen := make(map[*Type]bool)
	for d := a; d != nil; d = d.Parent {
		seen[d] = true
	}
	for d := b; d != nil; d = d.Parent {
		if seen[d] {
			return d
		}
	}
	return TAny
}

// CarrierDisjunctCap is the maximum number of alternatives a carrier
// disjunction may hold before it is widened to the common ancestor
// of all alternatives. Matches the report's recommended cap of 8.
const CarrierDisjunctCap = 8

// flattenAlternatives walks a carrier value and returns the unique
// type literals it represents. For a disjunct carrier, flattens its
// alternatives recursively; for any other carrier, returns a single
// type literal of its Parent.
func flattenAlternatives(v Value) []Value {
	if IsDisjunct(v) {
		di, _ := AsDisjunct(v)
		var out []Value
		for _, alt := range di.Alternatives {
			out = append(out, flattenAlternatives(alt)...)
		}
		return out
	}
	// A bare type literal IS its node — return it as-is. Taking
	// v.Parent of a literal would shift one lattice level up (the
	// node's parent), silently widening every stored alternative on
	// re-join. Carriers and concrete values stand for their Parent.
	if IsBareTypeNode(v) {
		return []Value{v}
	}
	return []Value{NewTypeLiteral(v.Parent)}
}

// carrierMixedConform reports whether v is a genuinely MIXED gradual
// carrier with respect to type t: a non-concrete Disjunct carrier whose
// flattened alternatives include at least one that conforms to t AND at
// least one that does not. Such a carrier makes a forward/stack dispatch
// split AMBIGUOUS — a concrete value drawn from it could match a more-
// specific overload (and be grabbed from the stack) or not (and be
// skipped, so the word forward-collects instead). The two splits leave
// different stacks, so the bytecode compiler cannot statically reproduce
// the runtime one. A pure carrier (all alternatives conform, or none do)
// and a bare dynamic carrier are NOT mixed and return false: the split is
// consistent, or is the dynamic path handled elsewhere.
func carrierMixedConform(v Value, t *Type) bool {
	if t == nil || !v.Carrier || IsConcrete(v) || v.Dynamic {
		return false
	}
	alts := flattenAlternatives(v)
	if len(alts) < 2 {
		return false
	}
	someConform, someReject := false, false
	for _, alt := range alts {
		// flattenAlternatives yields type LITERALS; the denoted type is
		// typeNodeOf(alt), NOT alt.Parent (a literal's Parent is the
		// denoted node's lattice parent — the `Boolean` literal's Parent
		// is `Scalar`).
		if node := TypeNodeOf(alt); node != nil && node.ConformsTo(t) {
			someConform = true
		} else {
			someReject = true
		}
	}
	return someConform && someReject
}

// JoinCarriers folds two carriers into a single carrier that
// represents the disjunction of both. Applies a few simple
// normalisations:
//
//   - Identical VTypes collapse to one carrier.
//   - If one side is a strict subtype of the other, the parent wins.
//   - Sibling literal types (e.g. Number/Integer/42 vs Number/Integer/99)
//     collapse to their nearest common ancestor (Number/Integer).
//   - Disjunctions wider than CarrierDisjunctCap widen to the common
//     ancestor of all alternatives.
//   - Otherwise a TDisjunct carrier is returned whose Data is a
//     DisjunctInfo listing the unique alternative type literals.
//
// This is the primary join used when the checker needs to combine
// two branch outcomes (e.g. `if` then/else).
// JoinCarriers merges two arm carriers (an `if`/loop/case branch result). If
// EITHER arm is gradual (dynamic), the merge is too — the same gradual
// contagion a dynamic operand already spreads through a dispatch result: a
// branch that may yield an unknown-typed value is itself optimistically typed,
// so the merge poly-matches a concrete slot instead of a strict disjunct
// rejecting it. Notably the "default-or-self" rebind `if (nd eq none) [Map]
// [nd]` over a `nd:Any` (gradual) param: the merge stays dynamic(Map|…) and a
// later `nd:Map` consumer matches, instead of a strict Disjunct(None|Map|…)
// failing no_signature (the tst/radix node-rebuild walkers). Looser, never
// tighter — a guard discharges the modality back to strict downstream.
func JoinCarriers(a, b Value) Value {
	out := joinCarriersInner(a, b)
	if a.Dynamic || b.Dynamic {
		out.Carrier = true
		out.Dynamic = true
	}
	return out
}

// isNoneArm reports whether a branch carrier is the None sentinel — the bare
// None type node, a `none` value, or a carrier bound to None.
func isNoneArm(v Value) bool {
	if v.Parent != nil {
		return v.Parent.Equal(TNone)
	}
	return IsNoneShape(v)
}

func joinCarriersInner(a, b Value) Value {
	// A None arm is a lattice-root literal — NewTypeLiteral(TNone) has BOTH
	// Data==nil AND Parent==nil (None has no lattice parent). The parent-math
	// collapse blocks below are unsafe against such a nil Parent: ConformsTo(nil)
	// and Equal(nil) are vacuously permissive, so `Integer.ConformsTo(None)` would
	// collapse the merge to NewCarrier(None.Parent==nil) — a Parent-less carrier
	// the engine HALTS on when it steps the `if` result (undefined stack entry).
	// Handle None arms explicitly: two None arms join to a None carrier; a mixed
	// None/value arm falls through to the alternatives-union path (None|T), the
	// tested general join — JoinCarrierStacks then flips that result gradual
	// (the optional / sentinel merge). Only the parent-math shortcuts are
	// bypassed; the union below already treats None as a first-class alternative.
	aNone := a.Parent == nil
	bNone := b.Parent == nil
	if aNone && bNone {
		return NewCarrier(TNone)
	}
	collapse := !aNone && !bNone
	if collapse && a.Parent.Equal(b.Parent) && !IsDisjunct(a) && !IsDisjunct(b) {
		out := a
		out.Carrier = true
		out.Data = nil
		// The merged carrier is a NEW value (an `if`/loop result), not arm `a`.
		// Keeping a's ID lets the result COLLIDE with a's own binding when an arm
		// returns a live local — `def v0 3 def v1 (if c [v0] [4])` makes the if-
		// result reuse v0's id, so a later `v0` reference resolves to the if-event
		// (the compiler bakes the wrong value). Mint a distinct identity.
		out.ID = GenerateID(IDPrefixForType(out.Parent))
		return out
	}
	if collapse && !IsDisjunct(a) && !IsDisjunct(b) {
		if a.Parent.ConformsTo(b.Parent) {
			// a is subtype of b → widen to b
			return NewCarrier(b.Parent)
		}
		if b.Parent.ConformsTo(a.Parent) {
			return NewCarrier(a.Parent)
		}
		// Collapse DIRECT siblings to their shared parent — the case
		// this was built for is value-tagged literals (Number/Integer/42
		// vs Number/Integer/99 → Number/Integer), and it also folds
		// Integer|Float → Number, where every dispatch the parent
		// reaches is one the alternatives reach identically. Distant
		// cousins (Integer vs String → Scalar) must NOT collapse: the
		// widened type changes first-match dispatch (Integer|String
		// reaching `add` picks the Scalar catch-all although the
		// Integer path takes [Number Number]) — keep them as a
		// disjunct so per-alternative dispatch
		// (disjunctPartitionReturns) sees the real alternatives.
		anc := CommonAncestorType(a.Parent, b.Parent)
		if anc != nil && !anc.Equal(TAny) &&
			a.Parent.Parent != nil && anc.Equal(a.Parent.Parent) &&
			b.Parent.Parent != nil && anc.Equal(b.Parent.Parent) {
			return NewCarrier(anc)
		}
	}
	// Gather unique alternatives across a and b, subsume subtypes,
	// then apply the width cap. SimplifyDisjunctAlts is the runtime
	// path's helper but produces identical output for the
	// type-literal-only inputs the carrier path supplies.
	combined := append([]Value(nil), flattenAlternatives(a)...)
	combined = append(combined, flattenAlternatives(b)...)
	alts := SimplifyDisjunctAlts(combined)
	if len(alts) == 1 {
		return CarrierOfLiteral(alts[0])
	}
	if len(alts) > CarrierDisjunctCap {
		t := TypeNodeOf(alts[0])
		for i := 1; i < len(alts); i++ {
			t = CommonAncestorType(t, TypeNodeOf(alts[i]))
		}
		return NewCarrier(t)
	}
	v := NewDisjunct(alts)
	v.Carrier = true
	return v
}

// RunCarrierBody runs a list body (a Value with Parent=TList) through a
// fresh sub-engine in check mode and returns the residual carrier
// stack. Returns nil if the body is not a concrete list. Requires
// that the registry is already in CheckMode (callers set it).
//
// Used by branch-aware words (e.g. `if`) to analyse each branch
// symbolically.
func RunCarrierBody(r *Registry, body Value) []Value {
	stk, _ := RunCarrierBodyWithDefs(r, body)
	return stk
}

// RunCarrierBodyKeepDefs is RunCarrierBody WITHOUT the def rollback — the
// check-mode twin of `do`'s runtime scoping, where body defs LEAK to the
// enclosing scope (`do [def x 5] end x add 1` → 6; a do-installed TYPE stays
// bound after the do). Rolling do-defs back was an infidelity with two
// symptoms: post-do reads flagged undefined (the wrapped-context
// false-positive family) and a do-installed type's binding vanishing while
// its minted part survived, so a later re-analysis of the SAME body tripped
// the parts conflict instead of the type-shadow path (validateTypeName's
// Defs.IsType skip). Branch / loop / quotation bodies keep the rollback —
// their execution is conditional and their defs are join-managed.
func RunCarrierBodyKeepDefs(r *Registry, body Value) []Value {
	return runCarrierBodyDefs(r, body, true)
}

// RunCarrierBodyWithDefs is the branch-aware helper that snapshots
// DefStack depths, runs the body through a sub-engine in check
// mode, and returns both the residual carrier stack and a map of
// every DefStacks[name] -> top-of-stack entry that was added
// during analysis. The top entry is popped (restored to snapshot)
// so the caller can decide whether to re-push, join, or discard.
//
// Only per-name "net additions" are reported. If a branch both
// pushes and pops for the same name, the net change is zero and
// the name is not in the returned map.
func RunCarrierBodyWithDefs(r *Registry, body Value) ([]Value, map[string]Value) {
	stk, adds := runCarrierBodyDefsAdds(r, body, false, false)
	return stk, adds
}

// RunCarrierCondBody is RunCarrierBodyWithDefs for an `if` CONDITION or a
// `case` code-body scrutinee: the fragment runs unconditionally exactly
// once BEFORE the branch decision, so it does NOT raise CondBodyDepth —
// an in-place fn redefinition there is not path-dependent and stays
// compilable (the paren-`do` condition twin compiles it with parity).
// Defs still roll back keep=false-style, exactly as before.
func RunCarrierCondBody(r *Registry, body Value) ([]Value, map[string]Value) {
	stk, adds := runCarrierBodyDefsAdds(r, body, false, true)
	return stk, adds
}

// runCarrierBodyDefs is the keep-defs entry over the shared body run.
func runCarrierBodyDefs(r *Registry, body Value, keep bool) []Value {
	stk, _ := runCarrierBodyDefsAdds(r, body, keep, false)
	return stk
}

func runCarrierBodyDefsAdds(r *Registry, body Value, keep, condFrag bool) ([]Value, map[string]Value) {
	if body.Data == nil {
		return nil, nil
	}
	elems, err := AsList(body)
	if err != nil || elems.IsNil() {
		return nil, nil
	}

	// Nested body analysis is not part of the enclosing straight
	// line: pause bytecode recording — unless a branch-lowering hook
	// armed fragment capture (the `if` ReturnsFn), in which case the
	// body's events record into a fragment for structured lowering.
	// Peek the arm BEFORE the guard consumes it: when recording into a
	// fragment, mark the body sub-engine element-eval-recordable so a
	// residual computed container it returns (`{a: x}` / `[x y]`, an if
	// arm's map/list value) records its OpMakeMap / OpMakeList assembly
	// instead of leaving an unresolvable residual. The branch/loop body
	// runs in the LIVE frame (its def-locals are present), so the
	// re-assembled-per-run operand semantics are sound — the same
	// property that already makes CONSUMED-arg container recording safe
	// in a fn body.
	recordable := r.Check.Recorder().PeekCaptureArm()
	defer r.Check.Recorder().BodyAnalysisGuard()()

	// Snapshot def-stack depths (all known names).
	snapshot := r.Defs.Snapshot()

	tokens := make([]Value, elems.Len())
	copy(tokens, elems.Slice())
	sub := New(r)
	sub.ElemEvalRecordable = recordable
	// Every body through here is a NESTED region (branch / loop /
	// quotation) — reached-conditionally by construction. Mark the depth
	// so unconditional-only diagnostics (unconditional_raise) stay silent.
	// A def-rolled-back body (keep=false — a branch arm or loop body) also
	// raises CondBodyDepth: unlike `do` (keep=true, which leaks its defs
	// unconditionally), its bindings are conditional, so an in-place fn
	// redefinition that clobbers an enclosing overload there is unsound to
	// compile (installDef consults CondBodyDepth to refuse it). Condition/
	// scrutinee fragments (condFrag — RunCarrierCondBody) are exempt: they
	// run unconditionally exactly once before the branch decision, so a
	// redefinition there is not path-dependent.
	r.Check.NestedBodyDepth++
	raiseCond := !keep && !condFrag
	if raiseCond {
		r.Check.CondBodyDepth++
		// A rolled-back CONDITIONAL body is a speculative region: an
		// `undef` of an enclosing binding inside it must not leak the
		// deletion into the model (SpecUndefBlocked — the wrapped-undef FP
		// class). keep=true (`do`) leaks by design; a condition fragment
		// runs unconditionally, so its undefs are real on both engines.
		r.Check.PushSpecBaseline(snapshot)
	}
	result, err := sub.Run(tokens)
	if raiseCond {
		r.Check.PopSpecBaseline()
		r.Check.CondBodyDepth--
	}
	r.Check.NestedBodyDepth--
	if err != nil {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "branch_error",
			Detail: "branch analysis error: " + err.Error(),
		})
		result = nil
	}

	// Keep-defs mode (`do` — leak fidelity): the body's bindings stay,
	// exactly as the runtime leaves them; nothing to report.
	if keep {
		return result, nil
	}
	// Collect the top of each def stack whose depth grew, then
	// restore depths back to snapshot.
	adds := map[string]Value{}
	for _, k := range r.Defs.Names() {
		before := snapshot[k] // zero for names not present before
		depth := r.Defs.Depth(k)
		if depth > before {
			top, _ := r.Defs.Top(k)
			adds[k] = top
			r.Defs.Truncate(k, before)
		}
	}
	return result, adds
}

// joinBranchDef merges a name's per-arm carriers for InstallJoinedDefs.
// When both inputs carry the SAME identity — the arms only NARROWED the
// enclosing binding (narrowDynamicUses preserves the value's ID; a read
// like `mkm (tab)` narrows-through-use without reassigning), never
// rebound it — the merge is an identity no-op, so KEEP that ID. The
// binding's value is genuinely unchanged across the branch, and
// preserving its ID keeps its compile seat (producing event / frame
// local) reachable for references AFTER the join. JoinCarriers otherwise
// mints a fresh ID — correct for a genuine if-RESULT (it must not
// collide with an arm's live local, JoinCarriers' own §), but for a
// merely-narrowed enclosing def that fresh ID strands every later
// reference as an unseated dynamic carrier ("fn call operand of unknown
// provenance"; the aless viewer's render fn hit exactly this). A GENUINE
// reassignment gives the arms DIFFERING IDs and keeps the fresh-ID join.
func joinBranchDef(a, b Value) Value {
	out := JoinCarriers(a, b)
	if a.ID != "" && a.ID == b.ID {
		out.ID = a.ID
	}
	return out
}

// InstallJoinedDefs merges the `adds` maps from two branches back
// into r.DefStacks. If both branches defined the same name, their
// carriers are joined via joinBranchDef and the joined carrier is
// pushed. If only one branch defined it, that def is pushed back —
// but joined with the pre-branch carrier (if any) since the other
// branch's path kept the original binding.
func InstallJoinedDefs(r *Registry, then, else_ map[string]Value) {
	seen := make(map[string]bool)
	for k, tv := range then {
		seen[k] = true
		if ev, ok := else_[k]; ok {
			r.Defs.Push(k, joinBranchDef(tv, ev))
			continue
		}
		// then-only: join with the pre-branch top-of-stack if any.
		if pre, ok := r.Defs.Top(k); ok {
			r.Defs.Push(k, joinBranchDef(tv, pre))
		} else {
			r.Defs.Push(k, tv)
		}
	}
	for k, ev := range else_ {
		if seen[k] {
			continue
		}
		// else-only: join with pre-branch top-of-stack.
		if pre, ok := r.Defs.Top(k); ok {
			r.Defs.Push(k, joinBranchDef(ev, pre))
		} else {
			r.Defs.Push(k, ev)
		}
	}
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
		aReal := i < len(a)
		bReal := i < len(b)
		if aReal {
			ai = a[i]
		} else {
			ai = NewCarrier(TNone)
		}
		if bReal {
			bi = b[i]
		} else {
			bi = NewCarrier(TNone)
		}
		joined := JoinCarriers(ai, bi)
		// Both arms produce a REAL value at this position and exactly one is
		// None: a None-vs-value merge is the optional / builder sentinel
		// pattern (`if (nd eq none) [build-node] [nd]`, where the else arm
		// hands back the still-`none` receiver) — None is "absent / not built
		// yet" and the downstream code targets the REAL type, so the merge is
		// gradual. Without this a DIRECT call passing a concrete `none`
		// (tst/radix's `TstMap.set` on an empty map → `none key val
		// tst-insert`) merged to a STRICT Disjunct(None|Map) whose None
		// alternative made every node-rebuild `get` fail no_signature — whereas
		// the same code reached through a gradual `:Any` param already merged
		// dynamically (JoinCarriers' arm-dynamic rule). Gated to both-real so a
		// PADDED None (a variadic 0-or-1 arm — `if cond [98]`) keeps its
		// precise strict shape; only a genuine value-vs-none branch widens.
		// Looser, never tighter; a guard discharges the modality back to strict.
		if aReal && bReal && (isNoneArm(ai) != isNoneArm(bi)) {
			joined.Carrier = true
			joined.Dynamic = true
		}
		out[i] = joined
	}
	return out
}

// loopAnalysisRounds bounds the Kleene iteration for loop-body
// analysis: round 1 with the pre-loop bindings, then re-runs with the
// joined bindings until stable. Three rounds suffice for ascent in a
// join-semilattice whose height is bounded by CarrierDisjunctCap.
const loopAnalysisRounds = 3

// FnAnalysisQuota caps how many distinct call shapes one fn's body is
// analysed for before the checker answers from the declaration (A9).
const FnAnalysisQuota = 64

// AnalyseLoopBody analyses a loop body to a bounded fixed point
// (design/checker-accuracy-review.10.md A4). Each round binds the
// loop's own names (iterator …) as carriers, runs the body, and
// JOINS the body's net def additions back into the enclosing
// bindings — "the loop may run zero times" is the join with the
// pre-loop binding, exactly InstallJoinedDefs' one-branch rule. If
// the joined bindings changed, the body is re-analysed with them (a
// rebinding like `def acc (acc add 0.5)` needs the second round to
// see Integer|Float), up to loopAnalysisRounds.
//
// Only the FINAL round's diagnostics are kept — earlier rounds run
// against not-yet-stable bindings and would both duplicate and
// misreport.
//
// The joined post-loop bindings are left installed (they ARE the
// post-loop environment); the loop-local binds are popped. Returns
// the final round's residual carrier stack.
//
// provenTrips asserts the CALLER's proof that the loop executes at least
// once at run time — a static trip count >= 1 (forCarrierAnalyse's
// staticBounds). Combined with the body carrying no flow-control sentinel
// (bodyHasSentinel — a break/continue/return can bypass a site or discard
// an iteration's spilled values), it gates the LoopBodyDepth stamp the
// S9.2a first-value split consults: the split's soundness argument is
// "every enclosing body runs unconditionally per iteration AND the split
// site is reached with its residual intact", which a computed count (zero
// trips leak the analysis-only binding) or loop control (PR #280 review:
// `continue` bypassed the bind, `break` kept a discarded iteration's
// value) breaks. A non-proven loop body still analyses identically — it
// just declines the split (NestedBodyDepth != LoopBodyDepth).
func AnalyseLoopBody(r *Registry, body Value, bindNames []string, bindVals []Value, provenTrips bool) []Value {
	proven := provenTrips && !bodyHasSentinel(body)
	// Loop-lowering hook (`for`): when armed, register the loop
	// bindings as VM locals and capture each round's events as a
	// fragment — the final round's capture (the stable one) is what
	// the caller's RecordLoop consumes via TakeFragment.
	es := r.Check.Recorder()
	loopCapture := es.ConsumeLoopArm()
	if loopCapture {
		for _, v := range bindVals {
			es.RegisterLocal(v.ID)
		}
		// Loop-carried def rebinds: a pre-loop `def` the body REBINDS gets a
		// unit frame slot (NoteLoopCarried per round below), a store at each
		// rebind site (RecordDefRebind, from the def handler), and a slot
		// resolution for every round's joined binding — so a read on a later
		// iteration or after the loop compiles instead of refusing "operand
		// of unknown provenance". EndLoopCarried exposes the slot inits to
		// the RecordLoop that follows this analysis.
		es.BeginLoopCarried()
		defer es.EndLoopCarried()
	}
	var stk []Value
	var installed []string
	diagBase := len(r.Check.Diagnostics)
	prev := map[string]Value{}
	for round := 0; round < loopAnalysisRounds; round++ {
		r.Check.TruncateDiagnostics(diagBase)
		for i, n := range bindNames {
			r.Defs.Push(n, bindVals[i])
		}
		// Checkpoint the recording pools before an armed round: only the FINAL
		// (stabilised) round's fragment is kept, so a non-final round's interned
		// consts / island spans and its SiteCounts are rolled back rather than
		// orphaned into the Program (bytecode artifact + metric bloat). The
		// def-stack convergence (r.Defs) is independent of the recording, so it
		// proceeds across rounds untouched.
		var cp EmitCheckpoint
		if loopCapture {
			cp = es.Checkpoint()
			es.ArmBranchCapture()
		}
		var adds map[string]Value
		if proven {
			r.Check.LoopBodyDepth++
		}
		stk, adds = RunCarrierBodyWithDefs(r, body)
		if proven {
			r.Check.LoopBodyDepth--
		}
		for i := len(bindNames) - 1; i >= 0; i-- {
			r.Defs.Pop(bindNames[i])
		}
		// Expose the original pre-loop bindings before re-joining.
		for i := len(installed) - 1; i >= 0; i-- {
			r.Defs.Pop(installed[i])
		}
		installed = installed[:0]
		joined := map[string]Value{}
		// Deterministic name order: carried-slot allocation and the joined
		// installs must not depend on Go map iteration (slot numbering and
		// the emit goldens would jitter run to run).
		names := make([]string, 0, len(adds))
		for k := range adds {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			v := adds[k]
			if pre, ok := r.Defs.Top(k); ok {
				j := JoinCarriers(v, pre)
				joined[k] = j
				if loopCapture {
					// A rebind of a PRE-EXISTING binding is loop-carried:
					// register (or refresh) its slot and alias this round's
					// joined ID so the next round's / post-loop reads resolve.
					es.NoteLoopCarried(k, j, pre)
				}
			} else {
				joined[k] = v
			}
		}
		for _, k := range names {
			r.Defs.Push(k, joined[k])
			installed = append(installed, k)
		}
		// Stabilised when the body adds no bindings (the common single-round
		// case) or the joined bindings equal the previous round's. The final
		// round is the one that stabilises, or the last permitted round.
		stable := len(joined) == 0
		if !stable {
			stable = len(joined) == len(prev)
			if stable {
				for k, v := range joined {
					pv, ok := prev[k]
					if !ok || !ValuesEqual(pv, v) {
						stable = false
						break
					}
				}
			}
			prev = joined
		}
		if loopCapture && !stable && round < loopAnalysisRounds-1 {
			es.Rollback(cp)
		}
		if stable {
			break
		}
	}
	return stk
}

// GuardClause describes one `x is T` clause detected in a condition.
type GuardClause struct {
	Name string
	Type *Type
	// ThenOnly marks a clause whose fact is sound only in the THEN branch —
	// a predicate-derived fact (`if (param is T) …` in the predicate's own
	// body, surfaced via a 2-token `pred x` guard). The complement (else)
	// carries no information for these, so ApplyComplementNarrowing skips
	// them; a direct `x is T` clause narrows both arms and stays ThenOnly
	// false.
	ThenOnly bool
}

// extractGuardClauses walks a condition list looking for triplets
// `Word(x) Word(is) TypeLiteral(T)` and returns the corresponding
// GuardClause entries. Skips anything that doesn't resolve to a
// bare type literal or an ObjectType. Accepts type-word references
// by looking them up on DefStacks.
func extractGuardClauses(r *Registry, condList Value) []GuardClause {
	if r == nil || condList.Data == nil {
		return nil
	}
	// A pre-evaluated paren condition arrives as a Boolean carrier
	// whose GuardFactInfo payload preserves the group's original
	// tokens (A3) — extract from those exactly as from a list body.
	var elems []Value
	if gf, ok := condList.Data.(GuardFactInfo); ok {
		elems = gf.Toks
	} else {
		list, err := AsList(condList)
		if err != nil || list.IsNil() || list.Len() < 3 {
			return nil
		}
		elems = list.Slice()
	}
	if len(elems) < 2 {
		return nil
	}
	// A compound boolean condition (`(a is Map) or (b is Map)`, `not (x is T)`)
	// does not license per-clause narrowing: under a disjunction either clause
	// could be the true one, and a negation inverts the arm each fact holds in.
	// Bail entirely — no component clause is sound — rather than narrow one.
	for _, e := range elems {
		if e.Parent.Equal(TWord) {
			if w, werr := AsWord(e); werr == nil {
				switch w.Name {
				case "or", "nor", "xor", "xnor", "not":
					return nil
				}
			}
		}
	}
	var out []GuardClause
	for i := 0; i+2 < len(elems); i++ {
		if !elems[i].Parent.Equal(TWord) || !elems[i+1].Parent.Equal(TWord) {
			continue
		}
		wx, err := AsWord(elems[i])
		if err != nil {
			continue
		}
		wis, err := AsWord(elems[i+1])
		if err != nil || wis.Name != "is" {
			continue
		}
		tv := elems[i+2]
		var minted *Type
		if tv.Data != nil && tv.Parent.Equal(TWord) {
			inner, _ := AsWord(tv)
			if e, ok := r.Defs.TopEntry(inner.Name); ok {
				tv = e.Body
				minted = e.TypeDef
			} else if t, ok := ResolveBuiltinTypeName(inner.Name); ok {
				// Builtin arm of the canonical cascade — post-opacity
				// (ADR-012 rule 4) `x is Integer` carries a Word here.
				tv = NewTypeLiteral(t)
			}
		}
		if tv.Data != nil && !IsClassType(tv) && !(tv.IsDepScalar() && minted != nil) {
			continue
		}
		// A bare type-literal clause IS its type; an ObjectType keeps
		// its type at Parent (the minted object-type node); a PREDICATE
		// refine (DepScalar body) narrows to its MINTED lattice node
		// (DefEntry.TypeDef — the body value's Parent is only the base
		// family), whose depScalarUnifier admits an abstract carrier
		// tagged with it nominally. The one membership rule makes the
		// guard exactly the test every downstream boundary re-asks, so
		// the then-branch may treat the name as the refined type. This
		// legalizes validate-then-call (`if (x is Big) [g x] [0]` with
		// x:Integer), previously a gating no_signature false positive
		// while the program ran correctly — the named blocker for
		// check-by-default (completion plan 2.2).
		guardType := tv.Parent
		switch {
		case tv.IsDepScalar() && minted != nil:
			guardType = CanonicalType(r, minted)
		case tv.Data == nil:
			gt := tv
			guardType = &gt
		}
		out = append(out, GuardClause{Name: wx.Name, Type: guardType})
	}
	// 2-token predicate guard: `if (is-map x) [then] [else]`. A user predicate
	// whose body is itself an `x is T` test (predicateImpliedType) implies its
	// argument is T in the THEN branch — but ONLY there (ThenOnly): a false
	// return says nothing about which non-T shape x has. The variable must be a
	// live binding; the predicate must reduce to a single `param is T` fact.
	if len(out) == 0 && len(elems) == 2 &&
		elems[0].Parent.Equal(TWord) && elems[1].Parent.Equal(TWord) {
		wp, perr := AsWord(elems[0])
		wx, xerr := AsWord(elems[1])
		if perr == nil && xerr == nil {
			if _, bound := r.Defs.Top(wx.Name); bound {
				if t, tok := predicateImpliedType(r, wp.Name); tok {
					out = append(out, GuardClause{Name: wx.Name, Type: t, ThenOnly: true})
				}
			}
		}
	}
	return out
}

// BoolWord returns "true" / "false" for use in human-readable
// diagnostic text.
func BoolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// LiteralCondValue inspects a condition list for a single boolean
// literal (true/false word or Boolean carrier). Returns (value,
// true) when the condition is statically determinable, or (false,
// false) otherwise. Used by `if` analysis to warn about
// unreachable branches.
func LiteralCondValue(condList Value) (bool, bool) {
	if condList.Data == nil {
		return false, false
	}
	list, err := AsList(condList)
	if err != nil || list.IsNil() || list.Len() != 1 {
		return false, false
	}
	only := list.Get(0)
	// A pre-evaluated paren cond arrives wrapped in a GuardFactInfo carrier
	// (the A3 guard-fact attach); its Prev payload holds the value the group
	// actually reduced to. Read a decided Boolean straight through the wrapper
	// so the unreachable-branch analysis still fires.
	if gf, gok := only.Data.(GuardFactInfo); gok {
		if bp, bok := gf.Prev.(BoolPayload); bok {
			return bp.B, true
		}
	}
	// Bare true/false word (parser emits these as Word values that
	// resolve to booleans in engine.stepWord; in check mode the
	// words stay as Words until the branch runs).
	if only.Parent.Equal(TWord) {
		w, err := AsWord(only)
		if err == nil {
			if w.Name == "true" {
				return true, true
			}
			if w.Name == "false" {
				return false, true
			}
		}
	}
	// Concrete Boolean value with Data set (post-runtime path).
	if only.Parent.ConformsTo(TBoolean) && only.Data != nil {
		b, err := AsBoolean(only)
		if err == nil {
			return b, true
		}
	}
	return false, false
}

// ApplyGuardNarrowing installs then-branch narrowings for each
// `x is T` clause in the condition. Returns a restore func to pop
// the narrowings after the then-branch runs.
func ApplyGuardNarrowing(r *Registry, condList Value) func() {
	noop := func() {}
	if !r.Check.IsActive() {
		return noop
	}
	clauses := extractGuardClauses(r, condList)
	if len(clauses) == 0 {
		return noop
	}
	deadArm := false
	for _, c := range clauses {
		narrowed := NewCarrier(c.Type)
		// is-narrowing is a static-only refinement: at runtime the binding is
		// UNCHANGED, so the narrowed carrier must keep the source's value ID — its
		// provenance (param slot / producing event). NewCarrier mints a FRESH ID
		// with no producedBy/localByID entry, so resolveOperand fails ("fn call
		// operand of unknown provenance") when the narrowed value feeds a user
		// call — the stats as-summary `if (x is List) [build x] [x]` shape. The
		// slot already holds the right runtime value because it IS the same
		// binding, so no value-passing half is needed (unlike a closure capture).
		if cur, ok := r.Defs.Top(c.Name); ok {
			narrowed.ID = cur.ID
			// MEET the guard type with the binding's current (abstract) carrier
			// rather than overwriting it with a bare NewCarrier(c.Type): an
			// incompatible pair collapses to Never — a DEAD then-arm the runtime
			// never runs — and a compound meet keeps its payload. A concrete
			// binding is a per-shape analysis artifact (see the redundant_guard
			// note) whose lattice tag under-approximates predicate membership, so
			// it keeps the plain NewCarrier form. A dynamic binding stays dynamic
			// through the meet so the narrowed value still discharges downstream
			// modality checks.
			if cur.Carrier && !IsConcrete(cur) && c.Type != nil {
				meet := TandValues(cur, NewTypeLiteral(c.Type))
				if IsNeverShape(meet) {
					deadArm = true
				} else if IsBareTypeNode(meet) {
					narrowed = NewCarrier(ValueType(meet))
					narrowed.ID = cur.ID
				} else {
					meet.Carrier = true
					narrowed = meet
					narrowed.ID = cur.ID
				}
				if cur.Dynamic && !deadArm {
					narrowed.Dynamic = true
				}
			}
			// Advisory (non-gating): the binding's STATIC type already
			// entails the guard, so the check cannot fail — the residue the
			// local-reasoning report calls the misleading defensive check
			// (`if (n is Big) …` where n:Big is already in the signature).
			// Non-concrete STRICT carriers only: a dynamic binding genuinely
			// needs the guard (it DISCHARGES the modality); a CONCRETE
			// binding is a per-shape analysis artifact (an `[x:Any]` param
			// analysed for the call `f 5` binds the literal 5, whose Integer
			// tag would flag a guard the fn's OTHER callers rely on) and its
			// lattice tag under-approximates predicate membership anyway
			// (value-level entailment — interval reasoning — is future
			// work). A non-concrete strict carrier IS the declared-type
			// record, so tag conformance is shape-independent. Dedup: fn
			// bodies re-analyse per shape and fixpoint round.
			if cur.Carrier && !IsConcrete(cur) && !cur.Dynamic &&
				cur.Parent != nil && c.Type != nil &&
				cur.Parent.ConformsTo(c.Type) {
				detail := "guard is always true: " + c.Name + " is already " +
					c.Type.String() + " — drop the check or make it an assertion"
				dup := false
				for _, d := range r.Check.Diagnostics {
					if d.Code == "redundant_guard" && d.Detail == detail {
						dup = true
						break
					}
				}
				if !dup {
					r.Check.AddDiagnostic(CheckDiagnostic{
						Code:   "redundant_guard",
						Detail: detail,
						Word:   "is",
						Row:    condList.Pos().Row,
						Col:    condList.Pos().Col,
					})
				}
			}
		}
		r.Defs.Push(c.Name, narrowed)
	}
	if deadArm {
		// The guard can never hold for this arg shape — the then-arm is dead.
		// Suppress its (unreachable) dispatch errors; the runtime never runs it.
		r.Check.SuppressBodyErrors++
	}
	return func() {
		if deadArm {
			r.Check.SuppressBodyErrors--
		}
		for _, c := range clauses {
			r.Defs.Pop(c.Name)
		}
	}
}

// ApplyComplementNarrowing installs else-branch narrowings — for
// each `x is T` clause it tries to compute the complement of T in
// x's current carrier type and, if non-trivial, pushes the
// complement carrier onto x's DefStack. Currently only refines
// when x's existing binding is a disjunction: the matching
// alternative is subtracted. Returns a restore func.
func ApplyComplementNarrowing(r *Registry, condList Value) func() {
	noop := func() {}
	if !r.Check.IsActive() {
		return noop
	}
	clauses := extractGuardClauses(r, condList)
	// Else-branch narrowing (and its dead-arm suppression) is sound only when
	// the extracted clauses COMPLETELY characterise the condition — i.e. it is a
	// single `x is T` guard. For a conjunction `A and B`, the else path is
	// `notA or notB`: neither clause can be subtracted (either could be the
	// false one), and one exhaustive clause does NOT make the else unreachable
	// (it stays reachable whenever the other clause fails). So restrict to the
	// single-clause case and leave a conjunction's else untouched. (In practice
	// a compound condition's inner guards are captured as opaque ParenExpr
	// values, so extractGuardClauses already returns 0 clauses for them; this
	// guard makes the single-guard assumption explicit and sound-by-construction
	// rather than reliant on that representation.)
	if len(clauses) != 1 {
		return noop
	}
	type applied struct{ name string }
	var pushed []applied
	deadArm := false
	for _, c := range clauses {
		// A ThenOnly clause (a predicate-derived fact) carries no else-branch
		// information: a false predicate says nothing about which non-T shape x
		// has, so there is nothing to subtract.
		if c.ThenOnly {
			continue
		}
		cur, ok := r.Defs.Top(c.Name)
		if !ok {
			continue
		}
		// A DYNAMIC binding's else path stays gradual: the guard failed at
		// runtime, but the binding's static type is Any (or a dynamic carrier),
		// so subtracting T would wrongly forbid every later use. Leave it as-is.
		if cur.Dynamic {
			continue
		}
		// Else-branch narrowing: x had type `cur`; the guard `x is T`
		// failed, so on the else path x is `cur tand (tnot T)`. The
		// negation + intersection algebra computes this uniformly and is
		// strictly more capable than the old exact-alternative subtraction:
		//   - a disjunct loses every alternative contained in T, including
		//     when T is a *supertype* of an alternative ((Integer tor
		//     String) tand tnot Number → String);
		//   - a plain type disjoint from T is unchanged (no-op);
		//   - a type wholly inside T collapses to Never (unreachable else).
		complement := NegateType(NewTypeLiteral(c.Type))
		narrowed := TandValues(cur, complement)
		if IsNeverShape(narrowed) {
			// Else branch is unreachable for x — the guard held for every value
			// of x's type. Mark the arm dead (so its unreachable dispatch errors
			// are suppressed) and leave the binding as-is rather than push a
			// Never carrier that fails every later use. Sound because this is the
			// sole clause of the condition (single-guard gate above).
			deadArm = true
			continue
		}
		// Normalise to carrier form: a single surviving type becomes a
		// carrier of that type (Parent = the type, like NewCarrier); a
		// disjunct or other compound keeps its payload and is marked
		// abstract.
		if IsBareTypeNode(narrowed) {
			narrowed = NewCarrier(ValueType(narrowed))
		} else {
			narrowed.Carrier = true
		}
		if ValuesEqual(narrowed, cur) {
			// Complement did not refine cur (T disjoint from cur, or boru
			// has no positive representation for the exact difference).
			continue
		}
		// Preserve the source binding's value ID (see ApplyGuardNarrowing): the
		// else-branch value is the SAME runtime binding, statically refined to the
		// complement type, so it must resolve to cur's provenance. Set AFTER the
		// ValuesEqual(narrowed, cur) check above so the "did not refine" early-out
		// (which can compare by ID) is unaffected.
		narrowed.ID = cur.ID
		r.Defs.Push(c.Name, narrowed)
		pushed = append(pushed, applied{name: c.Name})
	}
	if deadArm {
		// The guard held for every value of x's type — the else-arm is dead.
		// Suppress its (unreachable) dispatch errors; the runtime never runs it.
		r.Check.SuppressBodyErrors++
	}
	if len(pushed) == 0 && !deadArm {
		return noop
	}
	return func() {
		if deadArm {
			r.Check.SuppressBodyErrors--
		}
		for _, p := range pushed {
			r.Defs.Pop(p.name)
		}
	}
}

// FnAnalysisKey builds the memo key for one fn-body analysis: scope id +
// name + arg type paths + captured-name set + the body's construction
// site. The captures are included so two anonymous lambdas with identical
// bodies but different capture sets don't collide; the construction
// site (the first body token's source position) so two DIFFERENT
// definitions sharing a name and arg types — `def f fn […] … def f
// fn […]` — don't collide either (without it the compile pass bound
// every call to the FIRST definition's code unit). The same
// construction re-analysed (recursion, repeated calls) carries the
// same body tokens, so memoisation and in-flight recursion detection
// are unaffected.
//
// scopeID is the AnalysisScopeID of the registry whose body is being
// analysed. It namespaces the key so a module sub-registry's fn cannot
// alias a same-named, same-positioned parent fn once a check pass is
// shared across registries (design/module-fn-checkstate-ownership.1.md
// §5a) — the position suffix alone does not disambiguate, because parent
// and module are parsed from independent sources whose positions overlap.
// core_helpers' compile hook must build the SAME key (its FnSummaries
// delete relies on the match) — that's why this is a named helper, not
// two inlined loops, and why every caller passes r.AnalysisScopeID() of
// the same registry r that AnalyseFnBody runs the body in.
// carrierTypeName names the lattice type a carrier value denotes, for memo
// keying. A normal carrier carries its type in Parent; a root-node carrier
// (None / Any / Never) has a nil Parent because it IS its own lattice node, so
// its name comes from the value itself. Never deref Parent unguarded here — a
// `none` fold-seed promoted to a dynamic carrier reaches this path with a nil
// Parent and would otherwise panic (tst's `map-from-entries` accumulator).
func carrierTypeName(v Value) string {
	if v.Parent != nil {
		return v.Parent.String()
	}
	return v.String()
}

func FnAnalysisKey(scopeID uint64, name string, args []Value, captures []CapturedBinding, body []Value) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(scopeID, 10))
	sb.WriteByte('#')
	sb.WriteString(name)
	sb.WriteByte('#')
	for _, a := range args {
		sb.WriteString(carrierTypeName(a))
		sb.WriteByte(',')
	}
	if len(captures) > 0 {
		sb.WriteByte('|')
		for _, cb := range captures {
			sb.WriteString(cb.Name)
			sb.WriteByte(':')
			sb.WriteString(carrierTypeName(cb.Value))
			sb.WriteByte(',')
		}
	}
	if len(body) > 0 {
		sb.WriteByte('@')
		sb.WriteString(strconv.Itoa(body[0].Pos().Row))
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(body[0].Pos().Col))
	}
	return sb.String()
}

// fnQuotaKey identifies a fn DEFINITION for the per-fn analysis quota:
// scope + name + body source position. It deliberately differs from
// FnAnalysisKey in two ways. It OMITS the arg shapes — the quota exists to
// COUNT distinct arg shapes per definition, so the shapes cannot be part of
// the key that groups them. And unlike a bare name it distinguishes the many
// closure bodies that share a synthetic "<word>$body" name (each$body,
// fold$body, scan$body, …) by their source position: a name-only key
// conflated every higher-order closure in the whole program under one budget,
// so a module with 64+ distinct `each` loops exhausted the quota and forced
// later loops to bail to a provenance-less dynamic Any — which the compiler
// then refused ("code-body word each (Stage 2)"). Keyed by definition site,
// each distinct closure gets its own budget while the same body re-analysed
// under many arg shapes still shares one counter.
// The name may be empty (a transparent anonymous body); it is written
// verbatim, since the body position that follows already distinguishes
// distinct anonymous sites. The human-facing diagnostic uses a separate
// displayName, so the key need not spell "<anon>".
func fnQuotaKey(scopeID uint64, name string, body []Value) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(scopeID, 10))
	sb.WriteByte('#')
	sb.WriteString(name)
	if len(body) > 0 {
		sb.WriteByte('@')
		sb.WriteString(strconv.Itoa(body[0].Pos().Row))
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(body[0].Pos().Col))
	}
	return sb.String()
}

// declaredReturnBail synthesizes the result carriers for an analysis that
// cannot (or need not) run the body: one fresh carrier per declared return
// type, or a single dynamic Any when nothing is declared. Shared by the
// quota and in-flight-recursion short circuits.
func declaredReturnBail(declared []*Type) []Value {
	if len(declared) > 0 {
		out := make([]Value, len(declared))
		for i, t := range declared {
			out[i] = NewCarrier(t)
		}
		return out
	}
	return []Value{NewDynamicCarrier(TAny)}
}

// refineRecursiveSummary is AnalyseFnBody's bounded Kleene refinement (A2):
// the first run consumed an Any in-flight bail, so the summary was computed
// under the weakest hypothesis. Seed the memo with it and re-analyse — the
// recursive call now reads the seeded hypothesis from the cache — joining
// each round's result into the hypothesis until stable, up to two extra
// rounds. Only the final round's diagnostics are kept.
func refineRecursiveSummary(r *Registry, key string, diagBase int, result []Value, runOnce func() []Value) []Value {
	for round := 0; round < 2; round++ {
		r.Check.FnSummaries[key] = result
		r.Check.TruncateDiagnostics(diagBase)
		next := runOnce()
		joined := JoinCarrierStacks(result, next)
		if carrierStacksEqual(joined, result) {
			break
		}
		result = joined
	}
	return result
}

// runFnBodyOnce performs one full body analysis for AnalyseFnBody: snapshot
// def-stack depths so any defs the body, captures, or parameter bindings
// created unwind afterwards. The same snapshot is pushed as the fn-entry
// baseline so any inner fn/afn construction inside the body sees this scope
// as its enclosing-fn baseline — without it, ComputeCaptures would treat
// outer params as if they lived at module/global scope and miss the capture.
func runFnBodyOnce(r *Registry, name string, paramNames []string, body, args []Value, captures []CapturedBinding, anonymous bool) []Value {
	snapshot := r.Defs.Snapshot()
	r.PushFnBaseline(snapshot)
	defer r.PopFnBaseline()

	// Expose the params as the per-call args list so a body that reads
	// `args` / `args.N` resolves them in check mode. The params ARE the
	// frame's leading locals (0..n-1), so `args.N` folds to that local — at
	// lowering it becomes PUSH_LOCAL N, no runtime args stack needed
	// (carrierResults' `args` projection + tryFoldStaticIndex). Pushed
	// alongside the FnBaseline; popped together.
	_ = r.Args.Push(NewList(append([]Value(nil), args...)))
	defer func() { _, _ = r.Args.Pop() }()

	// Install lexical captures first so params (installed below)
	// shadow same-named captures — innermost binding wins,
	// matching runtime dispatch.
	for _, cb := range captures {
		r.Defs.Push(cb.Name, cb.Value)
	}

	// Bind named parameters as simple defs (carrier-typed).
	// Unnamed parameters flow through the stack — push them
	// before the body.
	var input []Value
	hasUnnamed := false
	for i, arg := range args {
		if i < len(paramNames) && paramNames[i] != "" {
			r.Defs.Push(paramNames[i], arg)
		} else {
			// An unnamed FN-VALUE param is inert frame DATA under the
			// arguments-are-inert unification (the interpreter no longer
			// auto-fires a frame argument — design/ARG-SEMANTICS-
			// UNIFICATION.0.md), and the unit model now mirrors the frame
			// exactly: the value re-pushes at entry, `args.N` folds to its
			// local, and the RET's NUnnamed trim discards an unconsumed
			// copy — so the former guard refusal is retired.
			input = append(input, arg)
			hasUnnamed = true
		}
	}
	input = append(input, body...)

	// Record whether this frame has stack-flowing unnamed params so a body
	// `args` / `args.N` read can refuse in compile mode (the unnamed input
	// stays live on the body stack; folding args.N over it strands the input
	// — design/EDGE-SPEC-FINDINGS.0.md §4). Save/restore around the body run
	// so nested analyses see their own frame's answer.
	prevUnnamed := r.Check.ArgsFrameUnnamed
	r.Check.ArgsFrameUnnamed = hasUnnamed
	defer func() { r.Check.ArgsFrameUnnamed = prevUnnamed }()

	sub := New(r)
	// When this body is being RECORDED into a compiled CALLBACK unit (a
	// stored-fn / spawn body — see compileStoredFnUnit / compileStoredBody,
	// name "storedfn$body" / "spawnbody$body"), mark the body sub-engine
	// element-eval-recordable so a residual COMPUTED container it returns
	// (`{message: (join …)}` / `[a b]`, a bare map/list result) records its
	// OpMakeMap / OpMakeList assembly instead of leaving an unresolvable
	// residual that refuses "body result of unknown provenance" (the
	// mini-redis catch-all shape). Mirrors the branch arm's treatment
	// (RunCarrierBodyWithDefs, peekCaptureArm).
	//
	// Admitted for CALLBACK bodies and MULTI-TOKEN fn bodies. A callback is
	// only ever invoked via InvokeCallback / CallBoru, which evaluate the
	// body residual IN the live frame on both engines. A multi-token body's
	// trailing computed container now ALSO evaluates in-frame on every
	// interpreter dispatch path — CallBoru-class and same-registry spliced,
	// consumed and unconsumed alike (mini-s3's s3-parse-range
	// `{from: from upto: upto}`; the historical spliced-path deferral that
	// blocked this admission is gone) — so the recorded OpMakeMap /
	// OpMakeList (which re-assembles per run) matches the interpreter
	// exactly. A SINGLE-LITERAL container body must NOT enable this: that
	// is the pinned no-closures transparency (`def mk fn [[c1:Integer]
	// [List] [[c1]]] mk 9` → the MODULE binding, def-node-binding.tsv §3 —
	// maps behave identically), where in-frame assembly would bake the
	// param and diverge. Such a body keeps refusing and falls back,
	// byte-identically. The admission is BodyEvalsResidual — the same
	// predicate the frame-tail builders use — so a single
	// paren-expression body (which the interpreter also evaluates
	// in-frame) records too.
	// A NAMED fn (!anonymous) always evaluates its body in-frame, so its
	// residual container is assembled against the live params and records
	// like any in-frame computation. Only an anonymous lambda keeps the
	// single-bare-literal transparency (BodyEvalsResidual), where in-frame
	// assembly would bake the param and diverge — that shape keeps refusing.
	if r.Check.Recorder().Active() && (isCallbackBodyName(name) || !anonymous || BodyEvalsResidual(body)) {
		sub.ElemEvalRecordable = true
	}
	result, err := sub.Run(input)
	if err != nil {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "fn_body_error",
			Detail: "fn body analysis error for " + name + ": " + err.Error(),
			Word:   name,
		})
		// A body that ERRORS under an ARMED recording (this analysis is being
		// compiled into an open fn/closure unit) cannot be faithfully lowered:
		// the unit would close EMPTY (the error aborted the body before its
		// residual recorded), and an empty unit SILENTLY DIVERGES from the
		// interpreter (a `var`-let or higher-order body whose check-mode error
		// is mere imprecision — e.g. `get` on an element carrier — runs fine at
		// runtime, so the VM's empty closure raises `body produced no result`
		// where the interpreter succeeds). Refuse so the program falls back to
		// the interpreter instead. Only when active: a SUSPENDED (plain) nested
		// analysis records nothing anyway and must not latch the program.
		if es := r.Check.Recorder(); es.Active() {
			es.MarkUncompilable("fn body analysis error in " + name + ": " + err.Error())
		}
		result = nil
	}
	r.Defs.Restore(snapshot)
	return result
}

// isCallbackBodyName reports whether name is a stored-fn / spawn callback
// body — compileClosureBody builds "storedfn$body" / "spawnbody$body" for the
// words "storedfn" / "spawnbody" (callable_words.go). Such a body is invoked
// only via InvokeCallback / CallBoru, which evaluate a residual COMPUTED
// container (`{message: (join …)}` / `[a b]`) IN the live frame on both
// engines, so recording its OpMakeMap / OpMakeList assembly is safe (it
// re-assembles per run, matching the interpreter). A normal user fn applied
// directly at top level leaves a DEFERRED residual the interpreter evaluates
// after the frame pops — recording there would diverge, so runFnBodyOnce gates
// elemEvalRecordable on this predicate.
func isCallbackBodyName(name string) bool {
	return name == "storedfn$body" || name == "spawnbody$body"
}

// AnalyseFnBody runs a user-defined fn body through a sub-engine in
// check mode, treating named parameters as deffed values bound to
// their arg carriers and unnamed parameters as pre-pushed stack
// values. Results are cached on the registry keyed by (name,
// arg-types) so recursive functions converge instead of looping.
//
// declared is the signature's declared return types (nil =
// unchecked). It is the induction hypothesis for recursion
// (design/checker-accuracy-review.10.md A2): an in-flight recursive
// call yields carriers of the DECLARED returns — the end-of-body
// return check is the matching proof obligation — instead of the
// everything-matches Any. For unchecked fns the Any bail-out
// remains, but a summary computed under it is refined by bounded
// re-analysis (the bail's result seeds the memo as the hypothesis
// for the next round) rather than cached as final.
//
// Returns the residual carrier stack. An empty or nil return means
// the analyser aborted (recursion detected or body not available) —
// callers should treat that as an Any carrier.
func AnalyseFnBody(r *Registry, name string, paramNames []string, body []Value, args []Value, captures []CapturedBinding, declared []*Type, anonymous bool) []Value {
	// Fn-body analysis runs nested sub-engines — not part of the
	// caller's straight line; pause bytecode recording, UNLESS a fn
	// compilation armed capture (StartFnCompile): the body's events then
	// record into the open fn fragment. Resolved at ENTRY, above every
	// early return below, so an armed analysis that bails early — empty
	// body, a cached summary, the per-fn analysis quota, or in-flight
	// recursion — still CONSUMES the one-shot fnArm rather than stranding
	// it for the next, unrelated AnalyseFnBody (whose guard would then
	// mis-consume it and leak that body's events into the live fn
	// fragment). Mirrors how captureArm/loopArm are consumed at the top
	// of their analysis functions.
	defer r.Check.Recorder().FnBodyGuard()()
	if len(body) == 0 {
		return nil
	}
	// Record the caller→callee edge for the dynamic-scope undefined-word
	// rescue. The current top of FnNameStack is the fn whose body is executing
	// and just triggered this dispatch (empty at the top level). Recorded
	// BEFORE the memo / quota / in-flight early-returns below so a cached or
	// recursively-bailing call still contributes its edge — the self-edge of a
	// recursive fn in particular.
	if name != "" {
		if n := len(r.Check.FnNameStack); n > 0 {
			r.Check.RecordCallEdge(r.Check.FnNameStack[n-1], name)
		}
	}
	// A FORWARD-referenced fn name that isn't defined yet leaks into this
	// per-call-site analysis as a concrete Undefined Atom argument. Gradualize
	// it to a dynamic Any carrier at the analysis boundary — copy-on-write so
	// the caller's slice is untouched — and note the def-read so the
	// dynamic-scope undefined-word rescue still attributes it. Otherwise the
	// phantom concrete value drives dispatch and memo keying as if it were a
	// real Atom, a false no_signature on the forward-ref call. Mirrors the
	// dispatch-arg gradualization in engine.go's match loop.
	sanitized := false
	for i := range args {
		if !args[i].Undefined {
			continue
		}
		if !sanitized {
			args = append([]Value(nil), args...)
			sanitized = true
		}
		c := NewDynamicCarrier(TAny)
		if a, aerr := AsAtom(args[i]); aerr == nil && a != "" {
			r.Check.Recorder().NoteDefRead(c.ID, a)
		}
		args[i] = WithPos(c, args[i])
	}
	key := FnAnalysisKey(r.AnalysisScopeID(), name, args, captures, body)

	if r.Check.FnSummaries == nil {
		r.Check.FnSummaries = map[string][]Value{}
	}
	if r.Check.FnInflight == nil {
		r.Check.FnInflight = map[string]bool{}
	}
	if cached, ok := r.Check.FnSummaries[key]; ok {
		return cached
	}
	// Per-fn analysis quota (A9): a polymorphic helper reached with
	// many distinct arg shapes re-analyses once per shape; past the
	// quota, answer from the declaration (or dynamic Any) and say so
	// once, instead of silently consuming the global step budget. Keyed
	// by DEFINITION SITE (fnQuotaKey), not by bare name, so genuinely-
	// distinct closure bodies that share a synthetic "<word>$body" name
	// do not pool one budget across the whole program.
	if r.Check.FnAnalysisCounts == nil {
		r.Check.FnAnalysisCounts = map[string]int{}
	}
	quotaKey := fnQuotaKey(r.AnalysisScopeID(), name, body)
	displayName := name
	if displayName == "" {
		displayName = "<anon>"
	}
	r.Check.FnAnalysisCounts[quotaKey]++
	// The quota is a CHECK-mode accuracy/step-budget heuristic: past the quota
	// it fabricates a declared-return `declaredReturnBail` carrier that no event
	// produced. That carrier has no producedBy entry, so a real COMPILE pass —
	// which must lower the body's true residual to an operand — cannot resolve
	// it and refuses "fn <name>: body result of unknown provenance" (the trie
	// `match-go` / recursive-collector shape, pushed past the name-keyed count
	// by the re-entrant Test.test compile scopes). The compiler must see the
	// real body events, so the quota does not apply while Compiling; recursion
	// still terminates via the FnInflight cycle-breaker below, and runaway
	// non-recursive shape growth is bounded by the step budget.
	if n := r.Check.FnAnalysisCounts[quotaKey]; n > FnAnalysisQuota && !r.Check.Compiling {
		if n == FnAnalysisQuota+1 {
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code: "analysis_truncated",
				Detail: "fn " + displayName + " was analysed for more than " +
					strconv.Itoa(FnAnalysisQuota) + " distinct call shapes; later shapes are typed from the declaration (or dynamic Any) without body re-analysis",
				Word: name,
			})
		}
		return declaredReturnBail(declared)
	}
	if r.Check.FnInflight[key] {
		// Recursion detected. With declared returns, the declaration
		// is the induction hypothesis (assume-guarantee) — precise,
		// and the body's return check is the proof obligation. Without
		// it, break the cycle with an Any carrier and count the bail
		// so the enclosing analysis knows its result needs refinement.
		if len(declared) > 0 {
			return declaredReturnBail(declared)
		}
		r.Check.InflightBails++
		// Plain-check void-recursion (recursion.tsv:53): seed the self-call with
		// a variadic spread (element Never) so the enclosing if-join folds the
		// per-frame leaked lead (`n mul 2` → Integer) into it, yielding a sound
		// 0-or-more residual that covers the runtime's depth-many values. The
		// armed/compiled path keeps its own STAGE A variadic model (Any bail).
		if !r.Check.Recorder().Active() {
			return []Value{NewVariadicCarrier(NewTypeLiteral(TNever))}
		}
		return []Value{NewCarrier(TAny)}
	}
	r.Check.FnInflight[key] = true
	defer delete(r.Check.FnInflight, key)

	// Recursive RE-ENTRY by name (a self-call with a different arg shape, which
	// has a different FnInflight key so it did not bail above): suppress the
	// error-level body diagnostics this re-run would emit. The outer, canonical
	// analysis of the same body tokens already reports any real defect; a re-run
	// whose args narrowed a param to a strict Any can spuriously fail dispatch
	// (trie's fuzzy-go recursing on `child = pair get 1` as nd:Map → false
	// kid-items/get/build-row no_signature). Tracked per NAME so only genuine
	// same-fn recursion is suppressed, not nested helper calls.
	if name != "" {
		if r.Check.FnNameInflight == nil {
			r.Check.FnNameInflight = map[string]int{}
		}
		if r.Check.FnNameInflight[name] > 0 {
			r.Check.SuppressBodyErrors++
			defer func() { r.Check.SuppressBodyErrors-- }()
		}
		r.Check.FnNameInflight[name]++
		defer func() { r.Check.FnNameInflight[name]-- }()
	}

	// Diagnostics emitted from here down come from CALL-TIME code —
	// tag them FnBody so an undefined_word that turns out to be a
	// forward reference can be rescued at end of pass
	// (RescueForwardRefDiagnostics).
	r.Check.FnBodyDepth++
	defer func() { r.Check.FnBodyDepth-- }()
	// A fn body is a speculative region too (it runs only if called): an
	// `undef` of an enclosing binding inside it must not leak the deletion
	// into the pass model (SpecUndefBlocked — the wrapped-undef FP class;
	// frame teardown pops in-region bindings and stays untouched).
	r.Check.PushSpecBaseline(r.Defs.Snapshot())
	defer r.Check.PopSpecBaseline()

	// Push this fn onto the named-fn stack so its body-local defs and
	// undefined_word diagnostics attribute to it, and record its parameters as
	// binders of their names. Params are frame-lifetime, so a name a callee
	// reads that is one of this fn's params is a sound dynamic-scope reference
	// whenever this fn reaches that callee. Anonymous bodies are transparent
	// (not pushed).
	if name != "" {
		r.Check.FnNameStack = append(r.Check.FnNameStack, name)
		defer func() {
			r.Check.FnNameStack = r.Check.FnNameStack[:len(r.Check.FnNameStack)-1]
		}()
		for _, pn := range paramNames {
			r.Check.RecordFnBinder(pn)
		}
	}

	// runOnce performs one full body analysis: snapshot def-stack
	// depths so any defs the body, captures, or parameter bindings
	// created unwind afterwards. The same snapshot is pushed as the
	// fn-entry baseline so any inner fn/afn construction inside the
	// body sees this scope as its enclosing-fn baseline — without it,
	// ComputeCaptures would treat outer params as if they lived at
	// module/global scope and miss the capture.
	runOnce := func() []Value { return runFnBodyOnce(r, name, paramNames, body, args, captures, anonymous) }

	bailsBefore := r.Check.InflightBails
	diagBase := len(r.Check.Diagnostics)
	result := runOnce()

	// Refinement (A2): the run consumed an Any in-flight bail, so the
	// summary was computed under the weakest hypothesis. Seed the memo
	// with it and re-analyse — the recursive call now reads the seeded
	// hypothesis from the cache — joining each round's result into the
	// hypothesis until stable, up to two extra rounds. Only the final
	// round's diagnostics are kept.
	//
	// Skip refinement when ARMED (this body is being recorded into a compiled fn
	// unit): each extra runOnce re-records the body into the SAME open fragment,
	// leaving multiple residuals the lowerer cannot reconcile. Refinement only
	// sharpens the summary TYPE precision, which a no-contract (`[]`-declared)
	// recursive fn doesn't need — its recursive self-call already reads Any from
	// the in-flight bail, and its return is unconstrained. So a single clean
	// recording is both sound and sufficient. (A non-armed nested analysis is
	// suspended by fnBodyGuard, so active() is true only for the armed compile.)
	armed := r.Check.Recorder().Active()
	// A variadic-spread result already reached its fixpoint in this run (the
	// element came from the else-arm's fixed lead, invariant across rounds), so
	// skip the Kleene refinement — re-running would join variadic-vs-variadic.
	if r.Check.InflightBails > bailsBefore && len(result) > 0 && !armed && !stackHasVariadic(result) {
		result = refineRecursiveSummary(r, key, diagBase, result, runOnce)
	}

	result = stripZeroOutResiduals(r, result)
	r.Check.FnSummaries[key] = result
	return result
}

// isDeferredWordList reports whether v is a parser-evaluated (`Eval`, unquoted)
// plain list that still carries a raw Word element — the unfolded def-node-binding
// deferred residual a transparent fn body like `[[c1]]` returns.
func isDeferredWordList(v Value) bool {
	if !v.Eval || v.Quoted || !v.Parent.Equal(TList) || v.Data == nil ||
		IsTypedList(v) || IsTableType(v) {
		return false
	}
	lst, err := AsList(v)
	if err != nil {
		return false
	}
	for _, e := range lst.Slice() {
		if IsWord(e) {
			return true
		}
		if e.Parent.Equal(TList) && isDeferredWordList(e) {
			return true
		}
	}
	return false
}

// deferredParamListResidual reports whether a fn body is a single deferred list
// literal that references one of the fn's parameters — the def-node-binding.tsv:54
// shape `[[c1]]` with param `c1`. Such a body returns the list RAW; the param is
// never captured (only `=>` lambdas close over params), so its words auto-evaluate
// in MODULE scope at end-of-run, not the param scope AnalyseFnBody's sub-engine
// used. Returning the raw list lets the residual fold in module scope downstream
// (matching the interpreter) instead of refusing on a param-poisoned carrier.
//
// Deliberately narrow: only a lone, unquoted, parser-evaluated list body that
// names a parameter qualifies. A body that DOESN'T reference a param already
// folds correctly under the normal path; a multi-statement or computed body
// keeps its existing analysis (and its faithful fallback when it can't lower).
func deferredParamListResidual(body []Value, paramNames []string) (Value, bool) {
	if len(body) != 1 {
		return Value{}, false
	}
	v := body[0]
	if v.Quoted || !v.Eval || !v.Parent.Equal(TList) || v.Data == nil ||
		IsTypedList(v) || IsTableType(v) {
		return Value{}, false
	}
	params := make(map[string]bool, len(paramNames))
	for _, p := range paramNames {
		if p != "" {
			params[p] = true
		}
	}
	if len(params) == 0 {
		return Value{}, false
	}
	referencesParam := false
	WalkBodyWords([]Value{v}, func(w WordInfo, _ Value) {
		if params[w.Name] {
			referencesParam = true
		}
	})
	if !referencesParam {
		return Value{}, false
	}
	return v, true
}

// stripZeroOutResiduals drops a body residual's 0-output statement-guard
// phantoms — a trailing `if cond [stmt] []` / `if cond [raise]` registers a
// phantom None carrier but produces 0 RUNTIME values — so a 0-return fn whose
// body is such a guard returns the right count to a multi-value caller (`b i
// i`, where b's call must net 0 so the loop body nets just the trailing i).
// Mirrors the fn-UNIT residual reconciliation (StartFnCompile.finish, which
// skips the same zeroOut events) on the CALL side. Recording-active only: the
// zeroOut flag is set during the compile pass.
func stripZeroOutResiduals(r *Registry, stk []Value) []Value {
	es := r.Check.Recorder()
	if !es.Active() || len(stk) == 0 {
		return stk
	}
	filtered := make([]Value, 0, len(stk))
	for _, v := range stk {
		if es.ZeroOutProduced(v.ID) {
			continue
		}
		filtered = append(filtered, v)
	}
	return filtered
}

// carrierStacksEqual reports whether two carrier stacks agree
// position-for-position — the fixed-point stability test.
func carrierStacksEqual(a, b []Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !ValuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// resolveTypeNameArgs is the check-side twin of stepWord's builtin
// type-name fallback (ADR-012 rule 4): the forward plan claims a bare
// type-name token as a raw Word, and at RUNTIME stepWord steps it to
// the canonical literal before it arrives at the handler — so the
// checker's ReturnsFn must see the same literal, or a statically-typed
// dispatch (`convert String x`) degrades to a dynamic result. Only the
// builtin arm applies here: a Defs-bound word was already substituted
// by the claim walk, and /q-captured names arrive as Atoms, never
// Words. Copies lazily — the common all-resolved case allocates nothing.
func resolveTypeNameArgs(args []Value) []Value {
	var out []Value
	for i, a := range args {
		if !IsWord(a) {
			continue
		}
		w, err := AsWord(a)
		if err != nil {
			continue
		}
		t, ok := ResolveBuiltinTypeName(w.Name)
		if !ok {
			continue
		}
		if out == nil {
			out = append([]Value{}, args...)
		}
		lit := NewTypeLiteral(t)
		lit = WithPos(lit, a)
		out[i] = lit
	}
	if out == nil {
		return args
	}
	return out
}

func init() { core.JoinCarriersHook = JoinCarriers }
