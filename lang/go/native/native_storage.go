package native

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/eng/go"
)

// storageNatives covers `set` / `get` / `context`. The unified
// dispatch table mixes Node / Object / Array (kernel-territory
// containers) and Store (context-aware, copy-on-write) sigs in one
// place, keeping `set` and `get` polymorphic from the caller's
// perspective.
//
// `set` and `get` carry ReturnsFn closures on the Store sigs that
// thread the static type tracker (r.RecordContextSet /
// r.LookupContextType) so check-mode can recover a typed carrier
// from a previous set on the same key.
//
// Algorithms (GetKey, AsStore, AsArray, CowSet, AsClassInstance,
// AsMutableMap, …) live in eng; this file owns the word names and
// dispatch wiring.
var storageNatives = []NativeFunc{
	{
		Name: "set",

		Signatures: []Signature{
			// Store (copy-on-write)

			{
				Args:      []*Type{TString, TAny, TStore},
				Impl:      Go(setStoreHandler),
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TStore},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setStoreHandler),
				Returns:   []*Type{},
				ReturnsFn: setStoreReturnsFn, BarrierPos: -1,
			},

			// Map (immutable — copy-returning). Unlike the three
			// mutable containers above, a Map is a value: set returns
			// a NEW map with the key bound and leaves the receiver
			// untouched — the same contract as push / StructUtil.setpath.
			{
				Args:    []*Type{TString, TAny, TMap},
				Impl:    Go(setMapHandler),
				Returns: []*Type{TMap}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setMapHandler),
				Returns:   []*Type{TMap}, BarrierPos: -1,
			},

			// List (immutable — copy-returning, completing the column
			// rule: Map and List both return the updated copy).
			{
				Args:    []*Type{TInteger, TAny, TList},
				Impl:    Go(setListHandler),
				Returns: []*Type{TList}, BarrierPos: -1,
			},

			// Class instance (in-place, SEALED): a declared field
			// writes in place and returns nothing; an undeclared
			// field is a loud sealed_field error — see
			// design/CLASS-OBJECT.10.md §3.3.
			{
				Args:    []*Type{TString, TAny, TClass},
				Impl:    Go(setClassInstanceHandler),
				Returns: []*Type{}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TClass},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setClassInstanceHandler),
				Returns:   []*Type{}, BarrierPos: -1,
			},

			// FlexMap (in-place key set; returns the node for chaining)
			{
				Args:    []*Type{TString, TAny, TFlexMap},
				Impl:    Go(setFlexMapHandler),
				Returns: []*Type{TFlexMap}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TFlexMap},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setFlexMapHandler),
				Returns:   []*Type{TFlexMap}, BarrierPos: -1,
			},

			// FlexList (in-place index set; 0..len-1 only — sparse is
			// an error, growth is append's job)
			{
				Args:    []*Type{TInteger, TAny, TFlexList},
				Impl:    Go(setFlexListHandler),
				Returns: []*Type{TFlexList}, BarrierPos: -1,
			},

			// FlexXml (in-place attribute set; name → value, like the DOM
			// setAttribute. Children grow via `append`.)
			{
				Args:    []*Type{TString, TAny, TFlexXml},
				Impl:    Go(setFlexXmlHandler),
				Returns: []*Type{TFlexXml}, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TFlexXml},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setFlexXmlHandler),
				Returns:   []*Type{TFlexXml}, BarrierPos: -1,
			},

			// Micron (IMMUTABLE — always errors): the explicit erroring
			// sig pins "set: <Kind> values are immutable" where
			// sig-absence would raise an opaque dispatch failure.
			// isInertConst's MicronPayload arm relies on this staying
			// an error — see eng/go/emit.go.
			{
				Args:      []*Type{TString, TAny, TMicron},
				Impl:      Go(setMicronHandler),
				Returns:   []*Type{},
				ReturnsFn: setMicronReturns, BarrierPos: -1,
			},
			{
				Args:      []*Type{TAtom, TAny, TMicron},
				QuoteArgs: map[int]bool{0: true},
				Impl:      Go(setMicronHandler),
				Returns:   []*Type{},
				ReturnsFn: setMicronReturns, BarrierPos: -1,
			},
		},
	},
	{
		Name:          "get",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// get EVALUATES its key: the bare-word-quoting atom sigs live on
		// `dot` (the word the `.`/`!.` sugar lowers to), so `lst get i`
		// reads the VALUE of i, not the literal field "i". A literal field
		// name uses `dot` (`m dot a`) or a string key (`m get "a"`).
		Signatures: stripQuoteArgs(accessorGetSignatures()),
	},
	{
		Name:          "dot",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// dot is `get` PLUS the bare-word-quoting atom sigs. The `.` / `!.`
		// dot-sugar lowers to dot/dotr (lowerReach), so `m.a` ≡ `m dot a`
		// reads the literal field "a" regardless of any binding of a.
		Signatures: accessorGetSignatures(),
	},
	{
		Name: "context",

		Signatures: []Signature{{
			Args:    []*Type{},
			Impl:    Go(contextHandler),
			Returns: []*Type{TStore}, BarrierPos: -1,
		}},
	},
}

