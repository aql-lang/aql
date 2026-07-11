package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// accessorNatives covers strict-access words.
//
// `getr` is the strict variant of `get`: same arg order
// ([Key, Container]) but it returns an error when the parent is None
// or the key/index is missing, instead of silently returning None.
//
// Usage:
//
//	{a:1} getr a       → 1
//	getr a {a:1}       → 1
//	{a:1} b getr       → ERROR (key not found)
//	none a getr        → ERROR (parent is none)
//	[10,20] 5 getr     → ERROR (index out of bounds)
var accessorNatives = []NativeFunc{
	{
		Name:          "getr",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// getr EVALUATES its key (the strict sibling of `get`); the
		// bare-word-quoting atom sigs live on `dotr`, which the `!.` sugar
		// lowers to. `m getr "k"` / `m getr <int>` work; a literal field
		// name uses `dotr` (`m dotr a`).
		Signatures: stripQuoteArgs(accessorGetrSignatures()),
	},
	{
		Name:          "dotr",
		CompileEffect: CompileModuleFold | CompileIslandPure,
		// dotr is `getr` PLUS the bare-word-quoting atom sigs — the strict
		// (`!.`) dot-sugar target. `m!.a` ≡ `m dotr a` reads the literal
		// field "a" and raises if absent.
		Signatures: accessorGetrSignatures(),
	},
	{
		// `has` is the Boolean presence predicate — the missing third
		// sibling of `get` (None on miss) and `getr` (raise on miss):
		// "is this key/index BOUND, regardless of value", so a
		// present-but-None entry is distinguishable from an absent one
		// (decision DX report finding 3). TOTAL within its container
		// table: a missing key, an out-of-range index, a None parent,
		// or a type-literal container all answer false — it never
		// raises, so it composes inside if/filter/conditions.
		//
		//	{a:None} has a       → true   (present, value is None)
		//	{a:1}    has b       → false  (absent)
		//	[10,20]  has 1       → true
		//	none     has a       → false
		Name: "has",
		// CompileModuleFold: a pure presence reader. CompileQuoteInert: the
		// bare-word key overloads (`{a:1} has b`, `none has a`) quote the key
		// as an inert Atom const, so the dispatch bakes a plain CALL_NATIVE
		// over the baked container + key — the VM runs the same pure handler.
		CompileEffect: CompileModuleFold | CompileQuoteInert,

		Signatures: []Signature{
			// [Key | Node] — Map, List, Options, record-shape
			{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(hasNodeHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TNode}, BarrierPos: 1, Impl: Go(hasNodeHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Impl: Go(hasNodeHandler), Returns: []*Type{TBoolean}},
			// [Key | Class instance]
			{Args: []*Type{TAtom, TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TClass}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TAtom, TResource}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TResource}, BarrierPos: 1, Impl: Go(hasObjectHandler), Returns: []*Type{TBoolean}},
			// [Key | Store]
			{Args: []*Type{TAtom, TStore}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(hasStoreHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TStore}, BarrierPos: 1, Impl: Go(hasStoreHandler), Returns: []*Type{TBoolean}},
			// [Key | Micron] — property presence, including the
			// derived properties (address/href/parts/abs) and the
			// optional Urlon fields (an absent port answers false).
			{Args: []*Type{TAtom, TMicron}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(hasMicronHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TString, TMicron}, BarrierPos: 1, Impl: Go(hasMicronHandler), Returns: []*Type{TBoolean}},
			// [Key | None] — total: an absent parent answers false.
			// The Atom/q overload captures a bare-word key (`none has
			// a`, `(m get sub) has k`), going one better than get/getr,
			// whose None sigs take only an evaluated key.
			{Args: []*Type{TAtom, TNone}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(hasNoneHandler), Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TNone}, BarrierPos: 1, Impl: Go(hasNoneHandler), Returns: []*Type{TBoolean}},
		},
	},
}

