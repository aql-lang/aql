package eng

// Core value classifiers (Stage 3a of the four-piece split): pure
// predicates over core Values and registry scope state that the compiler,
// checker, AND interpreter all consult. Moved down from the compiler-piece
// files — they carry no recording or emission behavior.

// materialiseMember materialises one container member and reports whether it
// CHANGED — a stripped carrier recovered to a concrete original (different
// Carrier flag or ID). ok=false when the member's original is unknown, so the
// whole container cannot be recovered. The shared per-member step of
// materialise's list and map arms.
// bearsActiveTokens reports whether a value (recursively through lists and
// maps) contains a token the interpreter evaluates or re-steps — a word, a
// paren expression, an interpolation, a splice or reach marker.
func bearsActiveTokens(v Value) bool {
	if IsWord(v) || IsParenExpr(v) || IsInterpString(v) || IsSplice(v) || IsReach(v) {
		return true
	}
	switch d := v.Data.(type) {
	case ListPayload:
		for _, e := range d.Elems {
			if bearsActiveTokens(e) {
				return true
			}
		}
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if bearsActiveTokens(mv) {
				return true
			}
		}
	}
	return false
}

// ModuleScopeBinding reports whether name's active binding sits at module /
// global scope — NOT an enclosing fn's param or body-local (the
// ComputeCaptures depth rule: Depth > baseline means enclosing-fn-local). A
// nil baseline (no enclosing fn) makes every binding module scope.
func ModuleScopeBinding(r *Registry, name string) bool {
	baseline := r.TopFnBaseline()
	if baseline == nil {
		return true
	}
	return r.Defs.Depth(name) <= baseline[name]
}

// fnValueZeroArg reports whether v is a function VALUE whose LANDING the
// read-guard must refuse: it carries a genuine 0-arg overload the
// interpreter auto-fires the moment the value lands with no operands
// (containerFnAutoDispatchRisk), in a shape the compiled landing does
// not yet model. Built from the kernel's canonical Fallback-flag
// predicates (fnValueHasZeroArgSig / fnValueOnlyZeroArgSigs, engine.go)
// — this replaced a count-based phantom heuristic that encoded the same
// verdicts opaquely.
//
// The refusal set is REPRESENTATION-dependent, and deliberately so —
// probe-verified (2026-08-01) on `m.x` with a 0-arg+1-arg overload set:
//   - the DIRECT-literal spelling `{x: (fn …)}` compiles and agrees
//     (7/7): the check pass runs the interpreter loop over the concrete
//     member, and the recorded events model the fire;
//   - the PARKED spelling `{x: mx/r}` (an aggregate view, recognisable
//     by its synthetic Fallback sig) compiled to the raw fn value —
//     a silent divergence — so it must refuse until the landing model
//     covers parked mixed-overload members (the tracked graduation;
//     frontier-nur038-seal.tsv's mixed-overload row pins the refusal).
//
// A pure property fn (every real overload 0-arg) refuses in BOTH
// representations. Merging the two arms = extending the compiled
// landing model, not editing this predicate.
func fnValueZeroArg(v Value) bool {
	d, ok := v.Data.(FnDefInfo)
	if !ok {
		return false
	}
	if !fnValueHasZeroArgSig(v) {
		return false // no genuine 0-arg overload: needs args, lands as data
	}
	if fnValueOnlyZeroArgSigs(d) {
		return true // pure property fn: auto-fires in both representations
	}
	for i := range d.Signatures {
		if d.Signatures[i].Fallback {
			return true // parked aggregate view: mixed-set landing unmodelled
		}
	}
	return false // direct literal: the recorded events model the fire
}