// accessorGetSignatures returns the full read-accessor signature set shared
// by `get` and `dot`. `dot` uses it verbatim (so a bare-word key is quoted
// as a literal field, the `.`-sugar behaviour); `get` uses the
// stripQuoteArgs variant (the same overloads with QuoteArgs cleared, so a
// bare WORD is evaluated but an already-evaluated Atom key still matches).
// Keeping ONE
// source for both words prevents the two from drifting apart.
func accessorGetSignatures() []Signature {
	return []Signature{
		// [Key | Node] — covers Map, List, Options, record-shape. The
		// atom / string key sigs narrow a concrete map FIELD read via
		// getNodeReturns; the integer-key sig stays Any because an
		// integer key is a LIST index (handled precisely downstream by
		// tryFoldStaticIndex) or a stringified map key — getNodeReturns
		// must not stringify-and-miss an integer over a list.
		{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getNodeHandler), ReturnsFn: getNodeReturns},
		{Args: []*Type{TString, TNode}, BarrierPos: 1, Impl: Go(getNodeHandler), ReturnsFn: getNodeReturns},
		{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Impl: Go(getNodeHandler), ReturnsFn: getIntKeyReturns},
		// [Key | ModuleExport] — transparent export access + $module/$name
		{Args: []*Type{TAtom, TModuleExport}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getModuleExportHandler), ReturnsFn: moduleExportGetReturns},
		{Args: []*Type{TString, TModuleExport}, BarrierPos: 1, Impl: Go(getModuleExportHandler), ReturnsFn: moduleExportGetReturns},
		// [Key | Module] — descriptor fields (id/kind/file/folder/exports)
		{Args: []*Type{TAtom, TModuleInst}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getModuleInstHandler), Returns: []*Type{TAny}},
		{Args: []*Type{TString, TModuleInst}, BarrierPos: 1, Impl: Go(getModuleInstHandler), Returns: []*Type{TAny}},
		// [Key | Class instance] — flat field read (no prototype
		// chain; class instances resolve every field at make). Field
		// type resolved from the schema (getObjectReturns).
		{Args: []*Type{TAtom, TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getObjectReturns},
		{Args: []*Type{TString, TClass}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getObjectReturns},
		// Resource/Entity instances (SDK object hierarchy) — flat field
		// read. Field type resolved from the Resource schema (getResourceReturns).
		{Args: []*Type{TAtom, TResource}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getResourceReturns},
		{Args: []*Type{TString, TResource}, BarrierPos: 1, Impl: Go(getObjectHandler), ReturnsFn: getResourceReturns},
		// [Key | None] — chained-read propagation
		{Args: []*Type{TAny, TNone}, BarrierPos: 1, Impl: Go(getNoneHandler), Returns: []*Type{TNone}},
		// [Key | Store] — check-mode-aware ReturnsFn picks up a
		// typed carrier from a previously-set key.
		{
			Args: []*Type{TString, TStore}, BarrierPos: 1, Impl: Go(getStoreHandler),
			ReturnsFn: getStoreReturnsFn,
		},
		{
			Args: []*Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getStoreHandler),
			ReturnsFn: getStoreReturnsFn,
		},
		// [Key | Micron] — structured-scalar property read: primary
		// fields plus the derived properties (Pathon parts/abs,
		// Emailon address, Urlon href). A miss reads none; the strict
		// twin is in accessorGetrSignatures. Microns sit under Scalar,
		// so these don't overlap the [Key | Node] rows above.
		{Args: []*Type{TAtom, TMicron}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getMicronHandler), ReturnsFn: getMicronReturns},
		{Args: []*Type{TString, TMicron}, BarrierPos: 1, Impl: Go(getMicronHandler), ReturnsFn: getMicronReturns},
		// [Key | Node/Xml] — well-known field read: 'tag' / 'attr' /
		// 'cren' (any other key → none). More specific than the
		// [Key | Node] sigs above, so it wins for an XML receiver and
		// keeps getNodeHandler off the non-map XML payload. Covers
		// FlexXml too (it conforms to Node/Xml).
		{Args: []*Type{TAtom, TXml}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getXmlHandler), Returns: []*Type{TAny}},
		{Args: []*Type{TString, TXml}, BarrierPos: 1, Impl: Go(getXmlHandler), Returns: []*Type{TAny}},
	}
}

