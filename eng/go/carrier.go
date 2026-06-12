package eng

import "strings"

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

// NewCarrier constructs a carrier Value for the given type. Data is
// nil for scalar types. For TList and TMap, Data is set to a
// ChildTypeInfo wrapping an Any carrier so the carrier satisfies
// positionalMatch's "concrete list/map" rule (it rejects values
// whose Data==nil when the signature requires a concrete TList or
// TMap). Typed-list carriers (element type known) are produced via
// NewCarrierTypedList / NewCarrierTypedListValue.
func NewCarrier(t *Type) Value {
	v := NewValueRaw(t, nil)
	v.Carrier = true
	if t.Equal(TList) || t.Equal(TMap) {
		v.Data = ChildTypeInfo{Child: Value{Parent: TAny, Carrier: true}}
	}
	return v
}

// NewDynamicCarrier constructs a bounded gradual carrier dynamic(t):
// a carrier whose Parent is the BOUND t and whose Dynamic flag flips
// matching to the not-disjoint rule (design/dynamic-modality-report.10.md).
// dynamic(Any) is the classic gradual `any` — compatible with every
// slot. Use this at an escape hatch where the checker has a best static
// bound but cannot prove the exact type.
func NewDynamicCarrier(t *Type) Value {
	v := NewCarrier(t)
	v.Dynamic = true
	return v
}

