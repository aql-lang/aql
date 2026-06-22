package eng

import (
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
	// Keep MODULE instances (Ideal/Module, Ideal/ModuleExport) concrete, same
	// rationale as FnDefInfo: stripping nulls the ExtensionPayload descriptor /
	// exports, so `MathUtil.$module` would become an opaque carrier the
	// get-resolution elision can no longer follow. They are immutable and
	// import-bound, so a pure read of one (`$name`, `$module.name`, `convert
	// Map …`) const-folds (tryFoldModuleConst).
	if isModuleFamilyValue(v) {
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
func carrierResults(r *Registry, word string, sig *Signature, args []Value, pos SrcPos, ownerReg *Registry) []Value {
	// `args` inside a compiled fn body projects the frame's params (pushed by
	// AnalyseFnBody) as a list value with NO recorded event. An `args.N` access
	// then folds to param N — a frame local — via tryFoldStaticIndex; bare
	// `args` has no foldable consumer and refuses at its use site. At top level
	// r.Args is empty so `args` falls through to RecordCall's refusal.
	if word == "args" {
		if top, ok, err := r.Args.Top(); err == nil && ok && IsConcrete(top) {
			return []Value{top}
		}
	}
	// `word [body]` is a compile-time macro splice: produce the __SP marker as a
	// non-emitting value (no runtime op). At its use site stepLiteral splices the
	// body inline and re-steps it against the live stack, so the expansion
	// compiles in place — late binding and all. (The `def NAME word …` that binds
	// the marker emits nothing either; the marker has no runtime existence.)
	if word == "word" && len(args) == 1 {
		return []Value{NewSplice(args[0])}
	}
	// `macroexpand (mac args…)` is Lisp-style compile-time expansion: the macro
	// and its operands are static, so run the expansion NOW and bake the
	// resulting token list as a const (code-as-data). Only when the expansion
	// is fully concrete (isInertConst — no carrier from a runtime operand) and
	// succeeds; a too-deep / erroring expansion falls through to refuse, and the
	// interpreter surfaces the same error.
	if word == "macroexpand" && len(args) == 1 {
		if toks, err := ExpandMacroForm(r, args[0]); err == nil {
			if lst := NewList(toks); isInertConst(lst) {
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
				if err2 == nil && constFoldAgrees(NewList(toks2), lst) {
					return []Value{lst}
				}
			}
		}
	}
	narrowDynamicUses(r, sig, args)
	// Per-alternative dispatch for strict disjunct inputs
	// (design/checker-accuracy-review.10.md A1). matchSignature tested
	// the disjunct as a single value, so the matched sig may not be
	// the one runtime dispatch takes for every alternative — e.g.
	// Integer|String reaches add's [Scalar Scalar]→String catch-all
	// although the Integer path takes [Number Number]. Resolve each
	// alternative independently and join the per-alternative returns.
	if out, ok := disjunctPartitionReturns(r, word, args, pos); ok {
		// A strict-disjunct straddle is a runtime-dispatch case, not an
		// inherent refusal: if the word is a safe poly candidate (core builtin,
		// no meta/fn-value/code-body sig) and its operands resolve, lower it to
		// OpCallNativePoly so the VM re-matches the one concrete alternative at
		// run time — e.g. `5 is (tnot (Integer gt 0))`. Otherwise refuse.
		if !tryRecordPoly(r, word, sig, args, out, pos, true, ownerReg, false) {
			r.Check.Emit.RecordPoly(word)
		}
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
	if !tryFoldStaticIndex(r, word, args, out) &&
		!tryFoldModuleConst(r, word, sig, args, out) &&
		!tryRecordClosure(r, word, sig, args, out, pos) &&
		!tryRecordPoly(r, word, sig, args, out, pos, false, ownerReg, false) &&
		!tryRecordFallback(r, word, sig, args, out, pos) {
		quoteInertOK := quoteOperandInertOK(r, word, sig, args)
		r.Check.Emit.RecordCall(word, sig, args, out, pos,
			dynOutNativeOK(r, word, sig, args, out) || quoteInertOK, quoteInertOK)
	}
	return out
}

// tryFoldStaticIndex folds a `get` / `getr` over a CONCRETE list with a STATIC,
// in-range, non-negative integer index to the element's existing operand —
// emitting nothing, since the result already has a compiled home. Its purpose is
// `args.N` (= `get N args`): the args projection is the list of param carriers,
// whose IDs are the frame locals, so `args.0` folds to PUSH_LOCAL 0. The fold is
// general but self-gating: it only fires when the element resolves to an operand
// (a local or interned const), so a literal-list element that was never interned
// declines and the normal poly/get path stands. outs[0] is rewritten to the
// element carrier so the value flowing on has the element's identity.
func tryFoldStaticIndex(r *Registry, word string, args, outs []Value) bool {
	es := r.Check.Emit
	if !es.active() || (word != "get" && word != "getr") || len(args) != 2 || len(outs) != 1 {
		return false
	}
	key, recv := args[0], args[1]
	if !recv.Parent.ConformsTo(TList) || !IsConcrete(recv) ||
		!key.Parent.ConformsTo(TInteger) || !IsConcrete(key) {
		return false
	}
	n, err := AsInteger(key)
	if err != nil || n < 0 {
		return false
	}
	lst, lerr := AsList(recv)
	if lerr != nil || lst.IsNil() || int(n) >= lst.Len() {
		return false
	}
	elem := lst.Get(int(n))
	if _, ok := es.resolveOperand(elem); !ok {
		return false // element has no compiled home (e.g. an un-interned literal) — decline
	}
	outs[0] = elem
	return true
}

// The PURE reader words whose result over a compile-time-known module value is a
// compile-time constant (get / getr / convert / typeof / is / size / has) now
// DECLARE CompileModuleFold on their NativeFunc (lang layer). `import` binds an
// immutable, deterministic ModuleExport / Module instance, so a read over it
// always yields the same value — baked rather than re-read at run time. See
// tryFoldModuleConst.

// isModuleFamilyValue reports whether v is a concrete module instance — an
// Ideal/Module descriptor or an Ideal/ModuleExport namespace (the values
// `import` binds). Identified by the stable registered type PATH (FixedIDs
// 5000/5001 in the lang layer) so the eng-level fold needs no lang import. These
// instances are immutable and produced deterministically by `import`, so a pure
// read of one is a compile-time constant.
func isModuleFamilyValue(v Value) bool {
	if !IsConcrete(v) || v.Parent == nil {
		return false
	}
	switch v.Parent.Path() {
	case "Ideal/Module", "Ideal/ModuleExport":
		return true
	}
	return false
}

// constFoldAgrees reports whether two const-fold probe evaluations produced
// the SAME bakeable value, compared by CanonValue — the exact structural key
// the const interner dedups by (emit.go's constIdx). It is the shared
// determinism gate behind every twice-and-compare const-bake
// (tryFoldModuleConst, the macroexpand splice in carrierResults, and the
// engine's constFoldContainerVal): a clock / rand / mutation-bearing read
// whose two probes drift renders a different canon and is refused, so no
// nondeterministic value is ever frozen into the program.
//
// CanonValue, NOT String(): String() is a DISPLAY rendering that conflates
// values which bake DIFFERENTLY — a bare type node vs the string of its name
// (`Integer` vs 'Integer'), an atom vs a same-spelled string (name/q vs
// 'name'), an Integer vs an equal-magnitude Float, a fn vs a same-shaped fn
// with a different body — so two genuinely divergent probes could
// String()-match and freeze an UNSOUND const. CanonValue is the bake
// identity itself ("same canon" ⟺ "interns to one const"), and it is no
// coarser than String() on any bakeable shape, so a legitimately
// deterministic fold still agrees (no coverage change) while the
// conflations String() hid can no longer slip a frozen value through.
func constFoldAgrees(a, b Value) bool { return CanonValue(a) == CanonValue(b) }

// tryFoldModuleConst const-folds a PURE read whose result is a compile-time
// constant because it depends only on a module value (immutable, import-bound)
// plus inert consts / type operands — `MathUtil.$name` -> 'MathUtil',
// `convert Map Foo` -> the export map, `MathUtil.$module.name` ->
// 'aql:math-util', `typeof MathUtil.$module` -> Module. The checker's recorded
// RESULT is NOT enough: a word like `convert`/`is`/`typeof` returns its declared
// TYPE (a Map carrier, a Boolean carrier) in check mode, not the concrete value,
// so baking that would render `Map` where the interpreter rebuilds `{a:1 b:2}`.
// Instead the dispatch is RE-EVALUATED concretely (check mode off) — twice, and
// only folded when both runs agree on the same resolvable value (an inert const
// or a bare type node), so a clock/rand/mutation-bearing read never freezes.
// The fold emits nothing; the concrete result rides as that const / type
// operand — the get/getr module-RESOLUTION elision (RecordCall), generalised to
// the synthetic accessors and projections whose result is data, not a fn.
// Declines unless the word is a known pure reader with a direct handler, at
// least one operand is a module value, and every other operand is itself a
// compile-time constant (an inert const or a type node) — a runtime operand
// never folds.
func tryFoldModuleConst(r *Registry, word string, sig *Signature, args, outs []Value) bool {
	es := r.Check.Emit
	if !es.active() || sig == nil || !sig.CompileEffect.Has(CompileModuleFold) || len(outs) != 1 ||
		sig.Handler == nil || len(sig.NoEvalArgs) > 0 {
		return false
	}
	sawModule := false
	for _, a := range args {
		switch {
		case isModuleFamilyValue(a):
			sawModule = true
		case IsBareTypeNode(a):
			// a type operand (the target of `convert Map …` / `… is Module`)
		case isInertConst(a):
			// an inert const operand (a quoted key atom, a scalar)
		default:
			return false // a runtime / non-const operand — not a compile-time fold
		}
	}
	if !sawModule {
		return false
	}
	one, ok := concreteHandlerEval(r, sig, args)
	if !ok {
		return false
	}
	two, ok := concreteHandlerEval(r, sig, args)
	if !ok || !constFoldAgrees(one, two) {
		return false
	}
	// A `get` that resolves to None is a MISSING key — but a module's keyspace
	// can GROW at runtime: minilang/parselang `register` installs new exports
	// (`parse_<name>`) AFTER the check pass folded the program. Folding the
	// missing-key get to None bakes a stale absence: the compiled program then
	// pushes None + leaves the call's args on the stack instead of dispatching
	// the registered word, diverging from the interpreter (which sees the
	// registered key). Decline the fold so the get stays dynamic and the
	// program falls back / islands faithfully. A PRESENT key (any non-None
	// value) folds as before; this only blocks the absent-key case.
	if word == "get" && IsNoneShape(one) {
		return false
	}
	switch {
	case isInertConst(one):
		outs[0] = one // ride as an inert const
	case IsBareTypeNode(one) && one.ID != "":
		outs[0] = one // ride as a type operand (OpPushType)
	default:
		return false
	}
	return true
}

// concreteHandlerEval runs sig.Handler on the already-resolved args with check
// mode OFF, so a pure reader produces its REAL value rather than the declared-
// type carrier the check-mode ReturnsFn emits. Nothing is recorded (the handler
// is called directly, off the emit path) and the def stack is snapshotted /
// restored so a stray binding cannot leak. Returns the single result when it is
// a concrete value or a bare type node (typeof's type literal). Mirrors
// concreteEvalOnce, but dispatches the one matched native instead of re-running
// a token stream — the args are already in sig order.
func concreteHandlerEval(r *Registry, sig *Signature, args []Value) (Value, bool) {
	snap := r.Defs.Snapshot()
	prev := r.Check.Mode
	r.Check.Mode = false
	res, err := sig.Handler(args, nil, nil, r)
	r.Check.Mode = prev
	r.Defs.Restore(snap)
	if err != nil || len(res) != 1 {
		return Value{}, false
	}
	if IsConcrete(res[0]) || (IsBareTypeNode(res[0]) && res[0].ID != "") {
		return res[0], true
	}
	return Value{}, false
}

// dynOutNativeOK reports whether a dispatch with a DYNAMIC output but CONCRETE
// args may still bake a plain CALL_NATIVE despite the dynamic result. Concrete
// args mean the checker RESOLVED the sig by real matching (not widening), so a
// dynamic output is just a declared-Any return (e.g. unify's [Any, Boolean]),
// not a best-guess sig — for a CORE builtin native the handler runs faithfully.
// The dynamic result is still registered, so any downstream TYPED consumer of
// it refuses via the dynamic-input guard, keeping it contained. Mirrors
// tryRecordPoly's safety (core sig, no meta/fn-value), and is the escape hatch
// RecordCall's anyDynamicCarrier(outs) refusal consults via forceDynOut.
func dynOutNativeOK(r *Registry, word string, sig *Signature, args, outs []Value) bool {
	es := r.Check.Emit
	if !es.active() || sig == nil || len(outs) == 0 {
		return false
	}
	// Concrete args + dynamic output only — a dynamic INPUT means the sig was
	// widened (a guess), which stays refused.
	if anyDynamicCarrier(args) || !anyDynamicCarrier(outs) {
		return false
	}
	if sig.CompileEffect.Has(CompileFallbackBody) {
		return false
	}
	// Meta / re-stepping shapes never bake (RecordCall refuses them regardless;
	// screen here so they don't slip through forceDynOut).
	if sig.FnFrame != nil || sig.FullStack || sig.RunInCheckMode || len(sig.QuoteArgs) > 0 {
		return false
	}
	// A code-body (NoEvalArgs) native bakes only when its bodies are INERT consts
	// with no enclosing-loop sentinel — the SAME screen RecordCall's code-body
	// refusal uses (noEvalBodiesInert), not a blanket NoEvalArgs exclusion. An
	// inert body bakes as a code-as-data const and the handler sub-runs it
	// faithfully (`await` runs its parallels; a body-running native runs its
	// body), so the dynamic (declared-Any) result is sound to bake. A non-inert
	// or sentinel-bearing body stays refused.
	if len(sig.NoEvalArgs) > 0 && !noEvalBodiesInert(sig, args) {
		return false
	}
	for _, t := range sig.Args {
		if t != nil && (t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) {
			return false
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			return false
		}
	}
	// The VM bakes this exact sig and calls sig.Handler DIRECTLY, so it must be
	// a REAL native binding: the word's own main-registry BUILTIN sig, OR a
	// trivial-delegation module inner-native sig reached via dot-access
	// (`StructUtil.clone …`). Both are sound to bake with the main registry —
	// for a module inner native the interpreter ALSO dispatches via execMatch
	// on the main engine (the wrapper's trivial delegation), so the call is
	// identical. The IsBuiltinWord gate on the core path is load-bearing: a
	// user `def ifu (usurp if)` makes r.Lookup("ifu") return the usurp-MODIFIED
	// if sig (pointer-equal to the match) — but ifu is not a builtin, so it is
	// excluded here and stays refused (a usurp'd if re-steps and returns
	// tape-coupled values). A usurp synthetic also matches no module export.
	if r.IsBuiltinWord(word) {
		if fn := r.Lookup(word); fn != nil {
			for i := range fn.Signatures {
				if &fn.Signatures[i] == sig {
					return true
				}
			}
		}
	}
	return isModuleInnerSig(r, word, sig)
}

// quoteOperandInertOK reports whether a dispatch with implicit-quote operands
// (sig.QuoteArgs) may still bake a plain CALL_NATIVE despite the quoted
// positions. It holds only for a MODULE INNER NATIVE (`Pkg.word`, confirmed by
// pointer identity) whose every quoted operand is an inert Atom const — the
// query DSL's table-name operands (`Query.from people`, `Query.join visits`):
// the name is captured unevaluated as a symbol the handler resolves at run time.
// This is the QuoteArgs analogue of dynOutNativeOK and of the get/getr/set
// exemption in RecordCall: the inner native is reached via the wrapper's trivial
// delegation, so the interpreter dispatches the SAME handler via execMatch on
// this engine, and a baked Atom is the same value either way. Restricting to
// module inner natives keeps it off the core meta words (usurp / force-arity /
// ref-family) whose quoted operands drive re-stepping dispatch, and off any
// user word (those refuse earlier as a user-fn call). Mutation-safety holds:
// the query builders return fresh lazy-query values, they do not mutate a
// pooled const.
func quoteOperandInertOK(r *Registry, word string, sig *Signature, args []Value) bool {
	if sig == nil || len(sig.QuoteArgs) == 0 {
		return false
	}
	// Meta / re-stepping / code-body shapes never bake (RecordCall refuses them
	// regardless; screen here so they cannot slip through this exemption).
	if sig.FnFrame != nil || sig.FullStack || sig.RunInCheckMode || len(sig.NoEvalArgs) > 0 {
		return false
	}
	// A word that DECLARES CompileQuoteInert (quote / codequote / raise / timeout
	// / interval): its quoted operand is inert data the handler consumes verbatim
	// — a quoted symbol, or a quoted code body held as data — so it bakes as a
	// plain CALL_NATIVE once every quoted operand is an inert const (an Atom, or a
	// quoted code list). The VM runs the same handler over the same baked value.
	// Unlike the module-inner branch below, this admits a non-Atom inert operand
	// (a `[body]` list) and a core builtin, but a non-inert quoted operand still
	// declines so the program falls back.
	if sig.CompileEffect.Has(CompileQuoteInert) {
		for i := range args {
			if sig.QuoteArgs[i] && !isInertConst(args[i]) {
				return false
			}
		}
		return true
	}
	for i := range args {
		if !sig.QuoteArgs[i] {
			continue
		}
		if _, ok := args[i].Data.(AtomPayload); !ok || !isInertConst(args[i]) {
			return false
		}
	}
	return isModuleInnerSig(r, word, sig)
}

// isModuleInnerSig reports whether sig is a native signature exported by a
// LOADED module's trivial-delegation wrapper for word — i.e. the inner native
// `word` dispatches when called as `Pkg.word`. The wrapper (an FnDefInfo
// carrying its sub-Registry) lives in the module's export map; the inner sig is
// `wrapper.Registry.Lookup(word).Signatures`. Pointer-identity confirms sig is
// THAT native, not a usurp-synthetic copy. O(loaded modules × exports) — only
// consulted on a dynamic-output dispatch the core-sig check already missed.
func isModuleInnerSig(r *Registry, word string, sig *Signature) bool {
	if r == nil || r.Modules == nil {
		return false
	}
	for _, md := range r.Modules.loaded {
		for _, em := range md.Exports {
			if em == nil {
				continue
			}
			for _, k := range em.Keys() {
				v, _ := em.Get(k)
				fd, ok := v.Data.(FnDefInfo)
				if !ok || fd.Registry == nil || fd.Name != word {
					continue
				}
				inner := fd.Registry.Lookup(fd.Name)
				if inner == nil {
					continue
				}
				for i := range inner.Signatures {
					if &inner.Signatures[i] == sig {
						return true
					}
				}
			}
		}
	}
	return false
}

// tryRecordPoly records a genuinely-dynamic typed dispatch (get/size/is/
// make/typeof/type-algebra over an Any-widened operand) as an
// OpCallNativePoly that re-matches the word's signatures at run time — the
// SAME first-match the interpreter takes — instead of islanding through a
// sub-engine (plan P3). Returns false (leaving the island path) for a
// concrete-operand call, a non-core sig, or an operand of unknown provenance.
//
// disjunctStraddle marks the OTHER legitimate poly trigger: a STRICT (non-
// dynamic) disjunct operand that straddles the word's signatures
// (disjunctPartitionReturns) — e.g. `5 is (tnot (Integer gt 0))`, where the
// complement type reaches more than one `is` overload. The runtime value is a
// single concrete alternative, so the same runtime MatchSignature dispatches it
// faithfully; only the dynamic-only gate is bypassed, every other safety gate
// (core builtin, no meta/fn-value/code-body sig, sig identity, resolvable
// operands) still applies.
func tryRecordPoly(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos, disjunctStraddle bool, ownerReg *Registry, dynamicRecovery bool) bool {
	es := r.Check.Emit
	if !es.active() || sig == nil || len(outs) != 1 {
		return false
	}
	// matchReg is the registry whose signatures the VM re-matches over: a module
	// sub-registry for a delegation-dispatched module word (`StructUtil.getpath`),
	// else the main registry. The recorder validates the matched sig against it
	// below, and the PolyRef carries it so callPoly looks up the right word.
	matchReg := r
	if ownerReg != nil {
		matchReg = ownerReg
	}
	// Code-body higher-order words compile to closures, not poly.
	if sig.CompileEffect.Has(CompileFallbackBody) {
		return false
	}
	// Only a REGISTERED builtin native — never a user-def fn or a usurp/ref
	// wrapper (`def ifu (usurp if)`), whose dispatch re-steps tokens and
	// returns tape-coupled values the VM cannot push. The runtime
	// MatchSignature re-dispatches over the builtin's own signatures. For a
	// module word the builtin lives in its OWN sub-registry (matchReg).
	if !matchReg.IsBuiltinWord(word) {
		return false
	}
	// Only a genuinely dynamic dispatch (the case the checker could not
	// commit to one overload — an island or a refusal today), a strict-
	// disjunct straddle (disjunctStraddle), or a no-signature recovery over an
	// Any-typed operand (dynamicRecovery — matchSignature found no overload
	// because an operand's type is statically unknown, e.g. a List/Map element).
	// A fully concrete, single-overload call lowers to a faithful baked
	// CALL_NATIVE, not poly.
	if !disjunctStraddle && !dynamicRecovery && !anyDynamicCarrier(args) && !anyDynamicCarrier(outs) {
		return false
	}
	// Shapes the VM re-match cannot faithfully dispatch: code bodies,
	// quoted/meta operands, user-fn frames, full-stack words, compile-time
	// words. (a CompileIslandPure get passes these — get's key is its only
	// QuoteArg and is handled below.)
	if sig.FnFrame != nil || sig.FullStack || sig.RunInCheckMode || len(sig.NoEvalArgs) > 0 {
		return false
	}
	if len(sig.QuoteArgs) > 0 && word != "get" && word != "getr" {
		return false
	}
	// A fn-valued operand or result means a fn-invoking / fn-returning word
	// (apply/usurp, an atom-keyed method get): the value would need dynamic
	// INVOCATION (the fn-value-call boundary, P4). Keep those out of poly.
	for _, t := range sig.Args {
		if t != nil && (t.ConformsTo(TFunction) || t.ConformsTo(TFnDef)) {
			return false
		}
	}
	for _, a := range args {
		if _, ok := a.Data.(FnDefInfo); ok {
			return false
		}
	}
	// get/getr over a Map/Object/Module receiver can return a Function FIELD
	// (a method). The VM handles that faithfully now: callPoly auto-applies a
	// named 0-arg method result (`r.bool`), and a method needing args
	// (`r.int`) stays a value and flows to CALL_DYNAMIC — so both atom- and
	// integer-keyed gets poly.
	// CORE-dispatch guard: the matched sig must be the word's binding IN THE
	// REGISTRY the VM will re-match over (matchReg), since callPoly re-matches
	// over matchReg.Lookup's signatures — a sig that is not that registry's
	// binding would re-run a different word of the same name.
	fn := matchReg.Lookup(word)
	if fn == nil {
		return false
	}
	sigOK := false
	for i := range fn.Signatures {
		if &fn.Signatures[i] == sig {
			sigOK = true
			break
		}
	}
	if !sigOK {
		return false
	}
	return es.RecordPolyCall(word, args, outs, pos, ownerReg)
}

// The code-body higher-order words that may compile as Stage-5 interpreter
// islands (each / fold / scan / filter / select / group / outer / inner / do /
// case / where / having / order — pure data transforms applying a code body to
// data, no registry mutation) DECLARE CompileFallbackBody on their NativeFunc.
//
// The F4 general dynamic-dispatch words — pure typed dispatches (get / getr /
// size / make / is / typeof / type-algebra) with no side effects, whose
// forward-form span re-DISPATCHES faithfully through a sub-engine when the
// checker widened the site to a dynamic carrier — DECLARE CompileIslandPure. The
// sub-engine picks the overload at run time exactly as the interpreter would, so
// soundness holds without a static sig commitment; the dynamic result flows on
// and a downstream TYPED dispatch still refuses via anyDynamicCarrier. (Report
// §9.1's TYPE_CHECK boundary, realised as an interpreter island.)

// tryRecordFallback attempts to compile a refused code-body higher-order
// word as an interpreter island: the construct re-runs through a
// sub-engine over `word arg0 arg1 …` in forward form. The baked args
// ride inside the island token span; a COMPUTED data arg (a prior
// compiled event's result, or a loop local) whose value the check pass
// can't materialise is THREADED instead — the VM preloads its runtime
// value onto the island and the span re-runs against it ("computed
// receiver" islands, e.g. `(iota 5) each […]`).
//
// Eligible iff the word is allow-listed, fully forward-eligible (so the
// forward-form span is faithful), single-result, every code-body arg is
// concrete AND free of references the VM can't honour (only registered
// words / known literals — a check-time `def` binding is a carrier at
// run time and would diverge), and at most ONE data arg is threaded and
// it is the TRAILING run of positions (so the baked args fill the
// forward prefix and the one threaded value back-fills the deepest sig
// position — positionally faithful by the split rule). A baked data arg
// must be deeply concrete. Returns true when recorded; false leaves the
// normal refusal (whole-program fallback) to stand. Soundness rides on
// the differential gate: a threaded value is the program's real runtime
// value, and the island's dynamic result still refuses any downstream
// TYPED dispatch via anyDynamicCarrier.
func tryRecordFallback(r *Registry, word string, sig *Signature, args, outs []Value, pos SrcPos) bool {
	es := r.Check.Emit
	if !es.active() || sig == nil || !sig.CompileEffect.Has(CompileFallbackBody|CompileIslandPure) || len(outs) != 1 {
		return false
	}
	// A higher-order callable word dispatched on its LENS (Reach) form, not a
	// code body (`filter $.on data`, `each $.name data`): the island mechanism
	// exists to run an interpreted CODE BODY, and a reach lens is inert data, not
	// code. Now that an inert lens bakes as a const (isInertReach), letting these
	// island would convert a clean refusal into a NEW interpreter island (a
	// regression on islandCeiling) for no gain — the reach form has no body to
	// run. Decline so it refuses; the lens-as-const value/apply/getpath forms
	// (which do not route here) still compile natively.
	if sig.Callable != nil && sig.Callable.BodyPos < len(args) && IsReach(args[sig.Callable.BodyPos]) {
		return false
	}
	// A dispatch whose output is already recorded was handled by a structured
	// ReturnsFn hook (e.g. `case`'s desugar to a branch chain) — islanding it
	// would DOUBLE-record (the island fallback PLUS the structured event),
	// leaving the extra event unconsumed on the simulated stack. Skip it; the
	// generic RecordCall path that follows likewise early-returns.
	if _, done := es.producedBy[outs[0].ID]; done {
		return false
	}
	// A pure typed word (get/make/is/typeof/size/type-algebra) is
	// islanded ONLY when the dispatch is genuinely dynamic — a dynamic
	// operand or a dynamic (Any-widened) result the normal path would
	// refuse anyway. A concrete-operand one compiles as a faithful
	// CALL_NATIVE and must NOT be islanded: islanding poisons its result
	// to dynamic, refusing every downstream typed dispatch (a net
	// coverage LOSS). The code-body words always island (they never lower
	// to CALL_NATIVE).
	if sig.CompileEffect.Has(CompileIslandPure) && !sig.CompileEffect.Has(CompileFallbackBody) &&
		!anyDynamicCarrier(args) && !anyDynamicCarrier(outs) {
		return false
	}
	// CORE-dispatch guard: the matched sig must belong to the word's
	// MAIN-registry binding (pointer identity into its sig backing
	// array). A module-qualified call (`ArrayUtil.group`) dispatches
	// the inner native through a SUB-registry, so its sig is a
	// different pointer — baking the bare name would re-run the core
	// word of that name (different semantics). The guard rejects those
	// so only a faithful bare-name re-run compiles.
	fn := r.Lookup(word)
	if fn == nil {
		return false
	}
	sigOK := false
	for i := range fn.Signatures {
		if &fn.Signatures[i] == sig {
			sigOK = true
			break
		}
	}
	if !sigOK {
		return false
	}
	span := make([]Value, 0, len(args)+1)
	span = append(span, NewWord(word))
	var ins []Value
	for i, a := range args {
		// A TYPE operand (a bare type node — `make Point …`, `x is Foo`,
		// the type-algebra args) bakes as a token in the span: the island
		// re-resolves it against the registry's lattice at run time, the
		// same place OpPushType resolves canonical types. It is never
		// threaded (a stack PUSH_TYPE would mis-order the dispatch).
		if IsBareTypeNode(a) && a.ID != "" {
			if len(ins) > 0 {
				return false
			}
			span = append(span, a)
			continue
		}
		cv, ok := es.materialise(a)
		baked := ok && IsConcrete(cv)
		if baked && sig.NoEvalArgs[i] {
			// A code body: legitimately contains words, but every one
			// must be VM-resolvable (no check-time def carriers).
			if !bodyFreeForFallback(r, cv) {
				return false
			}
		} else if baked && !isInertConst(cv) {
			// A baked data arg must be DEEPLY concrete plain data — a
			// carrier element anywhere (e.g. a def-bound list whose
			// interior the check pass stripped) would bake a type-only
			// artefact into the island and diverge (`[ProperString …]`
			// vs the real strings). isInertConst rejects any
			// carrier/dynamic/bare node in the tree.
			return false
		}
		if baked {
			if len(ins) > 0 {
				// A baked arg AFTER a threaded one would break the
				// forward-prefix / stack-suffix split — the threaded
				// values must be the trailing run. Refuse.
				return false
			}
			span = append(span, cv)
			continue
		}
		// Not bakeable: thread the runtime value. A code body must be
		// baked (its tokens carry the island's program); only a data
		// arg can thread. resolveOperand (in RecordFallback) refuses
		// anything without compiled provenance.
		if sig.NoEvalArgs[i] {
			return false
		}
		ins = append(ins, a)
	}
	if len(ins) > 1 {
		// Multi-threaded islands need the trailing run laid out on the
		// operand stack deepest-first; that ordering is a Stage-5
		// follow-on. One threaded value (the common computed-receiver
		// shape) is positionally unambiguous.
		return false
	}
	barrier := sig.BarrierPos
	if barrier < 0 || barrier > len(sig.Args) {
		barrier = len(sig.Args)
	}
	// Span faithfulness for a BARRIERED sig (e.g. `get`: key forward,
	// receiver stack). The forward-form span `word arg0 arg1 …` can only
	// place forward-eligible (position < barrier) args after the word; a
	// stack arg must reach the dispatch from the operand stack.
	//   - A THREADED stack arg already does (the island preloads it), so
	//     the baked args must all be forward-eligible (k <= barrier).
	//   - An ALL-BAKED island (no thread) on a PURE typed word with no
	//     code body rebuilds the span in STACK form: the stack args (B..N)
	//     ride before the word deepest-first, then the word, then the
	//     forward args — `{m} get a` instead of `get a {m}`. Code-body
	//     words can't (a baked body list would auto-evaluate when stepped
	//     on the stack), so they keep the forward-eligible constraint.
	canStackForm := len(ins) == 0 && sig.CompileEffect.Has(CompileIslandPure) && !sig.CompileEffect.Has(CompileFallbackBody)
	if !canStackForm && len(args)-len(ins) > barrier {
		return false
	}
	if canStackForm && barrier < len(args) {
		baked := span[1:] // sig order, parallel to args
		ns := make([]Value, 0, len(span))
		for i := len(baked) - 1; i >= barrier; i-- { // stack args, deepest first
			ns = append(ns, baked[i])
		}
		ns = append(ns, NewWord(word))
		for i := 0; i < barrier; i++ { // forward args, in order
			ns = append(ns, baked[i])
		}
		span = ns
	}
	return es.RecordFallback(FallbackSpan{Tokens: span, Desc: word}, ins, outs[0], pos)
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
		}
		if r.Lookup(w.Name) != nil {
			return
		}
		if _, ok := typeNames[w.Name]; ok {
			return
		}
		free = false
	})
	return free
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
	found := false
	WalkBodyWords([]Value{body}, func(w WordInfo, _ Value) {
		switch w.Name {
		case "break", "continue", "return":
			found = true
		}
	})
	return found
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

// anyAnyCarrier reports whether any value is an Any-typed carrier — a value
// whose static type is unknown (a List/Map element, an opaque module-fn
// result). Unlike anyDynamicCarrier it does not require the Dynamic flag: a
// strict Any carrier conforms to no concrete container/operand type, so it
// fails matchSignature and reaches the no-signature recovery, where it is the
// signal that the dispatch is genuinely runtime-dynamic (poly), not a concrete
// type error.
func anyAnyCarrier(vs []Value) bool {
	for _, v := range vs {
		if v.Carrier && v.Parent != nil && v.Parent.Equal(TAny) {
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
				// (`make Path …`, `make Foo …`) or a structural type body
				// (`make P {}` for a class/record, whose literal carries an
				// ObjectTypeInfo/RecordTypeInfo payload). ValueType yields the
				// made *Type in both cases (the node itself, or the body's
				// Parent); a fresh carrier of it is the per-call instance.
				t := ValueType(args[m])
				if r != nil {
					t = CanonicalType(r, t)
				}
				out[i] = NewCarrier(t)
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
	es := r.Check.Emit
	if es == nil || !es.active() {
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
		if len(s.Args) == 2 && s.Args[0] != nil && s.Args[1] != nil &&
			s.Args[0].Equal(TIdeal) && s.Args[1].Equal(TMap) {
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
		// The merged carrier is a NEW value (an `if`/loop result), not arm `a`.
		// Keeping a's ID lets the result COLLIDE with a's own binding when an arm
		// returns a live local — `def v0 3 def v1 (if c [v0] [4])` makes the if-
		// result reuse v0's id, so a later `v0` reference resolves to the if-event
		// (the compiler bakes the wrong value). Mint a distinct identity.
		out.ID = GenerateID(IDPrefixForType(out.Parent))
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

	// Nested body analysis is not part of the enclosing straight
	// line: pause bytecode recording — unless a branch-lowering hook
	// armed fragment capture (the `if` ReturnsFn), in which case the
	// body's events record into a fragment for structured lowering.
	defer r.Check.Emit.bodyAnalysisGuard()()

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
func AnalyseLoopBody(r *Registry, body Value, bindNames []string, bindVals []Value) []Value {
	// Loop-lowering hook (`for`): when armed, register the loop
	// bindings as VM locals and capture each round's events as a
	// fragment — the final round's capture (the stable one) is what
	// the caller's RecordLoop consumes via TakeFragment.
	es := r.Check.Emit
	loopCapture := es.ConsumeLoopArm()
	if loopCapture {
		for _, v := range bindVals {
			es.RegisterLocal(v.ID)
		}
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
		var cp emitCheckpoint
		if loopCapture {
			cp = es.Checkpoint()
			es.ArmBranchCapture()
		}
		var adds map[string]Value
		stk, adds = RunCarrierBodyWithDefs(r, body)
		for i := len(bindNames) - 1; i >= 0; i-- {
			r.Defs.Pop(bindNames[i])
		}
		// Expose the original pre-loop bindings before re-joining.
		for i := len(installed) - 1; i >= 0; i-- {
			r.Defs.Pop(installed[i])
		}
		installed = installed[:0]
		joined := map[string]Value{}
		for k, v := range adds {
			if pre, ok := r.Defs.Top(k); ok {
				joined[k] = JoinCarriers(v, pre)
			} else {
				joined[k] = v
			}
		}
		for k, v := range joined {
			r.Defs.Push(k, v)
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

// GuardFactInfo is the payload a check-mode paren evaluation attaches
// to a single Boolean carrier result: the group's ORIGINAL tokens,
// preserved so guard narrowing can see the `x is T` structure that
// evaluation reduced to a bare Boolean
// (design/checker-accuracy-review.10.md A3 — without it, the canonical
// `if (x is T) …` paren form narrowed nothing while the list form
// `if [x is T] …` narrowed fine). Check-mode only; the runtime never
// produces carriers.
type GuardFactInfo struct {
	Toks []Value
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
	if len(elems) < 3 {
		return nil
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
func FnAnalysisKey(scopeID uint64, name string, args []Value, captures []CapturedBinding, body []Value) string {
	var sb strings.Builder
	sb.WriteString(strconv.FormatUint(scopeID, 10))
	sb.WriteByte('#')
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
	if len(body) > 0 {
		sb.WriteByte('@')
		sb.WriteString(strconv.Itoa(body[0].Pos.Row))
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(body[0].Pos.Col))
	}
	return sb.String()
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
func AnalyseFnBody(r *Registry, name string, paramNames []string, body []Value, args []Value, captures []CapturedBinding, declared []*Type) []Value {
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
	defer r.Check.Emit.fnBodyGuard()()
	if len(body) == 0 {
		return nil
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
	// once, instead of silently consuming the global step budget.
	if r.Check.FnAnalysisCounts == nil {
		r.Check.FnAnalysisCounts = map[string]int{}
	}
	quotaKey := name
	if quotaKey == "" {
		quotaKey = "<anon>"
	}
	r.Check.FnAnalysisCounts[quotaKey]++
	if n := r.Check.FnAnalysisCounts[quotaKey]; n > FnAnalysisQuota {
		if n == FnAnalysisQuota+1 {
			r.Check.AddDiagnostic(CheckDiagnostic{
				Code: "analysis_truncated",
				Detail: "fn " + quotaKey + " was analysed for more than " +
					strconv.Itoa(FnAnalysisQuota) + " distinct call shapes; later shapes are typed from the declaration (or dynamic Any) without body re-analysis",
				Word:     name,
				Severity: SeverityInfo,
			})
		}
		if len(declared) > 0 {
			out := make([]Value, len(declared))
			for i, t := range declared {
				out[i] = NewCarrier(t)
			}
			return out
		}
		return []Value{NewDynamicCarrier(TAny)}
	}
	if r.Check.FnInflight[key] {
		// Recursion detected. With declared returns, the declaration
		// is the induction hypothesis (assume-guarantee) — precise,
		// and the body's return check is the proof obligation. Without
		// it, break the cycle with an Any carrier and count the bail
		// so the enclosing analysis knows its result needs refinement.
		if len(declared) > 0 {
			out := make([]Value, len(declared))
			for i, t := range declared {
				out[i] = NewCarrier(t)
			}
			return out
		}
		r.Check.InflightBails++
		return []Value{NewCarrier(TAny)}
	}
	r.Check.FnInflight[key] = true
	defer delete(r.Check.FnInflight, key)

	// Diagnostics emitted from here down come from CALL-TIME code —
	// tag them FnBody so an undefined_word that turns out to be a
	// forward reference can be rescued at end of pass
	// (RescueForwardRefDiagnostics).
	r.Check.FnBodyDepth++
	defer func() { r.Check.FnBodyDepth-- }()

	// runOnce performs one full body analysis: snapshot def-stack
	// depths so any defs the body, captures, or parameter bindings
	// created unwind afterwards. The same snapshot is pushed as the
	// fn-entry baseline so any inner fn/afn construction inside the
	// body sees this scope as its enclosing-fn baseline — without it,
	// ComputeCaptures would treat outer params as if they lived at
	// module/global scope and miss the capture.
	runOnce := func() []Value {
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
			if es := r.Check.Emit; es != nil && es.active() {
				es.MarkUncompilable("fn body analysis error in " + name + ": " + err.Error())
			}
			result = nil
		}
		r.Defs.Restore(snapshot)
		return result
	}

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
	armed := r.Check.Emit != nil && r.Check.Emit.active()
	if r.Check.InflightBails > bailsBefore && len(result) > 0 && !armed {
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
	}

	result = stripZeroOutResiduals(r, result)
	r.Check.FnSummaries[key] = result
	return result
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
	es := r.Check.Emit
	if es == nil || !es.active() || len(stk) == 0 {
		return stk
	}
	filtered := make([]Value, 0, len(stk))
	for _, v := range stk {
		if pr, ok := es.producedBy[v.ID]; ok && es.eventInfo[pr.seq].zeroOut {
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