// stripQuoteArgs returns sigs with the bare-word-quoting QuoteArgs flag
// cleared — the `get`/`getr` subset that EVALUATES its key. It does NOT drop
// the atom overloads: an already-evaluated Atom value (a variable bound to
// `a/q`, a computed atom) still matches `{TAtom, …}` and the handler coerces
// it, so `def k a/q  {a:1} get k` reads field "a". Only the implicit
// quote-on-bare-word goes away — an unbound bare word stays a Word, matches
// no overload, and errors, which is the split's intent. `dot`/`dotr` keep the
// QuoteArgs versions (literal bare-word keys). Sigs are copied so the shared
// accessorGetSignatures slice (used by `dot`) keeps its QuoteArgs.
func stripQuoteArgs(sigs []Signature) []Signature {
	out := make([]Signature, len(sigs))
	for i, s := range sigs {
		s.QuoteArgs = nil
		out[i] = s
	}
	return out
}

// ---- kernel-container handlers (Node / Store / Class / None) ----

// setMapHandler is the Map form of set. A Map stays immutable: the
// handler returns a NEW map with the key bound (overwriting an existing
// entry), leaving the receiver untouched. This is the language's rule
// of thumb made concrete — mutable containers (Store / Object / Array)
// mutate in place and return nothing; immutable values return the
// updated copy. Keys are strings or atoms, computed keys via parens:
// `m set (k) v`.
func setMapHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	m, err := RequireConcreteMap(args[2], "set")
	if err != nil {
		return nil, err
	}
	key := StoreKey(args[0])
	// The copy-returning column stays ENTIRELY immutable: a value with
	// flex inside is snapshot to its plain shape (eng.AdoptIntoNode),
	// so the "immutable" result can never change underneath through a
	// live flex handle. Flex-free values pass through untouched.
	val, aerr := eng.AdoptIntoNode(args[1])
	if aerr != nil {
		return nil, aerr
	}
	out := NewOrderedMap()
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out.Set(k, v)
	}
	out.Set(key, val)
	return []Value{NewMap(out)}, nil
}

