package eng

// Store-identity context typing — stage 1 of
// design/checker-precision-fronts.0.md §2. CheckState.ContextTypes is ONE
// flat string-keyed namespace for the whole check pass, so two stores'
// same-named keys JOIN and every store read is answered from a global
// map. The StoreShapeInfo payload keys the same best-effort typing by
// STORE IDENTITY instead: each store-class container the checker can
// identify (a context Store layer, a `flex` map) carries ONE abstract
// shape minted in check mode, and `set`/`get` over that carrier
// read/write ITS KeyTypes.
//
// The carrier is a NEW ABSTRACT payload, never a preserved concrete
// *StoreInstanceInfo: stores are mutable runtime state, and the check
// pass must be observation-free — toCarrier deliberately strips real
// store payloads, and the check-mode set/get handlers never run the
// runtime Impl (execMatch's check intercept routes them through
// ReturnsFn). The shape is what those check-mode twins mutate instead
// of the real store.
//
// Gradual/compat contract (precision only increases):
//   - a store the minting misses (an unshaped carrier from a fn param,
//     a join, an older pass) keeps the flat-ContextTypes path entirely;
//   - a SHAPED store whose key misses ALSO falls back to the flat map
//     (today's optimism, unchanged) — the shape only answers keys it
//     saw written through this store;
//   - writes go to BOTH the shape and the flat map, so unshaped readers
//     elsewhere in the pass lose nothing.
//
// Compile-pass discipline: minting, reads, and writes are all gated to
// plain (non-Compiling) check passes. Store and flex programs COMPILE
// natively today through the flat-map typing, and the compiled stream
// must stay byte-identical — the shape machinery is checker precision,
// not compile coverage (the CodeEffectInfo discipline).

// StoreShapeInfo is the abstract check-mode shape of ONE store-class
// container (a context Store layer, a FlexMap): the per-key JOIN of the
// value carriers written through it. A pointer payload deliberately —
// every Value copy of the carrier (a def binding, a stack duplicate, a
// nested-field read) aliases the SAME shape, mirroring the runtime
// aliasing of the mutable container it stands for. All mutation is
// join-only (JoinCarriers), so sandbox rollbacks that cannot un-mutate
// a shared pointee only ever WIDEN a claim — monotone, never unsound.
type StoreShapeInfo struct {
	// Scope is the context-stack depth at mint time for context-layer
	// shapes (stage-2 layering substrate); 0 for non-context containers
	// (flex maps, patrun instances).
	Scope int
	// KeyTypes records key → joined written-value carrier, with the
	// same JoinCarriers-on-rewrite semantics as the flat ContextTypes
	// map (so a single-store program reads back exactly what the flat
	// path recorded). nil until the first write.
	KeyTypes map[string]Value
	// Vals is the join of the UNKEYED value writes for containers whose
	// write key is not a string (a patrun's pattern-keyed `add`). The
	// zero Value (Parent == nil) means "nothing recorded".
	Vals Value
	// ValsPoisoned marks the unkeyed join unusable: a write the shape
	// cannot describe honestly (a dispatch-bearing value — a stored
	// lambda re-dispatched by the reader) poisons it, and readers keep
	// the pre-existing dynamic(Any) hatch.
	ValsPoisoned bool
	// DeclaredVal is the DECLARED element type of a typed container (a
	// `patrun T` table): nil for an inferred/untyped shape. When set, a
	// reader (`find`) surfaces `dynamic(DeclaredVal ∪ None)` directly,
	// bypassing the Vals join and its poisoning entirely — the type is a
	// declaration, not an inference.
	DeclaredVal *Type
}

// payloadMarker — see payload.go's catalogue; registered there.

// NewStoreShapeCarrier mints an abstract store-shaped carrier: a
// carrier of t (TStore, TFlexMap, …) whose Data is a FRESH
// *StoreShapeInfo. One mint per creation site / live layer — sharing
// the returned Value shares the shape (that is the aliasing model).
func NewStoreShapeCarrier(t *Type, scope int) Value {
	v := NewCarrier(t)
	v.Data = &StoreShapeInfo{Scope: scope}
	return v
}

// StoreShapeOf returns the shape payload of a store-shaped CARRIER, or
// ok=false for anything else (a real store value, a bare carrier, a
// concrete container).
func StoreShapeOf(v Value) (*StoreShapeInfo, bool) {
	if !v.Carrier {
		return nil, false
	}
	ss, ok := v.Data.(*StoreShapeInfo)
	return ss, ok
}