// NewDynamicCarrierValue promotes an existing carrier value (e.g. a
// disjunct carrier for dynamic(A tor B), or a narrowed bound) to the
// dynamic modality, preserving its Parent/Data bound.
func NewDynamicCarrierValue(bound Value) Value {
	bound.Carrier = true
	bound.Dynamic = true
	return bound
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
		return []Value{NewCarrierTypedList(elem)}
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
		IsOpenParen(v) || IsParenExpr(v) || IsInterpString(v) ||
		IsReturnCheck(v) || IsDefCleanup(v) {
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
	// Keep concrete integer literals concrete so static index checking
	// can recover the value (an out-of-bounds literal index like
	// `[10 20] 5 getr`). Stripping would lose the value and force the
	// index check to give up. This only preserves genuine concrete
	// integers (IntPayload) — DepScalar constraints (Data is a
	// DepScalarInfo) and carriers are untouched. Precision only
	// increases: a literal stays concrete until a word consumes it and
	// produces a computed carrier, exactly as lists/maps already behave.
	if v.Parent.Equal(TInteger) && IsConcrete(v) && !v.IsDepScalar() {
		return v
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
	if i > 0 && isLiteralWord(in[i-1]) {
		return true
	}
	if i+1 < len(in) && isLiteralWord(in[i+1]) {
		return true
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
func carrierResults(r *Registry, word string, sig *Signature, args []Value, pos SrcPos) []Value {
	narrowDynamicUses(r, sig, args)
	// Per-alternative dispatch for strict disjunct inputs
	// (design/checker-accuracy-review.0.md A1). matchSignature tested
	// the disjunct as a single value, so the matched sig may not be
	// the one runtime dispatch takes for every alternative — e.g.
	// Integer|String reaches add's [Scalar Scalar]→String catch-all
	// although the Integer path takes [Number Number]. Resolve each
	// alternative independently and join the per-alternative returns.
	if out, ok := disjunctPartitionReturns(r, word, args, pos); ok {
		return out
	}
	var out []Value
	switch {
	case sig.ReturnsFn != nil:
		raw := sig.ReturnsFn(args, r)
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
	// Gradual contagion (design/dynamic-modality-report.10.md): a result
	// derived from a dynamic carrier is itself dynamic, so the modality
	// flows downstream instead of dying after one dispatch. The bound is
	// the sig's declared return (the first-cut result; the full
	// first-match partition over the bound is a later slice). Sound — it
	// only loosens matching, never tightens — and a guard discharges it
	// back to strict. ReturnsFn results that are already dynamic (e.g.
	// ReturnsIdentity of a dynamic input) stay so via toCarrier.
	if anyDynamicCarrier(args) {
		for i := range out {
			out[i].Carrier = true
			out[i].Dynamic = true
		}
		// First-match partition (design/dynamic-modality-report.10.md): a
		// dynamic bound can reach MULTIPLE of the word's overloads, whose
		// returns may differ. The single matched-sig return is then too
		// narrow — it would wrongly reject a downstream use of one of the
		// other reachable returns. Widen the (single) result to the union
		// of all reachable returns. No-op for the common case (one
		// reachable return), so unobservable with return-uniform words.
		if len(out) == 1 {
			if rets := dynamicReachableReturns(r, word, args); len(rets) >= 2 {
				alts := make([]Value, len(rets))
				for i, t := range rets {
					alts[i] = NewTypeLiteral(t)
				}
				out[0] = NewDynamicCarrierValue(NewDisjunct(alts))
			}
		}
	}
	return out
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
	for _, a := range args {
		if IsDisjunct(a) && a.Carrier && !a.Dynamic {
			hasStrictDisjunct = true
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

	// Cross product of alternatives, bounded.
	combos := [][]Value{nil}
	for i, a := range args {
		var alts []Value
		if IsDisjunct(a) && a.Carrier && !a.Dynamic {
			for _, lit := range flattenAlternatives(a) {
				alts = append(alts, carrierOfLiteral(lit))
			}
		} else {
			alts = []Value{a}
		}
		if len(combos)*len(alts) > disjunctPartitionCap {
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

	var joined []Value
	matchedAny := false
	for _, combo := range combos {
		comboSig := firstMatchingSig(fn, combo)
		if comboSig == nil {
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code: "partial_dispatch",
				Detail: word + " has no overload for alternative (" +
					comboTypeNames(combo) + ") of a disjunct input — that path would fail dispatch at runtime",
				Word:     word,
				Row:      pos.Row,
				Col:      pos.Col,
				Severity: SeverityWarning,
			})
			continue
		}
		var rets []Value
		if comboSig.ReturnsFn != nil {
			raw := comboSig.ReturnsFn(combo, r)
			rets = make([]Value, len(raw))
			for i, v := range raw {
				rets[i] = toCarrier(v)
			}
		} else if comboSig.Returns != nil {
			rets = make([]Value, len(comboSig.Returns))
			for i, t := range comboSig.Returns {
				rets[i] = NewCarrier(t)
			}
		} else {
			// Unannotated overload on one path: the whole-disjunct
			// fallback (missing_returns + dynamic Any) is the better
			// behaviour than a partial join.
			return nil, false
		}
		if !matchedAny {
			joined = rets
			matchedAny = true
			continue
		}
		if len(rets) != len(joined) {
			return nil, false
		}
		for i := range joined {
			joined[i] = JoinCarriers(joined[i], rets[i])
		}
	}
	if !matchedAny {
		return nil, false
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
		if len(s.Args) != len(args) {
			continue
		}
		ok := true
		for j := range args {
			if !sigTypeMatches(args[j], s.Args[j]) {
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
func dynamicReachableReturns(r *Registry, word string, args []Value) []*Type {
	fn := r.Lookup(word)
	if fn == nil || len(fn.Signatures) < 2 {
		return nil
	}
	var rets []*Type
	seen := map[string]bool{}
	for i := range fn.Signatures {
		s := &fn.Signatures[i]
		if len(s.Args) != len(args) || len(s.Returns) != 1 || s.Returns[0] == nil {
			return nil // a shape we don't refine — defer to contagion
		}
		reach := true
		for j := range args {
			if !sigTypeMatches(args[j], s.Args[j]) {
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
func narrowDynamicUses(r *Registry, sig *Signature, args []Value) {
	if r == nil || sig == nil || !r.Check.IsActive() {
		return
	}
	for i, a := range args {
		if !a.Dynamic || a.DynFrom == "" {
			continue
		}
		// Only narrow a binding that is itself still dynamic (consistent
		// with this value) — guards against a since-rebound name.
		cur, ok := r.Defs.Top(a.DynFrom)
		if !ok || !cur.Dynamic {
			continue
		}
		slot := sigArgType(sig, i)
		if slot == nil {
			continue
		}
		bound := cur
		bound.Dynamic, bound.DynFrom = false, ""
		narrowed := TandValues(bound, NewCarrier(slot))
		// A successful match guarantees a non-disjoint intersection; skip
		// when the bound did not actually tighten (no-op / avoids
		// unbounded layer growth on repeated same-type uses).
		if isNeverShape(narrowed) || ValuesEqual(bound, narrowed) {
			continue
		}
		r.Defs.Push(a.DynFrom, NewDynamicCarrierValue(narrowed))
	}
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

// ReturnsIdentity is a ReturnsFunc helper that returns its inputs
// unchanged (as carriers). Use for stack operations that preserve
// their inputs — dup, swap, over, rot, etc. — where the output types
// are directly expressible in terms of the input types.
//
// The mapping is a permutation-description slice: result[i] = args[mapping[i]].
// Example: swap is ReturnsIdentity(1, 0); over is ReturnsIdentity(0, 1, 0).
func ReturnsIdentity(mapping ...int) ReturnsFunc {
	return func(args []Value, _ *Registry) []Value {
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

// carrierOfLiteral converts a bare type-literal value (the node
// itself, as stored in DisjunctInfo.Alternatives) into a carrier OF
// that node — i.e. Parent points at the literal's type, not at the
// literal's lattice parent.
func carrierOfLiteral(lit Value) Value {
	lt := lit
	return NewCarrier(&lt)
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
func JoinCarriers(a, b Value) Value {
	if a.Parent.Equal(b.Parent) && !IsDisjunct(a) && !IsDisjunct(b) {
		out := a
		out.Carrier = true
		out.Data = nil
		return out
	}
	if !IsDisjunct(a) && !IsDisjunct(b) {
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
		return carrierOfLiteral(alts[0])
	}
	if len(alts) > CarrierDisjunctCap {
		t := typeNodeOf(alts[0])
		for i := 1; i < len(alts); i++ {
			t = CommonAncestorType(t, typeNodeOf(alts[i]))
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
	if body.Data == nil {
		return nil, nil
	}
	elems, err := AsList(body)
	if err != nil || elems.IsNil() {
		return nil, nil
	}

	// Snapshot def-stack depths (all known names).
	snapshot := r.Defs.Snapshot()

	tokens := make([]Value, elems.Len())
	copy(tokens, elems.Slice())
	sub := New(r)
	result, err := sub.Run(tokens)
	if err != nil {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "branch_error",
			Detail: "branch analysis error: " + err.Error(),
		})
		result = nil
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

// InstallJoinedDefs merges the `adds` maps from two branches back
// into r.DefStacks. If both branches defined the same name, their
// carriers are joined via JoinCarriers and the joined carrier is
// pushed. If only one branch defined it, that def is pushed back —
// but joined with the pre-branch carrier (if any) since the other
// branch's path kept the original binding.
func InstallJoinedDefs(r *Registry, then, else_ map[string]Value) {
	seen := make(map[string]bool)
	for k, tv := range then {
		seen[k] = true
		if ev, ok := else_[k]; ok {
			r.Defs.Push(k, JoinCarriers(tv, ev))
			continue
		}
		// then-only: join with the pre-branch top-of-stack if any.
		if pre, ok := r.Defs.Top(k); ok {
			r.Defs.Push(k, JoinCarriers(tv, pre))
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
			r.Defs.Push(k, JoinCarriers(ev, pre))
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

// GuardClause describes one `x is T` clause detected in a condition.
type GuardClause struct {
	Name string
	Type *Type
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
	list, err := AsList(condList)
	if err != nil || list.IsNil() || list.Len() < 3 {
		return nil
	}
	elems := list.Slice()
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
		if tv.Data != nil && tv.Parent.Equal(TWord) {
			inner, _ := AsWord(tv)
			if v, ok := r.Defs.Top(inner.Name); ok {
				tv = v
			}
		}
		if tv.Data != nil && !IsObjectType(tv) {
			continue
		}
		// A bare type-literal clause IS its type; an ObjectType keeps
		// its type at Parent (the minted object-type node).
		guardType := tv.Parent
		if tv.Data == nil {
			gt := tv
			guardType = &gt
		}
		out = append(out, GuardClause{Name: wx.Name, Type: guardType})
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
	for _, c := range clauses {
		r.Defs.Push(c.Name, NewCarrier(c.Type))
	}
	return func() {
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
	if len(clauses) == 0 {
		return noop
	}
	type applied struct{ name string }
	var pushed []applied
	for _, c := range clauses {
		cur, ok := r.Defs.Top(c.Name)
		if !ok {
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
		if isNeverShape(narrowed) {
			// Else branch is unreachable for x — leave the binding as-is
			// rather than push a Never carrier that fails every later use.
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
			// Complement did not refine cur (T disjoint from cur, or AQL
			// has no positive representation for the exact difference).
			continue
		}
		r.Defs.Push(c.Name, narrowed)
		pushed = append(pushed, applied{name: c.Name})
	}
	if len(pushed) == 0 {
		return noop
	}
	return func() {
		for _, p := range pushed {
			r.Defs.Pop(p.name)
		}
	}
}

// AnalyseFnBody runs a user-defined fn body through a sub-engine in
// check mode, treating named parameters as deffed values bound to
// their arg carriers and unnamed parameters as pre-pushed stack
// values. Results are cached on the registry keyed by (name,
// arg-types) so recursive functions converge instead of looping.
//
// Returns the residual carrier stack. An empty or nil return means
// the analyser aborted (recursion detected or body not available) —
// callers should treat that as an Any carrier.
func AnalyseFnBody(r *Registry, name string, paramNames []string, body []Value, args []Value, captures []CapturedBinding) []Value {
	if len(body) == 0 {
		return nil
	}
	// Memoisation key: name + arg type paths + captured-name set.
	// The captures are included so two anonymous lambdas with
	// identical bodies but different capture sets don't collide.
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('#')
	for _, a := range args {
		sb.WriteString(a.Parent.String())
		sb.WriteByte(',')
	}
	if len(captures) > 0 {
		sb.WriteByte('|')
		for _, cb := range captures {
			sb.WriteString(cb.Name)
			sb.WriteByte(':')
			sb.WriteString(cb.Value.Parent.String())
			sb.WriteByte(',')
		}
	}
	key := sb.String()

	if r.Check.FnSummaries == nil {
		r.Check.FnSummaries = map[string][]Value{}
	}
	if r.Check.FnInflight == nil {
		r.Check.FnInflight = map[string]bool{}
	}
	if cached, ok := r.Check.FnSummaries[key]; ok {
		return cached
	}
	if r.Check.FnInflight[key] {
		// Recursion detected — break the cycle with an Any carrier.
		return []Value{NewCarrier(TAny)}
	}
	r.Check.FnInflight[key] = true
	defer delete(r.Check.FnInflight, key)

	// Snapshot def-stack depths so we can unwind any defs the body,
	// captures, or parameter bindings created. The same snapshot is
	// pushed as the fn-entry baseline so any inner fn/afn construction
	// inside the body sees this scope as its enclosing-fn baseline —
	// without it, ComputeCaptures would treat outer params as if they
	// lived at module/global scope and miss the capture.
	snapshot := r.Defs.Snapshot()
	r.PushFnBaseline(snapshot)
	defer r.PopFnBaseline()

	// Install lexical captures first so params (installed below)
	// shadow same-named captures — innermost binding wins, matching
	// runtime dispatch.
	for _, cb := range captures {
		r.Defs.Push(cb.Name, cb.Value)
	}

	// Bind named parameters as simple defs (carrier-typed). Unnamed
	// parameters flow through the stack — push them before the body.
	var input []Value
	for i, arg := range args {
		if i < len(paramNames) && paramNames[i] != "" {
			r.Defs.Push(paramNames[i], arg)
		} else {
			input = append(input, arg)
		}
	}
	input = append(input, body...)

	sub := New(r)
	result, err := sub.Run(input)
	if err != nil {
		r.Check.AddDiagnostic(CheckDiagnostic{
			Code:   "fn_body_error",
			Detail: "fn body analysis error for " + name + ": " + err.Error(),
			Word:   name,
		})
		result = nil
	}

	// Restore def-stacks to snapshot.
	r.Defs.Restore(snapshot)

	r.Check.FnSummaries[key] = result
	return result
}