func isInertConst(v Value) bool {
	if v.Carrier || v.Dynamic || IsBareTypeNode(v) {
		return false
	}
	switch d := v.Data.(type) {
	case IntPayload, FloatPayload, StrPayload, BoolPayload, AtomPayload,
		PathonPayload, NonePayload, BigIntPayload, DecimalPayload,
		TimePayload, DurationPayload, TimezonePayload:
		return true
	case MicronPayload:
		// A Micron instance (Emailon / Urlon / user kind) is inert ONLY
		// because the family is immutable — `set` on a Micron is an
		// explicit error (the erroring set signatures in the language
		// layer). The payload's OrderedMap is pointer-backed, so if
		// mutation were ever allowed, pooled consts would corrupt
		// across loop iterations and this arm must be removed.
		return true
	case ExtensionPayload:
		// The kernel cannot see into an extension payload, so mutability
		// is the OWNING TYPE's call: bake only when its Behavior declares
		// the values immutable (the ConstBakeable capability — Bytes),
		// reached by the same parent-chain walk every capability dispatch
		// uses. Extension types without the capability — Socket, Listener,
		// Timeout/Interval, Module instances, every plugin type that never
		// opted in — keep the historical refusal. This is what lets a
		// module-scope `def crlf (convert Bytes "\r\n")` bake into a
		// stored-fn unit (mini-s3's recv-until delimiter) with freshness
		// still owned by the existing frozen-read / dep-snapshot gates.
		for t := v.Parent; t != nil; t = t.Parent {
			if cb, ok := t.Behavior().(ConstBakeable); ok {
				return cb.BakeableConst(v)
			}
		}
		return false
	case MicronTypeInfo:
		// A Micron type body (`refine Micron {fields}`): structural
		// descriptor like the RecordTypeInfo arm below — sound when its
		// field map is carrier-free.
		return typeBodyConstOK(v)
	case DepScalarInfo:
		// A predicate / refinement type (`Integer gt 10`): self-contained
		// (base family + bound, no registry, no canonical-pointer hazard per
		// eng CLAUDE.md), so the value bakes by value into the const pool.
		// The bound is recovered for a stripped operand via origByID
		// (RememberOriginal at the constructor); type-algebra words
		// (tcmp/teq/tand/…) then run over the baked predicate at run time.
		return true
	case RecordTypeInfo, OptionsTypeInfo, ChildTypeInfo, DisjunctInfo, ClassTypeInfo, TableTypeInfo:
		// STRUCTURAL type bodies (what a bound type name pushes at a
		// use site — make's operand). Sound as consts when their
		// interior is carrier-free (typeBodyConstOK): the payload is
		// pointer-backed (shared, not copied) and the minted lattice
		// node rides the body's Parent POINTER, which stays
		// canonical. Never deduped. A class/object body qualifies only
		// when every field default is data (no method fn-values). A
		// Table type (`Test.TestSet`) is a thin wrapper over its row
		// RecordType, so it folds whenever that record does.
		return typeBodyConstOK(v)
	case FnUndefInfo:
		// A function SIGNATURE value (`fnsig [[Integer] [String]]`,
		// `typeof (Mapper of [Integer String])`): pure descriptor data —
		// param/return *Type pointers, no invocable body and no mutable state —
		// so it bakes by value (the *Type pointers are shared, already
		// canonical from construction). typeof/teq/is then read the baked
		// signature at run time. A pattern-bearing param (rare in signatures)
		// could embed non-const data, so it refuses conservatively.
		return fnSigConstOK(d)
	case FnDefInfo:
		// A function VALUE used as DATA — a residual (`f/r`), a map/list member
		// (`{b:f/r}`), or an introspection operand (`arityof (fn …)`) — NOT a
		// call site. It bakes as a const only with no closure state: no captured
		// bindings (which would snapshot check-pass values, divergent from the VM
		// pass) and no module sub-registry. The body tokens ride inside the
		// payload and are never re-stepped while the value is data; a CALL of the
		// value is a separate dispatch path (a bare `(fn …) args` auto-dispatch
		// records the fn-body splice and refuses; a `/r`-referenced fn does not
		// auto-dispatch, so `f/r` / `{b:f/r}` are pure data).
		if len(d.Captured) > 0 {
			return false
		}
		if d.Registry == nil {
			return true
		}
		// A module-export fn value bakes as DATA — a bare residual (`MathUtil.sqrt`),
		// a branch-arm operand, a container member, OR a comparator passed to another
		// fn (`xs M.sort M.by-num`). The sub-registry pointer it carries is the SAME
		// object the compiled run shares (RunProgram runs on the check-pass
		// registry), so the baked const and the live fn are one object — no
		// check/VM value divergence. The VM applies it faithfully at run time:
		//   - a TRIVIAL-DELEGATION wrapper (`MathUtil.sqrt`, every own sig a
		//     `[Word(inner)]` pass-through) via callDynamic → tryNativeFnApply, which
		//     re-resolves the inner native in that registry;
		//   - a REAL boru body via the island sub-engine (callDynTrailTop/…'s
		//     `vc.island().Run([fn, args…])`), which INTERPRETS the fn in
		//     fnDef.Registry — CallBoru, module-private scope and all. So a real body
		//     applies soundly too (compile == interpret, verified). A macro stays
		//     refused (applied only by name / compile-time expansion, never as data).
		return !d.Macro
	case *SurfaceInfo:
		// A surface type (`def Shape surface {area: (fnsig …)}`): an immutable
		// contract descriptor riding its canonical minted node via the Type
		// pointer (shared, not copied). Its conformance set (Conform, filled in
		// by `exposes`) is consulted through the SAME shared payload the
		// canonical node's installed unifier holds — and the compiled path runs
		// the VM over the check-pass registry without re-minting, so the baked
		// const and the live surface are one object, never divergent. Admitting
		// it lets `Shape` as a residual / operand to is/typeof/teq/tand/tor/tnot/
		// unify all compile. The Required method shapes are fnsig values, const
		// by fnSigConstOK above.
		return surfaceConstOK(d)
	case ListPayload:
		for _, e := range d.Elems {
			if !isInertConstMember(e) {
				return false
			}
		}
		return true
	case MapPayload:
		if d.M == nil {
			return false
		}
		for _, k := range d.M.Keys() {
			mv, _ := d.M.Get(k)
			if !isInertConstMember(mv) {
				return false
			}
		}
		return true
	case XmlElementPayload:
		// An immutable Node/Xml literal (`<a x="1"><b/>text</a>`). It is a
		// constant value at parse time (parser/xml_literal.go emits
		// NewXmlElement; only a ${}-interpolated literal becomes the deferred
		// Word/__XI builder, which is genuine runtime construction and stays
		// refused). Value-semantics with structural sharing — never mutated in
		// place: the MUTABLE FlexXml is *FlexXmlData, which falls to default and
		// never bakes (the bytecode_constbake_test mutation-safety guard) — so it
		// is sound to pool, exactly like the List / Map cases. Bakes when its
		// attribute values and child nodes are themselves inert members (text
		// scalars + nested immutable elements, recursed via isInertConstMember →
		// isInertConst).
		for _, c := range d.Cren {
			if !isInertConstMember(c) {
				return false
			}
		}
		if d.Attr != nil {
			for _, k := range d.Attr.Keys() {
				av, _ := d.Attr.Get(k)
				if !isInertConstMember(av) {
					return false
				}
			}
		}
		return true
	case ReachInfo:
		// A receiverless inert lens (`$.name`, `$.a.b`, `$!.x`, `$.1`). See
		// isInertReach: only the non-eval, no-receiver, all-literal-key shape
		// qualifies — the dot-access Eval reach (which the engine expands in
		// place) is excluded.
		return isInertReach(v)
	case ParenExprPayload:
		// A codequote'd (Quoted) ParenExpr — `codequote (1 add 2)` →
		// `paren([1 word(add) 2])`. It is immutable CODE-AS-DATA: stepLiteral
		// leaves a Quoted ParenExpr unevaluated (engine.go step 4), and the VM
		// never re-steps a const, so it bakes by value exactly like the
		// macroexpand token list. An UNQUOTED ParenExpr is expanded and
		// re-stepped in place, so it is NEVER a const — gate strictly on Quoted.
		// Its tokens must themselves be inert members (Words / atoms / scalars /
		// nested inert parens), screened by isInertConstMember.
		return isInertQuotedParen(v)
	case *TypeSchemaInfo:
		// An installed generic schema (`def Box gen [T] class {value:T}`). The
		// schema is immutable data — its instantiation memo lives in the
		// registry, not this struct — and rides the canonical minted node via
		// its Type pointer (shared, not copied), so it bakes as a const exactly
		// like a structural type body. Admitting it lets `make Box {…}` (T
		// inferred at run time), `is`/`typeof`/`teq` over the schema, and the
		// schema as a residual all compile; the instantiated `Box of [Integer]`
		// type already baked via typeBodyConstOK.
		return schemaConstOK(d)
	default:
		return false
	}
}