// RecordKey joins a written value carrier into the shape under key —
// the per-store twin of CheckState.RecordContextSet, same join
// semantics. Join-only: repeat writes widen, never replace (replace-
// on-write is the stage-4 flow-sensitivity step; join keeps every
// sandbox/branch interaction monotone).
func (s *StoreShapeInfo) RecordKey(key string, v Value) {
	if s == nil || key == "" {
		return
	}
	if s.KeyTypes == nil {
		s.KeyTypes = map[string]Value{}
	}
	if existing, ok := s.KeyTypes[key]; ok {
		s.KeyTypes[key] = JoinCarriers(existing, v)
		return
	}
	s.KeyTypes[key] = v
}

// LookupKey returns the joined carrier recorded for key through THIS
// store, or ok=false when this store never saw the key written (the
// caller then falls back to the flat map — today's behaviour).
func (s *StoreShapeInfo) LookupKey(key string) (Value, bool) {
	if s == nil {
		return Value{}, false
	}
	v, ok := s.KeyTypes[key]
	return v, ok
}

// RecordVal joins an unkeyed value write (a patrun `add`) into Vals. A
// value the shape cannot describe honestly — a dispatch-bearing stored
// Function/FnDef/Reach/Splice, whose reader re-dispatches it — poisons
// the join instead, and LookupVals declines from then on.
func (s *StoreShapeInfo) RecordVal(v Value) {
	if s == nil || s.ValsPoisoned {
		return
	}
	if v.Parent == nil || v.Parent.ConformsTo(TFunction) || v.Parent.ConformsTo(TFnDef) ||
		IsReach(v) || IsSplice(v) {
		s.ValsPoisoned = true
		s.Vals = Value{}
		return
	}
	if s.Vals.Parent == nil {
		s.Vals = v
		return
	}
	s.Vals = JoinCarriers(s.Vals, v)
}

// LookupVals returns the unkeyed value join, or ok=false when nothing
// was recorded or the join is poisoned.
func (s *StoreShapeInfo) LookupVals() (Value, bool) {
	if s == nil || s.ValsPoisoned || s.Vals.Parent == nil {
		return Value{}, false
	}
	return s.Vals, true
}

// CloneShape returns a shape with a COPIED KeyTypes map (entries — and
// any nested shape pointers inside them — still shared). Used where the
// RUNTIME disconnects aliasing with a deep copy (`flex` of a flex): the
// copy's later writes must not narrow reads through the original's
// claims and vice versa. Entry-level sharing is safe because all
// mutation is join-only.
func (s *StoreShapeInfo) CloneShape() *StoreShapeInfo {
	if s == nil {
		return nil
	}
	cp := &StoreShapeInfo{Scope: s.Scope, Vals: s.Vals, ValsPoisoned: s.ValsPoisoned, DeclaredVal: s.DeclaredVal}
	if s.KeyTypes != nil {
		cp.KeyTypes = make(map[string]Value, len(s.KeyTypes))
		for k, v := range s.KeyTypes {
			cp.KeyTypes[k] = v
		}
	}
	return cp
}

// ContextShape returns the abstract shape carrier for a LIVE context
// layer, minting it on first sight. Keyed by the layer's identity (the
// *StoreInstanceInfo pointer): in check mode the runtime COW replace
// never runs (set's Impl is intercepted), so the top-of-stack pointer
// is stable for the lifetime of its scope — every `context` call in one
// scope shares one shape, and each engine Run's pushed layer (a `do`
// body, a module load) gets its own. Holding the pointer as a map key
// also keeps the layer reachable, so a recycled allocation can never
// alias two scopes' shapes. The returned Value is a fresh-ID copy
// sharing the shape pointer.
func (c *CheckState) ContextShape(store *StoreInstanceInfo, scope int) Value {
	if c == nil || store == nil {
		return NewCarrier(TStore)
	}
	if c.CtxShapes == nil {
		c.CtxShapes = map[*StoreInstanceInfo]Value{}
	}
	v, ok := c.CtxShapes[store]
	if !ok {
		v = NewStoreShapeCarrier(TStore, scope)
		c.CtxShapes[store] = v
	}
	v.ID = GenerateID(IDPrefixForType(v.Parent))
	return v
}