// setClassInstanceHandler is the sealed in-place write for class
// instances: a field declared in the class schema (own or inherited)
// writes into the flat field map and returns nothing; an undeclared
// field raises sealed_field loudly — the open-bag use case belongs to
// plain maps / FlexMaps, not class instances (design/CLASS-OBJECT.10.md).
func setClassInstanceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	if !IsConcrete(container) {
		return nil, r.AqlError("set_error", "set: cannot set field on type literal", "set")
	}
	oi, ok := container.Data.(ClassInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("set: expected a class instance, got %s", container.Parent.String())
	}
	key := StoreKey(args[0])
	val := args[1]
	if oi.TypeRef != nil {
		all := oi.TypeRef.AllFields()
		constraint, declared := all.Get(key)
		if !declared {
			name := oi.TypeRef.Name
			if name == "" {
				name = container.Parent.Name
			}
			return nil, r.AqlErrorHint("sealed_field",
				fmt.Sprintf("set: %q is not a field of %s (fields: %s)", key, name, strings.Join(all.Keys(), " ")),
				"set",
				"class instances are sealed — declare the field in the class schema, or use a plain map for open data")
		}
		// Write-time enforcement matches construction: the same strict
		// field check make runs (typed fields conform, predicates run,
		// defaulted fields constrain to the default's own type).
		checked, err := MakeClassFieldValue(val, constraint, r)
		if err != nil {
			return nil, r.AqlError("type_error",
				fmt.Sprintf("set: field %q: %s", key, err.Error()), "set")
		}
		val = checked
	}
	oi.Fields.Set(key, val)
	return nil, nil
}

func setFlexMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	m, err := AsMutableMap(container)
	if err != nil {
		return nil, r.AqlError("set_error", "set: expected a FlexMap, got "+container.Parent.String(), "set")
	}
	// A flex tree stays ENTIRELY mutable: a plain Node value is deep-
	// flexed on the way in — otherwise a later write into the immutable
	// inner is copy-returning and silently lost. Flex handles share.
	val, aerr := eng.AdoptIntoFlex(args[1])
	if aerr != nil {
		return nil, r.AqlError("set_error", aerr.Error(), "set")
	}
	m.Set(StoreKey(args[0]), val)
	return []Value{container}, nil
}

func setFlexListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	fd, err := AsFlexList(container)
	if err != nil {
		return nil, r.AqlError("set_error", "set: expected a FlexList, got "+container.Parent.String(), "set")
	}
	asInt, ierr := args[0].AsConcreteInteger()
	if ierr != nil {
		return nil, r.AqlError("set_error", "set: FlexList index must be a concrete integer", "set")
	}
	idx := int(asInt)
	if idx < 0 || idx >= len(fd.Elems) {
		return nil, r.AqlErrorHint("set_error",
			fmt.Sprintf("set: index %d out of bounds for FlexList (length %d)", idx, len(fd.Elems)),
			"set", "use append to grow a FlexList; sparse FlexLists are an error")
	}
	// Entirely-mutable invariant: adopt a plain Node element into flex.
	val, aerr := eng.AdoptIntoFlex(args[1])
	if aerr != nil {
		return nil, r.AqlError("set_error", aerr.Error(), "set")
	}
	fd.Elems[idx] = val
	return []Value{container}, nil
}

func setFlexXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	container := args[2]
	fd, err := AsFlexXml(container)
	if err != nil {
		return nil, r.AqlError("set_error", "set: expected a FlexXml, got "+container.Parent.String(), "set")
	}
	name := StoreKey(args[0])
	if !eng.IsValidXmlName(name) {
		return nil, r.AqlErrorHint("set_error",
			"set: "+name+" is not a valid XML attribute name", "set",
			"attribute names start with a letter or '_' and contain letters, digits, '-', '.', or ':'")
	}
	// Attributes are String-valued; store a String view of the value so a
	// flex attribute round-trips through `node` and renders correctly.
	val := args[1]
	if !val.Is(TString) {
		val = NewString(ValToString(val))
	}
	fd.Attr.Set(StoreKey(args[0]), val)
	return []Value{container}, nil
}

// getXmlHandler reads a well-known field of a Node/Xml (or FlexXml)
// element: 'tag' → String, 'attr' → Map, 'cren' → List of all children.
// Any other key reads as None (lenient, like a missing map key), so
// `x.tag` / `x.attr` / `x.cren` dotted access works. The element-only
// view and subtree text are the computed words `elem` / `text`.
func getXmlHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	tag, attr, cren, ok := XmlParts(args[1])
	if !ok {
		return nil, r.AqlError("get_error", "get: cannot access field on a non-Xml value", "get")
	}
	switch getKey(args[0]) {
	case "tag":
		return []Value{NewString(tag)}, nil
	case "attr":
		return []Value{NewMap(attr)}, nil
	case "cren":
		return []Value{NewList(cren)}, nil
	default:
		return []Value{NewTypeLiteral(TNone)}, nil
	}
}