// accessorGetrSignatures returns the full strict-read signature set shared by
// `getr` and `dotr`. `dotr` uses it verbatim (bare-word key quoted as a
// literal field — the `!.` sugar); `getr` uses the stripQuoteArgs variant
// (same overloads, QuoteArgs cleared: an evaluated Atom key still matches; a
// bare WORD is evaluated). One source keeps the two words from drifting.
func accessorGetrSignatures() []Signature {
	return []Signature{
		// [Key | Node] — key forward, container from stack
		// Field type narrows exactly as get's twin does (getNodeReturns);
		// the strict-read difference is the miss behaviour (raise vs none),
		// so a statically-provable miss over a concrete container is
		// flagged at check time (getrNodeReturns).
		{Args: []*Type{TAtom, TNode}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrMapHandler), ReturnsFn: getrNodeReturns},
		{Args: []*Type{TString, TNode}, BarrierPos: 1, Impl: Go(getrMapHandler), ReturnsFn: getrNodeReturns},
		{Args: []*Type{TInteger, TNode}, BarrierPos: 1, Impl: Go(getrMapHandler), ReturnsFn: returnsGetrIndexChecked},
		// [Key | Class instance] — strict field read (mirrors get's TClass
		// sigs; getrObjectHandler resolves the flat instance via
		// AsClassInstance and raises on a missing field). Field type
		// narrows from the schema via getObjectReturns, as get does; a
		// concrete key provably outside the CLOSED class schema is a
		// guaranteed runtime not_found, flagged by the getr wrapper.
		{Args: []*Type{TAtom, TClass}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrObjectHandler), ReturnsFn: getrObjectReturns},
		{Args: []*Type{TString, TClass}, BarrierPos: 1, Impl: Go(getrObjectHandler), ReturnsFn: getrObjectReturns},
		{Args: []*Type{TAtom, TResource}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrObjectHandler), Returns: []*Type{TAny}},
		{Args: []*Type{TString, TResource}, BarrierPos: 1, Impl: Go(getrObjectHandler), Returns: []*Type{TAny}},
		// [Key | ModuleExport] / [Key | Module]
		{Args: []*Type{TAtom, TModuleExport}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrModuleExportHandler), ReturnsFn: moduleExportGetrReturns},
		{Args: []*Type{TString, TModuleExport}, BarrierPos: 1, Impl: Go(getrModuleExportHandler), ReturnsFn: moduleExportGetrReturns},
		{Args: []*Type{TAtom, TModuleInst}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrModuleInstHandler), ReturnsFn: moduleInstGetReturns},
		{Args: []*Type{TString, TModuleInst}, BarrierPos: 1, Impl: Go(getrModuleInstHandler), ReturnsFn: moduleInstGetReturns},
		// [Key | Micron] — strict structured-scalar property read
		// (mirrors get's TMicron sigs; a miss raises not_found instead
		// of reading none).
		{Args: []*Type{TAtom, TMicron}, QuoteArgs: map[int]bool{0: true}, BarrierPos: 1, Impl: Go(getrMicronHandler), ReturnsFn: getrMicronReturns},
		{Args: []*Type{TString, TMicron}, BarrierPos: 1, Impl: Go(getrMicronHandler), ReturnsFn: getrMicronReturns},
		// [Key | None]
		// Always raises not_found — never produces a value; a receiver the
		// checker KNOWS is None (a literal, a strict None carrier from a
		// statically-absent lenient read) flags the same error statically.
		{Args: []*Type{TAny, TNone}, BarrierPos: 1, Impl: Go(getrNoneHandler), ReturnsFn: getrNoneReturns},
	}
}

// ---- has handlers (the get lookups, returning presence as Boolean) ----

func hasNodeHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return []Value{NewBoolean(false)}, nil
	}
	if key.Parent.ConformsTo(TInteger) {
		idx, _ := AsInteger(key)
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			i := int(idx)
			return []Value{NewBoolean(i >= 0 && i < list.Len())}, nil
		}
		// Fall through to map lookup with stringified key (get parity).
	}
	k := getKey(key)
	if m, _ := AsMap(container); m != nil {
		_, ok := m.Get(k)
		return []Value{NewBoolean(ok)}, nil
	}
	return []Value{NewBoolean(false)}, nil
}

func hasObjectHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	container := args[1]
	if !IsConcrete(container) {
		return []Value{NewBoolean(false)}, nil
	}
	k := getKey(args[0])
	if m, err := AsMutableMap(container); err == nil {
		_, found := m.Get(k)
		return []Value{NewBoolean(found)}, nil
	}
	if ri, err := AsResourceInstance(container); err == nil {
		_, ok := ri.GetField(k)
		return []Value{NewBoolean(ok)}, nil
	}
	oi, err := AsClassInstance(container)
	if err != nil {
		return []Value{NewBoolean(false)}, nil
	}
	_, ok := oi.GetField(k)
	return []Value{NewBoolean(ok)}, nil
}

func hasStoreHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	store, err := AsStore(args[1])
	if err != nil {
		return []Value{NewBoolean(false)}, nil
	}
	_, ok := store.Get(getKey(args[0]))
	return []Value{NewBoolean(ok)}, nil
}

func hasNoneHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(false)}, nil
}

// returnsGetrIndexChecked is the check-mode ReturnsFn for the
// integer-index `getr` sig. It runs the static bounds check (a no-op
// for map/object containers and unknown-length carriers) and returns
// the container's element-type carrier. `getr` errors at runtime on an
// out-of-bounds list index, so a provably out-of-range literal —
// `[10 20] 5 getr` — is flagged at `aql check`.
func returnsGetrIndexChecked(args []Value, r *Registry) []Value {
	if len(args) >= 2 {
		CheckListIndex(r, args[0], args[1], "getr")
		return ReturnsListElemAt(1)(args, r)
	}
	return []Value{NewCarrier(TAny)}
}

// getrNodeReturns is getNodeReturns plus the strict-read static-miss
// contract (the map/list twin of getrMicronReturns): getr means REQUIRED,
// so a miss the checker can prove — a concrete key absent from a concrete
// map, or a concrete list read with a non-integer key — is a guaranteed
// runtime error, flagged with the byte-identical getrMapHandler text. The
// miss is re-proved against the container directly (never inferred from
// the None carrier getNodeReturns returns) because a PRESENT key whose
// stored value is `none` produces the same carrier shape and succeeds at
// runtime. Everything non-concrete keeps getNodeReturns' gradual result.
func getrNodeReturns(args []Value, r *Registry) []Value {
	out := getNodeReturns(args, r)
	if r == nil || !r.Check.IsActive() || len(args) != 2 ||
		!IsConcrete(args[0]) || !IsConcrete(args[1]) {
		return out
	}
	key, container := args[0], args[1]
	if container.Parent.ConformsTo(TList) && !key.Parent.ConformsTo(TInteger) {
		eng.CheckAddUniqueDiagnostic(r, "getr_error",
			fmt.Sprintf("getr: expected a map, got %s", container.Parent.String()), "getr", key.Pos())
		return out
	}
	if m, err := AsMap(container); err == nil && m != nil && container.Parent.ConformsTo(TMap) {
		if _, ok := m.Get(getKey(key)); !ok {
			eng.CheckAddUniqueDiagnostic(r, "not_found",
				fmt.Sprintf("getr: key %q not found in map", getKey(key)), "getr", key.Pos())
		}
	}
	return out
}

// getrObjectReturns is getObjectReturns plus the strict-read static-miss
// contract for CLASS instances: a class schema is a closed field set (a
// runtime `set` cannot add fields — it rejects unknown names), so a
// concrete key the resolvable schema lacks is a guaranteed runtime
// not_found, flagged with getrObjectHandler's text. An unresolvable
// schema, a computed key, and every present field keep getObjectReturns'
// result unchanged.
func getrObjectReturns(args []Value, r *Registry) []Value {
	out := getObjectReturns(args, r)
	if r == nil || !r.Check.IsActive() || len(args) != 2 ||
		!IsConcrete(args[0]) || args[1].Parent == nil {
		return out
	}
	body, ok := r.TopTypeBody(args[1].Parent.Leaf())
	if !ok {
		return out
	}
	info, oerr := AsClassType(body)
	if oerr != nil {
		return out
	}
	if _, ok := info.AllFields().Get(getKey(args[0])); !ok {
		eng.CheckAddUniqueDiagnostic(r, "not_found",
			fmt.Sprintf("getr: field %q not found in object", getKey(args[0])), "getr", args[0].Pos())
	}
	return out
}