// isFnTypedCarrier reports whether v is a Function-typed CARRIER — a
// [Function]-returning call result on the simulated stack (e.g. `(mk2 5)`), as
// distinct from a CONCRETE baked fn value (Carrier false, the introspection /
// inert-`/r` case). The carrier bit is what resolves the apply-vs-inert
// ambiguity in Finalize: a carrier lead auto-applies, a concrete fn does not.
func isFnTypedCarrier(v Value) bool {
	return v.Carrier && v.Parent != nil &&
		v.Parent.ConformsTo(TFunction)
}

// isFnValueResidual reports whether v is ANY fn value — a concrete FnDefInfo (a
// baked /r reference) or a Function-typed value (carrier or not). Used to
// keep a fn value out of the trailing-arg positions of a leading-fn apply.
func isFnValueResidual(v Value) bool {
	// One body with the VM's runtime test (isAppliableFn, vm.go): the
	// recorder's residual question and the VM's apply question are the
	// SAME question, and post-ADR-011 both reduce to Function
	// conformance (the payload arm covers values mid-construction).
	return isAppliableFn(v)
}

// isModuleFamilyValue reports whether v is a concrete module value — an
// Ideal/Module descriptor, or a module NAMESPACE (a plain Map carrying the
// module-namespace facet — the value `import` binds; NUR038 retired the
// Ideal/ModuleExport wrapper type). The descriptor is identified by the
// kernel-declared TModule (moduletype.go — the former string-path probe
// is gone), the namespace by its kernel facet. These values are
// immutable and produced deterministically by `import`, so a pure read
// of one is a compile-time constant (runtime export growth is
// ledger-modelled — module_export_growth.go).
func isModuleFamilyValue(v Value) bool {
	if !IsConcrete(v) || v.Parent == nil {
		return false
	}
	return ModuleNSOf(v) != nil || v.Parent.Equal(TModule)
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

// isAppliableFn reports whether a runtime value is a callable the interpreter
// would auto-apply: a Function-typed value or an FnDefInfo payload.
func isAppliableFn(v Value) bool {
	if _, ok := v.Data.(FnDefInfo); ok {
		return true
	}
	return v.Parent != nil && v.Parent.ConformsTo(TFunction)
}

// dynStackShuffleWords is the closed set of Forth-style stack words whose
// dispatch the compiler may trust over dynamically-shaped stacks: each is
// stack-only (BarrierPos 0 — it never forward-collects) and, when its
// registered signature is the single all-Any one, one extra or differently-
// typed stack value cannot flip which overload it takes. Shared by
// dynamicStackShuffleOK (the refusal bypass for a MATCHED dispatch over a
// dynamic operand) and dynShuffleConsumerAt (the gradual-arity modeling
// gate for a mixed-arity mutator's statement-position result).
var dynStackShuffleWords = map[string]bool{
	"dup": true, "swap": true, "drop": true, "over": true, "rot": true,
	"nip": true, "tuck": true, "dup2": true, "swap2": true, "drop2": true,
	"over2": true,
}