// isClosureBearingWrapper reports whether v is a module-method wrapper FnDef
// whose inner native declares a CallableSpec (a code-body higher-order word
// like Rand.list-of). Such a wrapper, surfaced concrete from a field read,
// dispatches through execFnDefLiteral's trivial-delegation -> execMatch ->
// carrierResults -> tryRecordClosure, lowering its NoEvalArgs body to a
// closure unit. The shape is seed-agnostic (see getNodeReturns), so it is
// sound to resolve from an abstract instance carrier. A plain RNG-bound
// wrapper (rand-int) has no CallableSpec and stays dynamic.
func isClosureBearingWrapper(v Value) bool {
	fd, ok := v.Data.(FnDefInfo)
	if !ok || fd.Registry == nil || fd.Name == "" {
		return false
	}
	inner := fd.Registry.Lookup(fd.Name)
	if inner == nil {
		return false
	}
	for i := range inner.Signatures {
		if inner.Signatures[i].Callable != nil {
			return true
		}
	}
	return false
}

// getNodeReturns narrows a field read over a CONCRETE map / record-shape
// to the stored value's type when the key is statically known, instead of
// the poison Any the [Key|Node] sigs otherwise declare — `{a:1 b:2}.a`
// checks as Integer, `{x:'hi'}.x` as ProperString, a statically-absent key
// as None. A computed receiver, a non-literal key, a non-OrderedMap
// container (AsMap nil), or a list falls back to dynamic(Any) — the exact
// shape carrierResults already produced for the declared Any, so those
// paths (e.g. tryFoldStaticIndex's list-element fold) are unchanged.
// Object / Class instances are abstract type carriers in check mode (no
// field payload), so they keep their own Any sigs until schema resolution
// lands. Mirrors getNodeHandler's map branch so check and run agree.
func getNodeReturns(args []Value, _ *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 {
		return dyn
	}
	key, container := args[0], args[1]
	if !IsConcrete(key) {
		return dyn
	}
	// A record-schema carrier (a record-typed param reparented at the call
	// boundary by narrowArgsToParams) carries a RecordTypeInfo schema rather
	// than a concrete map. Recover the field's declared type from the schema so
	// a body's `c get "field"` types instead of degrading to Any. The result is
	// GRADUAL, never strict: a field absent at run time (an open map) or a value
	// the param guard did not pin reads optimistically and is discharged by a
	// guard — never a committed strict op on a possibly-absent field (the
	// Array<T> OOB→None lesson). A field outside the schema, or a dispatch-
	// bearing (Function/FnDef) field, keeps dynamic Any.
	if rt, ok := container.Data.(RecordTypeInfo); ok && rt.Fields != nil {
		fv, ok := rt.Fields.Get(getKey(key))
		if !ok {
			return dyn
		}
		ft := ValueType(fv)
		if ft == nil || ft.ConformsTo(TFunction) || ft.ConformsTo(TFnDef) {
			return dyn
		}
		return []Value{NewDynamicCarrier(ft)}
	}
	if !IsConcrete(container) || !container.Parent.ConformsTo(TMap) {
		return dyn
	}
	m, err := AsMap(container)
	if err != nil || m == nil {
		return dyn
	}
	val, ok := m.Get(getKey(key))
	if !ok {
		return []Value{NewCarrier(TNone)} // statically-absent key reads as None
	}
	// Only narrow plain DATA fields. A field bearing dispatch — a stored
	// Function / FnDef, a /r ref (Reach), or a word-splice — keeps the
	// dynamic Any the poly / island path already handles: returning its
	// concrete value would push the compiler to lower a fn-value call or a
	// modifier re-dispatch and refuse to compile (fn-value.tsv `m.f 2 3`,
	// path-modifier.tsv `m.a/u`). Return a FRESH carrier of the field's
	// TYPE, not the stored value — the stored value's Value ID is shared
	// with the map field and collides in the emitter's operand-provenance
	// tracking (`def a {b:5} [a.b]`). Deep nesting (`.a.b`) narrows the
	// first hop to a Map carrier and the rest falls to dynamic(Any), which
	// is still strictly better than the all-Any baseline.
	// A closure-bearing module-method wrapper FnDef (`r.list-of`, where `r` is
	// a `Rand.with-seed` instance) folds to the concrete wrapper so its dispatch
	// records the SAME closure-word path the module-export form takes
	// (`Rand.list-of [body] n` -> PUSH_CLOSURE). The wrapper delegates to an
	// inner native carrying a CallableSpec; surfacing it lets execFnDefLiteral's
	// trivial-delegation reach execMatch -> carrierResults -> tryRecordClosure, so
	// the NoEvalArgs body lowers to a closure unit (re-run per iteration) rather
	// than being auto-evaluated and frozen. The wrapper SHAPE (which methods
	// exist, their sigs, NoEvalArgs, CallableSpec) is static for any instance --
	// only the captured RNG state differs, and a closure-driver handler like
	// rand-list-of does NOT itself draw from the RNG (the body does, against the
	// module-export generator it names), so the resolved shape is seed-agnostic.
	// Cloned for a fresh ID (the stored value's ID is shared with the map field
	// and would collide in operand-provenance tracking). A NON-closure wrapper
	// (`r.int`, RNG-bound) stays dynamic so it does not bake a seed-specific
	// handler -- it takes the runtime CALL_DYNAMIC path instead.
	if (val.Parent.ConformsTo(TFunction) || val.Parent.ConformsTo(TFnDef)) &&
		isClosureBearingWrapper(val) {
		return []Value{CloneValue(val)}
	}
	if val.Parent.ConformsTo(TFunction) || val.Parent.ConformsTo(TFnDef) ||
		IsReach(val) || IsSplice(val) {
		return dyn
	}
	// A concrete LIST / MAP field folds to the stored CONTAINER value, not
	// merely its type carrier: cloned so it carries a FRESH ID (the stored
	// value's ID is shared with the map field and would collide in the
	// emitter's operand-provenance tracking — `def a {b:5} [a.b]`). Folding
	// the concrete container lets a downstream `(m get "k") size` / `for`
	// over the field see a concrete count instead of a carrier Integer, so
	// a statically-empty collection's loop body is correctly pruned rather
	// than analysed over Any (module-test:38). Scalars keep the type
	// carrier — there is nothing to fold, and the ID-collision guard above
	// is the reason we never returned the bare stored scalar.
	if IsConcrete(val) && (val.Parent.ConformsTo(TList) || val.Parent.ConformsTo(TMap)) {
		return []Value{CloneValue(val)}
	}
	return []Value{NewCarrier(val.Parent)}
}