// flexShapeMaxDepth caps MintFlexShapeCarrier's recursion over nested
// concrete maps. A concrete map graph is finite but may be deep or (via
// shared flex handles adopted into a literal) cyclic; past the cap a
// nested map field decays to a bare FlexMap carrier.
const flexShapeMaxDepth = 8

// MintFlexShapeCarrier converts a CONCRETE plain map — the operand of
// `flex` in check mode — into an abstract FlexMap carrier bearing its
// StoreShapeInfo, mirroring FlexDeepCopy's shape statically:
//
//   - a nested concrete plain-map field mints a nested FlexMap shape
//     (recursively, depth-capped) — runtime FlexDeepCopy converts it to
//     a FlexMap;
//   - a concrete list / xml field records a bare FlexList / FlexXml
//     carrier (element typing is a later stage);
//   - an already-shaped field (a flex handle stored in the literal)
//     shares its shape pointer — runtime adoption shares the handle;
//   - a dispatch-bearing field (Function/FnDef/Reach/Splice) is OMITTED
//     so reads keep the dynamic(Any) hatch (the getNodeReturns
//     exclusion — surfacing it would push fn-value dispatch);
//   - any other field (concrete scalar, carrier) records as-is.
//
// ok=false declines (non-concrete / non-plain-map / structural-type
// input) and the caller keeps its legacy conversion, so precision only
// increases.
func MintFlexShapeCarrier(src Value, depth int) (Value, bool) {
	if depth > flexShapeMaxDepth {
		return Value{}, false
	}
	if !IsConcrete(src) || src.Parent == nil || !src.Parent.ConformsTo(TMap) ||
		IsRecordType(src) || IsOptionsType(src) || IsTypedMap(src) {
		return Value{}, false
	}
	m, err := AsMap(src)
	if err != nil || m == nil {
		return Value{}, false
	}
	out := NewStoreShapeCarrier(TFlexMap, 0)
	ss, _ := StoreShapeOf(out)
	for _, k := range m.Keys() {
		fv, _ := m.Get(k)
		if fv.Parent == nil || fv.Parent.ConformsTo(TFunction) || fv.Parent.ConformsTo(TFnDef) ||
			IsReach(fv) || IsSplice(fv) {
			continue // dispatch-bearing: absent key reads dynamic(Any)
		}
		ss.RecordKey(k, AdoptShapeValue(fv, depth+1))
	}
	return out, true
}

// AdoptShapeValue is the static twin of AdoptIntoFlex for a value
// WRITTEN into a shaped flex container (a `set` value, a minted field):
// a concrete plain map becomes a nested FlexMap shape, a concrete
// list/xml becomes the corresponding bare flex carrier, an
// already-shaped carrier shares its pointer, anything else records
// as-is.
func AdoptShapeValue(v Value, depth int) Value {
	if _, ok := StoreShapeOf(v); ok {
		return v // flex handles share
	}
	if nested, ok := MintFlexShapeCarrier(v, depth); ok {
		return nested
	}
	if IsConcrete(v) && v.Parent != nil {
		switch {
		case v.Parent.ConformsTo(TXml):
			return NewCarrier(TFlexXml)
		case v.Parent.ConformsTo(TList):
			return NewCarrier(TFlexList)
		}
	}
	return v
}

// ShapeFieldRead surfaces a shape-recorded value at a READ site
// (`get`/`dot` over a shaped flex container). Always GRADUAL, never a
// strict or concrete commitment — a flex tree has runtime writers the
// shape cannot see (StructUtil.setpath, walk mutation), so the claim is
// a bound a guard discharges, exactly the record-schema field rule
// (recordSchemaFieldReturns). A nested shape / disjunct join keeps its
// payload so chained reads narrow too; a dispatch-bearing or
// unrepresentable value keeps dynamic(Any).
func ShapeFieldRead(v Value) Value {
	if _, ok := StoreShapeOf(v); ok {
		return NewDynamicCarrierValue(v)
	}
	if IsDisjunct(v) {
		return NewDynamicCarrierValue(v)
	}
	ft := ValueType(v)
	if ft == nil || ft.ConformsTo(TFunction) || ft.ConformsTo(TFnDef) {
		return NewDynamicCarrier(TAny)
	}
	return NewDynamicCarrier(ft)
}