// getrNoneReturns mirrors getrNoneHandler statically: the [Any|None] sig
// ALWAYS raises, so a receiver the checker knows is None — the `none`
// literal, or a strict (non-dynamic) None carrier from a statically-absent
// lenient read — flags the same not_found. A DYNAMIC None (an optimistic
// maybe-miss) stays silent; the residual model stays empty either way
// (the sig never produces a value).
func getrNoneReturns(args []Value, r *Registry) []Value {
	if r != nil && r.Check.IsActive() && len(args) == 2 &&
		eng.IsNoneShape(args[1]) && !args[1].Dynamic {
		eng.CheckAddUniqueDiagnostic(r, "not_found",
			"getr: parent is None — nothing to read a key from", "getr", args[0].Pos())
	}
	return []Value{}
}

// notFoundKeyError builds the strict-read not_found error with a
// did-you-mean suggestion over the container's actual keys (the
// diagnostics phase-4 hook: a typo'd key suggests its neighbours) and
// the key's source position, so the report points at the offending
// access. The compiled path records the SAME error (built by
// buildNotFoundKeyError) via RecordTrapErr, so the two engines match.
func notFoundKeyError(r *Registry, detail, key string, keyPos eng.SrcPos, keys []string) error {
	return buildNotFoundKeyError(r, detail, key, keyPos, keys)
}

// buildNotFoundKeyError is the shared not_found builder both the runtime
// handlers and the compile-time trap use, so the strict-read miss error
// is byte-identical across engines.
func buildNotFoundKeyError(r *Registry, detail, key string, keyPos eng.SrcPos, keys []string) *eng.AqlError {
	ae := eng.MakeAqlErrorAt("not_found", detail, "getr", srcOf(r), "", keyPos)
	if s := eng.DidYouMean(key, keys); s != "" {
		ae.Suggestions = append(ae.Suggestions, eng.DiagSuggestion{Message: s})
	}
	return ae
}

// srcOf returns the registry's source text (for error excerpts), or "".
func srcOf(r *Registry) string {
	if r == nil {
		return ""
	}
	return r.Source
}

func getrMapHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	// Integer key on list.
	if key.Parent.ConformsTo(TInteger) {
		if list, _ := AsList(container); !list.IsNil() && container.Parent.ConformsTo(TList) {
			_as3, _ := AsInteger(key)
			idx := int(_as3)
			if idx < 0 || idx >= list.Len() {
				return nil, fmt.Errorf("getr: index %d out of bounds (length %d)", idx, list.Len())
			}
			return []Value{list.Get(idx)}, nil
		}
	}
	k := getKey(key)
	m, _ := AsMap(container)
	if m == nil {
		return nil, fmt.Errorf("getr: expected a map, got %s", container.Parent.String())
	}
	val, ok := m.Get(k)
	if !ok {
		return nil, notFoundKeyError(r, fmt.Sprintf("getr: key %q not found in map", k), k, key.Pos(), m.Keys())
	}
	return []Value{val}, nil
}

func getrObjectHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	key := args[0]
	container := args[1]
	if !IsConcrete(container) {
		return nil, r.AqlError("getr_error", "getr: cannot access property on type literal", "getr")
	}
	k := getKey(key)
	if m, err := AsMutableMap(container); err == nil {
		val, found := m.Get(k)
		if !found {
			return nil, notFoundKeyError(r, fmt.Sprintf("getr: key %q not found in object", k), k, key.Pos(), m.Keys())
		}
		return []Value{val}, nil
	}
	if ri, err := AsResourceInstance(container); err == nil {
		val, ok := ri.GetField(k)
		if !ok {
			return nil, notFoundKeyError(r, fmt.Sprintf("getr: field %q not found in resource", k), k, key.Pos(), ri.Fields.Keys())
		}
		return []Value{val}, nil
	}
	oi, _ := AsClassInstance(container)
	val, ok := oi.GetField(k)
	if !ok {
		return nil, notFoundKeyError(r, fmt.Sprintf("getr: field %q not found in object", k), k, key.Pos(), oi.Fields.Keys())
	}
	return []Value{val}, nil
}

func getrNoneHandler(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	return nil, r.AqlError("not_found", "getr: parent is None — nothing to read a key from", "getr")
}