// getIntKeyReturns narrows an INTEGER-key read over a CONCRETE list to the
// indexed element's type — the plain-check-mode analogue of the emit-only
// tryFoldStaticIndex fold, so `[10 20 30] get 0` checks as Integer instead of
// dynamic(Any). An out-of-range or negative index reads as None (mirroring
// getNodeHandler). A computed index, a non-list receiver (a Map keyed by a
// stringified integer, an abstract object carrier), or a dispatch-bearing
// element keeps dynamic(Any). Restricting to LISTS is deliberate: an integer
// key over a list is an INDEX, never a stringified map key (the bug that
// mistyped a parselang `get 1` over a list as None).
func getIntKeyReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if len(args) != 2 || !IsConcrete(args[0]) || !IsConcrete(args[1]) ||
		!args[1].Parent.ConformsTo(TList) {
		return dyn
	}
	idx, err := AsInteger(args[0])
	if err != nil {
		return dyn
	}
	list, lerr := AsList(args[1])
	if lerr != nil || list.IsNil() {
		return dyn
	}
	i := int(idx)
	if i < 0 || i >= list.Len() {
		return []Value{NewCarrier(TNone)} // out-of-range index reads as None
	}
	el := list.Get(i)
	if el.Parent.ConformsTo(TFunction) || el.Parent.ConformsTo(TFnDef) ||
		IsReach(el) || IsSplice(el) {
		return dyn
	}
	// A QUOTED CODE element (a dispatch-table entry — `def ops [quote [1
	// add 2] …] (ops get 0)`) carries its analysed stack effect on the
	// carrier, so a downstream `do` types its result instead of the
	// dynamic(Any) hatch (design/checker-precision-fronts.0.md §1 stage 1).
	// The helper self-gates to plain (non-Compiling) check mode and
	// declines anything it cannot analyse cleanly, so this only ever
	// NARROWS the plain-carrier fallback below.
	if c, ok := AnalyseCodeEffect(r, el); ok {
		return []Value{c}
	}
	return []Value{NewCarrier(el.Parent)}
}

func getNodeHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	// Integer key: list index access.
	if key.Parent.ConformsTo(TInteger) {
		idx, _ := AsInteger(key)
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			i := int(idx)
			if i < 0 || i >= list.Len() {
				return []Value{NewTypeLiteral(TNone)}, nil
			}
			return []Value{list.Get(i)}, nil
		}
		// Fall through to map lookup with stringified key.
	}
	// String/atom/word key: map property access.
	k := getKey(key)
	if m, _ := AsMap(container); m != nil {
		val, ok := m.Get(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	return []Value{NewTypeLiteral(TNone)}, nil
}

// getObjectReturns narrows a field read over an OBJECT / CLASS instance
// carrier to the field's DECLARED type, resolved from the type SCHEMA. In
// check mode the instance is an abstract type carrier (no field payload),
// so the field type comes from the bound type's ClassTypeInfo.AllFields()
// (own + inherited). A method field (function-typed), an absent/sealed
// field (→ None), or a type whose schema can't be resolved keeps the
// dynamic(Any) the poly path handles — the same dispatch-bearing exclusion
// as the concrete-map case (a returned fn value would push the compiler to
// lower a fn-value call and refuse).
func getObjectReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if r == nil || len(args) != 2 || !IsConcrete(args[0]) || args[1].Parent == nil {
		return dyn
	}
	body, ok := r.TopTypeBody(args[1].Parent.Leaf())
	if !ok {
		return dyn
	}
	info, oerr := AsClassType(body)
	if oerr != nil {
		return dyn
	}
	fv, ok := info.AllFields().Get(getKey(args[0]))
	if !ok {
		return []Value{NewCarrier(TNone)} // sealed / absent field reads as None
	}
	ft := ValueType(fv)
	if ft == nil || ft.ConformsTo(TFunction) || ft.ConformsTo(TFnDef) {
		return dyn
	}
	return []Value{NewCarrier(ft)}
}

// getResourceReturns is getObjectReturns for the Resource/Entity
// hierarchy: it reads the field type from the ResourceType schema so a
// Resource-instance field read (`e.spec`) carries its declared type in
// check/bytecode contexts instead of degrading to Any. Resource/Entity
// are installed as type bindings (installResourceTypes), so their
// schema is reachable via TopTypeBody like a class type's.
func getResourceReturns(args []Value, r *Registry) []Value {
	dyn := []Value{NewDynamicCarrier(TAny)}
	if r == nil || len(args) != 2 || !IsConcrete(args[0]) || args[1].Parent == nil {
		return dyn
	}
	// Resource/Entity are def bindings, not type bindings, so resolve
	// the schema through the def store by the instance's type name.
	info, ok := lookupResourceTypeByName(r, args[1].Parent.Leaf())
	if !ok {
		return dyn
	}
	fv, ok := info.AllFields().Get(getKey(args[0]))
	if !ok {
		return []Value{NewCarrier(TNone)} // absent field reads as None
	}
	ft := ValueType(fv)
	if ft == nil || ft.ConformsTo(TFunction) || ft.ConformsTo(TFnDef) {
		return dyn
	}
	return []Value{NewCarrier(ft)}
}

func getObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("get_error", "get: cannot access property on type literal", "get")
	}
	k := getKey(key)
	if m, err := AsMutableMap(container); err == nil {
		val, found := m.Get(k)
		if !found {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	if ri, err := AsResourceInstance(container); err == nil {
		val, ok := ri.GetField(k)
		if !ok {
			return []Value{NewTypeLiteral(TNone)}, nil
		}
		return []Value{val}, nil
	}
	oi, _ := AsClassInstance(container)
	val, ok := oi.GetField(k)
	if !ok {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	return []Value{val}, nil
}

func getNoneHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewTypeLiteral(TNone)}, nil
}

// ---- set Store handler ----

func setStoreHandler(args []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store, err := AsStore(args[2])
	if err != nil {
		return nil, fmt.Errorf("set: expected a Store, got %s", args[2].Parent.String())
	}
	key := StoreKey(args[0])
	CowSet(store, key, args[1], reg)
	return nil, nil
}

func setStoreReturnsFn(args []Value, r *Registry) []Value {
	r.Check.RecordContextSet(StoreKey(args[0]), args[1])
	return nil
}

// ---- get Store handler ----

func getStoreHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return nil, fmt.Errorf("get: expected a Store, got %s", args[1].Parent.String())
	}
	key := getKey(args[0])
	val, ok := store.Get(key)
	if !ok {
		return nil, r.AqlError("unknown key_error", fmt.Sprintf("unknown key: %s", key), "unknown key")
	}
	return []Value{val}, nil
}

func getStoreReturnsFn(args []Value, r *Registry) []Value {
	v, ok := r.Check.LookupContextType(StoreKey(args[0]))
	if !ok {
		// Escape hatch: the checker has no proven type for this key.
		// Emit a bounded gradual carrier dynamic(Any) — optimistically
		// compatible with any slot — rather than strict Carry<Any>, which
		// would fail every typed slot downstream and force a no_signature
		// or Any catch-all. (design/dynamic-modality-report.10.md, escape
		// hatch 1.) A key recorded by a prior `set` keeps its real, strict
		// carrier.
		return []Value{NewDynamicCarrier(TAny)}
	}
	return []Value{v}
}

// ---- context handler ----

// contextHandler implements the "context" word that pushes the
// current context Store onto the stack.
//
// The context is a Store (Object/Store), allowing get/set to operate on it
// directly and prototype chain resolution for nested scopes.
func contextHandler(_ []Value, _ map[string]Value, _ []Value, reg *Registry) ([]Value, error) {
	store := reg.Contexts.Top()
	if store == nil {
		return nil, reg.AqlError("context_error", "context: no active context", "context")
	}
	return []Value{NewStoreValue(TStore, store)}, nil
}

// setListHandler is the List form of set: copy-returning, like Map —
// a NEW list with the element at the index replaced; the receiver is
// untouched. Out-of-range indices are a loud error (edits, not
// lookups). Completes the immutable column of the container 2x2.
func setListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	_idx, err := args[0].AsConcreteInteger()
	if err != nil {
		return nil, r.AqlError("set_error", "set: expected a concrete Integer index", "set")
	}
	lst, err2 := RequireConcreteList(args[2], "set")
	if err2 != nil {
		return nil, err2
	}
	idx := int(_idx)
	n := lst.Len()
	if idx < 0 || idx >= n {
		return nil, r.AqlError("index_out_of_range",
			fmt.Sprintf("set: index %d out of range for list of length %d", idx, n), "set")
	}
	// Entirely-immutable invariant for the copy-returning column: see
	// setMapHandler.
	val, aerr := eng.AdoptIntoNode(args[1])
	if aerr != nil {
		return nil, aerr
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		out[i] = lst.Get(i)
	}
	out[idx] = val
	return []Value{NewList(out)}, nil
}
