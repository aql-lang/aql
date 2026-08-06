package eng

// Seam-6a wave B: direct in-package unit tests for the previously
// unreached guard / refusal arms in carrier.go (the check-mode compile
// helpers). Per design/TEST-SEAMS.10.md these are driven by direct calls
// with synthetic Values and, where a recorder gate applies, an armed
// EmitState (r.Check.Emit = NewEmitState()). No package globals are
// swapped; every test uses a fresh registry.

import (
	"math/big"
	"testing"
)

// armEmit arms a live bytecode recorder on r so es.Active() is true.
func armEmit(r *Registry) *EmitState {
	es := NewEmitState()
	r.Check.Emit = es
	return es
}

// --- joinedElementCarrier ---------------------------------------------------

func TestS6aJoinedElementCarrierNilMapPayload(t *testing.T) {
	m := NewMap(NewOrderedMap())
	m.Data = MapPayload{M: nil}
	if _, ok := joinedElementCarrier(m); ok {
		t.Error("nil-backed MapPayload must decline")
	}
}

func TestS6aJoinedElementCarrierNilParentElem(t *testing.T) {
	lst := NewList([]Value{NewInteger(1), {}})
	if _, ok := joinedElementCarrier(lst); ok {
		t.Error("a nil-Parent element must decline the join")
	}
}

// --- UnionCarrierForType ----------------------------------------------------

func TestS6aUnionCarrierForTypeNil(t *testing.T) {
	if _, ok := UnionCarrierForType(nil); ok {
		t.Error("UnionCarrierForType(nil) must decline")
	}
}

// --- DataListElemTypeFromValue ----------------------------------------------

func TestS6aDataListElemTypeMapMixedBranches(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", NewInteger(1))
	m.Set("b", NewList([]Value{NewInteger(2)}))
	m.Set("c", NewString("x")) // never reached: the walk breaks at Any
	if got := DataListElemTypeFromValue(NewMap(m)); !got.Equal(TAny) {
		t.Errorf("cross-branch map values: got %v, want Any", got)
	}
}

func TestS6aDataListElemTypeMapNilParents(t *testing.T) {
	m := NewOrderedMap()
	m.Set("a", Value{})
	if got := DataListElemTypeFromValue(NewMap(m)); !got.Equal(TAny) {
		t.Errorf("nil-Parent map values: got %v, want Any", got)
	}
}

// --- tryFoldScalarConst / concreteHandlerEval / scalarFoldOperand -----------

// statefulSig builds a Signature whose handler returns f(callCount).
func statefulSig(effect CompileEffect, f func(n int) []Value) *Signature {
	n := 0
	return &Signature{
		CompileEffect: effect,
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			n++
			return f(n), nil
		}),
	}
}

func TestS6aTryFoldScalarConstNondeterministicDeclines(t *testing.T) {
	r := newTestRegistry(t)
	sig := statefulSig(CompileScalarFold, func(n int) []Value {
		return []Value{NewInteger(int64(n))}
	})
	if _, ok := tryFoldScalarConst(r, sig, []Value{NewInteger(1)}); ok {
		t.Error("a nondeterministic handler must not const-fold")
	}
}

func TestS6aTryFoldScalarConstNonInertResultDeclines(t *testing.T) {
	r := newTestRegistry(t)
	sig := statefulSig(CompileScalarFold, func(int) []Value {
		return []Value{NewTypeLiteral(TInteger)} // bare type node: not inert
	})
	if _, ok := tryFoldScalarConst(r, sig, []Value{NewInteger(1)}); ok {
		t.Error("a non-inert-const result must not fold")
	}
	// Positive twin: a deterministic inert scalar folds.
	sig2 := statefulSig(CompileScalarFold, func(int) []Value {
		return []Value{NewInteger(7)}
	})
	folded, ok := tryFoldScalarConst(r, sig2, []Value{NewInteger(1)})
	if !ok {
		t.Fatal("deterministic inert const should fold")
	}
	if n, err := AsInteger(folded); err != nil || n != 7 {
		t.Errorf("folded = %v, want 7", folded)
	}
}

func TestS6aConcreteHandlerEvalNonConcreteResult(t *testing.T) {
	r := newTestRegistry(t)
	sig := &Signature{
		Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
			return []Value{NewCarrier(TInteger)}, nil
		}),
	}
	if _, ok := concreteHandlerEval(r, sig, nil); ok {
		t.Error("a carrier result is neither concrete nor a bare type node")
	}
}

func TestS6aScalarFoldOperandBigNumPayloads(t *testing.T) {
	if !ScalarFoldOperand(NewBigInteger(big.NewInt(5))) {
		t.Error("a BigInt payload is a scalar fold operand")
	}
	if ScalarFoldOperand(NewList([]Value{NewCarrier(TInteger)})) {
		t.Error("a carrier-bearing list is not a scalar fold operand")
	}
}

// --- isConcreteContainerReturn ----------------------------------------------

func TestS6aIsConcreteContainerReturn(t *testing.T) {
	if isConcreteContainerReturn(NewDynamicCarrier(TList)) {
		t.Error("dynamic values are not concrete container returns")
	}
	if isConcreteContainerReturn(Value{}) {
		t.Error("nil-Parent values are not concrete container returns")
	}
	if !isConcreteContainerReturn(NewCarrier(TList)) {
		t.Error("a strict List carrier is a concrete container return")
	}
}

// --- applyGradualContagion ----------------------------------------------------

func TestS6aGradualContagionStrictDedup(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.Strict = true
	pos := SrcPos{Row: 3, Col: 7}
	r.Check.Diagnostics = []CheckDiagnostic{{
		Code: "dynamic_dispatch", Word: "s6aword", Row: 3, Col: 7,
	}}
	out := applyGradualContagion(r, "s6aword",
		[]Value{NewDynamicCarrier(TAny)},
		[]Value{NewCarrier(TInteger)}, pos, false)
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("duplicate dynamic_dispatch diagnostic added: %d", len(r.Check.Diagnostics))
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Errorf("contagion should mark the result dynamic: %+v", out)
	}
}

func TestS6aGradualContagionZeroOutSingleValueSibling(t *testing.T) {
	r := newTestRegistry(t)
	r.RegisterNativeFunc(NativeFunc{
		Name: "s6aset1",
		Signatures: []Signature{{
			Args: []*Type{TAny},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(1)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	done := r.Check.Begin()
	defer done()
	out := applyGradualContagion(r, "s6aset1",
		[]Value{NewDynamicCarrier(TAny)}, nil, SrcPos{}, false)
	if len(out) != 1 || !out[0].Dynamic || !out[0].Parent.Equal(TInteger) {
		t.Errorf("0-out mutator over dynamic receiver should model one dynamic Integer: %+v", out)
	}
}

// --- tryFoldModuleConst -------------------------------------------------------

func TestS6aTryFoldModuleConstNondeterministicDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	// A real Ideal/Module descriptor — isModuleFamilyValue now keys on
	// the kernel-declared TModule (Equal, not the retired string-path
	// probe), so a minted look-alike no longer reaches the fold.
	modVal := NewModuleInstance(ModuleDesc{ID: "s6am"})
	sig := statefulSig(CompileModuleFold, func(n int) []Value {
		return []Value{NewInteger(int64(n))}
	})
	outs := []Value{NewCarrier(TInteger)}
	if tryFoldModuleConst(r, "s6amfold", sig, []Value{modVal}, outs) {
		t.Error("a nondeterministic module read must not const-fold")
	}
}

// --- dynOutNativeOK -----------------------------------------------------------

func TestS6aDynOutNativeOKFnValuedArgDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := &Signature{Args: []*Type{TFunction}}
	args := []Value{NewInteger(1)}
	outs := []Value{NewDynamicCarrier(TAny)}
	if dynOutNativeOK(r, "s6adyn", sig, args, outs) {
		t.Error("a Function-typed arg slot must refuse the dyn-out bake")
	}
}

// --- isModuleInnerSig ---------------------------------------------------------

func TestS6aIsModuleInnerSigGuards(t *testing.T) {
	if isModuleInnerSig(nil, "w", nil) {
		t.Error("nil registry must decline")
	}

	r := newTestRegistry(t)
	sub := newTestRegistry(t)
	em := NewOrderedMap()
	em.Set("s6aw", NewValueRaw(TFunction, FnDefInfo{Registry: sub, Name: "s6aw"}))
	r.Modules.MarkLoaded("s6amod", ModuleDesc{
		ID: "s6amod",
		Exports: map[string]*OrderedMap{
			"nil-entry": nil, // the em == nil continue arm
			"real":      em,  // inner Lookup(name) == nil continue arm
		},
	})
	if isModuleInnerSig(r, "s6aw", &Signature{}) {
		t.Error("an export whose sub-registry lacks the word must decline")
	}
}

// --- tryRecordPoly ------------------------------------------------------------

func TestS6aTryRecordPolyQuotedNonGetDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(NativeFunc{
		Name: "s6apolyq",
		Signatures: []Signature{{
			Args:      []*Type{TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyq").Signatures[0]
	if tryRecordPoly(r, "s6apolyq", sig, []Value{NewAtom("k")}, []Value{}, SrcPos{}, true, nil, false, nil) {
		t.Error("a quoted-operand word other than get/getr/set/del must not poly")
	}
}

// TestS6aTryRecordPolyQuotedDelAdmits is the POSITIVE twin of the decline
// test above: `del` sits in the get/getr/set exemption family (one QuoteArg
// — the inert Atom key — baked as a const operand; an unshadowable builtin
// mutator), so an identically-shaped sig REGISTERED UNDER THE NAME `del`
// must pass the quoted-operand gate and record the poly. The pairing pins
// the admitted arm the same way the decline test pins the refused arm — a
// regression that drops del from the exemption fails here, not silently in
// a compiled corpus somewhere.
func TestS6aTryRecordPolyQuotedDelAdmits(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(NativeFunc{
		Name: "del",
		Signatures: []Signature{{
			Args:      []*Type{TAtom},
			QuoteArgs: map[int]bool{0: true},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("del").Signatures[0]
	if !tryRecordPoly(r, "del", sig, []Value{NewAtom("k")}, []Value{}, SrcPos{}, true, nil, false, nil) {
		t.Error("del carries the get/getr/set quoted-operand exemption and must poly")
	}
}

func TestS6aTryRecordPolyFnValuedArgDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(NativeFunc{
		Name: "s6apolyf",
		Signatures: []Signature{{
			Args: []*Type{TFunction},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyf").Signatures[0]
	if tryRecordPoly(r, "s6apolyf", sig, []Value{NewInteger(1)}, []Value{}, SrcPos{}, true, nil, false, nil) {
		t.Error("a Function-typed arg slot must not poly")
	}
}

func TestS6aTryRecordPolyReservedLiteralNoBinding(t *testing.T) {
	// "true" is IsBuiltinWord (reserved literal) in every registry but has
	// no Lookup binding — the fn == nil arm.
	r := newTestRegistry(t)
	armEmit(r)
	if tryRecordPoly(r, "true", &Signature{}, nil, []Value{}, SrcPos{}, true, nil, false, nil) {
		t.Error("a reserved literal with no fn binding must not poly")
	}
}

// --- tryRecordFallback ---------------------------------------------------------

// registerIslandWord registers a word with the given compile effect and
// arg count and returns the REGISTERED sig pointer (identity matters).
func registerIslandWord(t *testing.T, r *Registry, name string, effect CompileEffect, argc int, barrier int) *Signature {
	t.Helper()
	args := make([]*Type, argc)
	for i := range args {
		args[i] = TAny
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: name,
		Signatures: []Signature{{
			Args:          args,
			CompileEffect: effect,
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(0)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: barrier,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration of %s: %v", name, err)
	}
	return &r.Lookup(name).Signatures[0]
}

func TestS6aTryRecordFallbackForeignSigDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	registerIslandWord(t, r, "s6afb0", CompileFallbackBody, 1, -1)
	foreign := &Signature{CompileEffect: CompileFallbackBody}
	outs := []Value{NewCarrier(TInteger)}
	if tryRecordFallback(r, "s6afb0", foreign, []Value{NewInteger(1)}, outs, SrcPos{}) {
		t.Error("a sig that is not the word's registered binding must decline")
	}
}

func TestS6aTryRecordFallbackTypeNodeAfterThreadedDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb1", CompileFallbackBody, 2, -1)
	args := []Value{NewCarrier(TInteger), NewTypeLiteral(TInteger)}
	outs := []Value{NewCarrier(TInteger)}
	if tryRecordFallback(r, "s6afb1", sig, args, outs, SrcPos{}) {
		t.Error("a type operand after a threaded arg must decline")
	}
}

func TestS6aTryRecordFallbackBakedAfterThreadedDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb2", CompileFallbackBody, 2, -1)
	args := []Value{NewCarrier(TInteger), NewInteger(5)}
	outs := []Value{NewCarrier(TInteger)}
	if tryRecordFallback(r, "s6afb2", sig, args, outs, SrcPos{}) {
		t.Error("a baked arg after a threaded arg must decline")
	}
}

func TestS6aTryRecordFallbackTwoThreadedDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb3", CompileFallbackBody, 2, -1)
	args := []Value{NewCarrier(TInteger), NewCarrier(TString)}
	outs := []Value{NewCarrier(TInteger)}
	if tryRecordFallback(r, "s6afb3", sig, args, outs, SrcPos{}) {
		t.Error("two threaded args must decline (multi-thread islands unsupported)")
	}
}

func TestS6aTryRecordFallbackBarrierClamp(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb4", CompileFallbackBody, 1, -1)
	sig.BarrierPos = 99 // out-of-range barrier clamps to TotalArgs
	args := []Value{NewInteger(5)}
	outs := []Value{NewCarrier(TInteger)}
	// All-baked, fallback-body word: forward-eligible constraint holds
	// (1 baked arg <= clamped barrier 1) — the attempt proceeds past the
	// clamp; whether the recorder accepts is not the point here.
	tryRecordFallback(r, "s6afb4", sig, args, outs, SrcPos{})
	if sig.BarrierPos != 99 {
		t.Error("the clamp must be local, not written back to the sig")
	}
}

func TestS6aTryRecordFallbackStackFormRebuild(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb5", CompileIslandPure, 2, 1)
	args := []Value{NewInteger(1), NewInteger(2)}
	outs := []Value{NewDynamicCarrier(TAny)} // dynamic out: island is legitimate
	// barrier 1 < len(args) 2 with all-baked args on a pure word: the span
	// is rebuilt in stack form (deepest stack arg first, then the word,
	// then the forward args).
	if !tryRecordFallback(r, "s6afb5", sig, args, outs, SrcPos{}) {
		t.Fatal("all-baked pure island in stack form should record")
	}
}

func TestS6aTryRecordFallbackBakedNonInertDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb6", CompileIslandPure, 1, -1)
	// A FlexList is CONCRETE (materialise bakes it) but MUTABLE, so
	// isInertConst rejects it: baking a mutable instance into the island's
	// const span would let a later mutation corrupt the frozen bake. The
	// non-inert-data arg branch declines, so the program falls back to the
	// interpreter unchanged. (An enclosing-scope binding read of such a value
	// no longer reaches here — resolveOperand routes it to a dynamic-scope
	// lookup — so this direct unit test pins the refusal contract.)
	args := []Value{NewFlexList([]Value{NewInteger(1)})}
	outs := []Value{NewDynamicCarrier(TAny)} // dynamic out: island is legitimate
	if tryRecordFallback(r, "s6afb6", sig, args, outs, SrcPos{}) {
		t.Error("a baked but non-inert (mutable) data arg must decline the island")
	}
}

// --- bodyFreeForFallback --------------------------------------------------------

func TestS6aBodyFreeForFallbackLiteralAndTypeWords(t *testing.T) {
	r := newTestRegistry(t)
	if !BodyFreeForFallback(r, NewList([]Value{NewWord("true"), NewWord("false")})) {
		t.Error("bare literal words are VM-resolvable")
	}
	if !BodyFreeForFallback(r, NewList([]Value{NewWord("Integer")})) {
		t.Error("type-name words are VM-resolvable")
	}
	if BodyFreeForFallback(r, NewList([]Value{NewWord("s6a_no_such")})) {
		t.Error("an unresolvable word must fail the body scan")
	}
}

// --- disjunctPartitionReturns ------------------------------------------------------

func strictDisjunct(alts ...Value) Value {
	d := NewDisjunct(alts)
	d.Carrier = true
	return d
}

func TestS6aDisjunctPartitionUnknownWordDeclines(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	d := strictDisjunct(NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	if _, ok := disjunctPartitionReturns(r, "s6a_unknown_word", []Value{d}, SrcPos{}); ok {
		t.Error("an unknown word must decline the partition")
	}
}

func TestS6aDisjunctPartitionComboCapDeclines(t *testing.T) {
	r := newTestRegistry(t)
	registerIslandWord(t, r, "s6apart", 0, 1, -1)
	done := r.Check.Begin()
	defer done()
	alts := make([]Value, disjunctPartitionCap+1)
	for i := range alts {
		alts[i] = NewInteger(int64(i))
	}
	if _, ok := disjunctPartitionReturns(r, "s6apart", []Value{strictDisjunct(alts...)}, SrcPos{}); ok {
		t.Error("a cross product over the cap must decline")
	}
}

func TestS6aDisjunctPartitionUnannotatedOverloadDeclines(t *testing.T) {
	r := newTestRegistry(t)
	// A word with NO Returns and NO ReturnsFn: the default arm declines.
	r.RegisterNativeFunc(NativeFunc{
		Name: "s6apartu",
		Signatures: []Signature{{
			Args: []*Type{TAny},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	done := r.Check.Begin()
	defer done()
	d := strictDisjunct(NewTypeLiteral(TInteger), NewTypeLiteral(TString))
	if _, ok := disjunctPartitionReturns(r, "s6apartu", []Value{d}, SrcPos{}); ok {
		t.Error("an unannotated overload must decline the whole partition")
	}
}

// --- disjunctCombos / joinReturnRows / dynamicReachableValueReturns ----------------

func TestS6aDisjunctCombosOverLimit(t *testing.T) {
	d := strictDisjunct(NewInteger(1), NewInteger(2), NewInteger(3), NewInteger(4))
	if _, ok := disjunctCombos([]Value{d}, 3); ok {
		t.Error("a product beyond the limit must decline")
	}
}

func TestS6aJoinReturnRowsArityMismatch(t *testing.T) {
	rows := [][]Value{
		{NewCarrier(TInteger)},
		{NewCarrier(TInteger), NewCarrier(TString)},
	}
	if _, ok := joinReturnRows(rows); ok {
		t.Error("mismatched return arity must decline")
	}
}

func TestS6aDynamicReachableValueReturnsUnknownWord(t *testing.T) {
	r := newTestRegistry(t)
	if got := dynamicReachableValueReturns(r, "s6a_missing", nil); got != nil {
		t.Errorf("unknown word: got %v, want nil", got)
	}
}

// --- narrowDynamicUses / slotIsPolymorphic -----------------------------------------

func TestS6aNarrowDynamicUsesGuards(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()

	// Unbound origin name: skipped.
	a := NewDynamicCarrier(TAny)
	a.SetDynFrom("s6a_unbound")
	sig := &Signature{Args: []*Type{TInteger}}
	narrowDynamicUses(r, "w", sig, []Value{a})
	if _, ok := r.Defs.Top("s6a_unbound"); ok {
		t.Error("an unbound origin must not gain a binding")
	}

	// Bound but no longer dynamic: skipped.
	r.Defs.Push("s6a_rebound", NewInteger(3))
	b := NewDynamicCarrier(TAny)
	b.SetDynFrom("s6a_rebound")
	narrowDynamicUses(r, "w", sig, []Value{b})
	if got, _ := r.Defs.Top("s6a_rebound"); got.Dynamic {
		t.Error("a since-rebound name must stay untouched")
	}

	// Nil slot type: skipped.
	r.Defs.Push("s6a_dyn", NewDynamicCarrier(TAny))
	c := NewDynamicCarrier(TAny)
	c.SetDynFrom("s6a_dyn")
	nilSlotSig := &Signature{Args: []*Type{nil}}
	depth := r.Defs.Depth("s6a_dyn")
	narrowDynamicUses(r, "w", nilSlotSig, []Value{c})
	if r.Defs.Depth("s6a_dyn") != depth {
		t.Error("a nil slot type must not push a narrowing")
	}
}

func TestS6aSlotIsPolymorphicEmptyWord(t *testing.T) {
	r := newTestRegistry(t)
	if slotIsPolymorphic(r, "", nil, 0, TInteger) {
		t.Error("an empty word is never polymorphic")
	}
}

// --- ReturnsFreshInstance -----------------------------------------------------------

func TestS6aReturnsFreshInstanceParameterlessSchemaNonConcreteData(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()
	fields.Set("x", NewTypeLiteral(TInteger))
	body := NewRecordType(fields)
	schemaT := r.Types.MintType("S6aRec", TIdeal)
	schema := NewTypeSchema(schemaT, &TypeSchemaInfo{Kind: SchemaRecord, Body: body})

	fn := ReturnsFreshInstance(0)
	out := fn([]Value{schema, NewCarrier(TMap)}, r)
	if len(out) != 1 {
		t.Fatalf("out len = %d, want 1", len(out))
	}
	if !out[0].Parent.Equal(TMap) || !out[0].Dynamic {
		t.Errorf("want a dynamic Map carrier riding the field schema, got %+v", out[0])
	}
	if _, ok := out[0].Data.(RecordTypeInfo); !ok {
		t.Errorf("want RecordTypeInfo payload, got %T", out[0].Data)
	}
}

// --- RecordTypedDefMake / objectMakeSig ---------------------------------------------

func TestS6aRecordTypedDefMakeGuards(t *testing.T) {
	if _, ok := RecordTypedDefMake(nil, Value{}, Value{}, SrcPos{}); ok {
		t.Error("nil registry must decline")
	}
	r := newTestRegistry(t)
	armEmit(r)
	// eng registries have no `make` word: objectMakeSig returns nil.
	if _, ok := RecordTypedDefMake(r, NewTypeLiteral(TInteger), NewMap(NewOrderedMap()), SrcPos{}); ok {
		t.Error("a registry without `make` must decline")
	}
}

func TestS6aObjectMakeSigNoMatchingOverload(t *testing.T) {
	r := newTestRegistry(t)
	if objectMakeSig(r) != nil {
		t.Error("no make word registered: want nil")
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "make",
		Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	if objectMakeSig(r) != nil {
		t.Error("make without an [Ideal Map] overload: want nil")
	}
}

// --- CommonAncestorType / isNoneArm -------------------------------------------------

func TestS6aCommonAncestorTypeNil(t *testing.T) {
	if got := CommonAncestorType(nil, TInteger); !got.Equal(TAny) {
		t.Errorf("nil operand: got %v, want Any", got)
	}
}

func TestS6aIsNoneArmZeroValue(t *testing.T) {
	if isNoneArm(Value{}) {
		t.Error("the zero Value is not a None arm")
	}
}

// --- joinCarriersInner width cap -----------------------------------------------------

func TestS6aJoinCarriersWidthCapCollapses(t *testing.T) {
	mk := func(lo, hi int64) Value {
		var alts []Value
		for i := lo; i <= hi; i++ {
			alts = append(alts, NewInteger(i))
		}
		return strictDisjunct(alts...)
	}
	a := mk(1, 5)
	b := mk(6, 10)
	got := joinCarriersInner(a, b)
	if !got.Carrier || !got.Parent.Equal(TInteger) || IsDisjunct(got) {
		t.Errorf("10 alternatives must collapse to a plain Integer carrier, got %+v", got)
	}
}

// --- joinCarriersInner None arms -----------------------------------------------------

// A None arm is a lattice root: NewTypeLiteral(TNone) has Parent==nil. The
// parent-math collapse blocks must not run against it — ConformsTo(nil) is a
// vacuous true, so `Integer.ConformsTo(None)` would collapse the merge to
// NewCarrier(None.Parent==nil), a Parent-less carrier the engine halts on when
// it steps an `if b [99] [None]` result (undefined stack entry). The join must
// instead yield a valid, non-nil-Parent carrier.
func TestS6aJoinCarriersNoneArm(t *testing.T) {
	none := NewTypeLiteral(TNone)
	if none.Parent != nil {
		t.Fatalf("precondition: None literal has a nil Parent, got %v", none.Parent)
	}

	// value-vs-None: Integer|None union, a valid disjunct carrier (never a
	// Parent-less carrier).
	got := joinCarriersInner(NewInteger(99), none)
	if got.Parent == nil {
		t.Fatalf("value-vs-None join must not produce a nil-Parent carrier: %+v", got)
	}
	if !IsDisjunct(got) {
		t.Errorf("Integer joined with None should be a disjunct carrier, got %+v", got)
	}

	// None-vs-value is symmetric.
	if g2 := joinCarriersInner(none, NewInteger(99)); g2.Parent == nil {
		t.Fatalf("None-vs-value join must not produce a nil-Parent carrier: %+v", g2)
	}

	// both-None collapses to a proper (non-nil-Parent) None carrier.
	both := joinCarriersInner(none, NewTypeLiteral(TNone))
	if both.Parent == nil || !both.Parent.Equal(TNone) || !both.Carrier {
		t.Errorf("None joined with None should be a None carrier, got %+v", both)
	}
}

// --- RunCarrierBodyWithDefs -----------------------------------------------------------

func TestS6aRunCarrierBodyErrorPath(t *testing.T) {
	r := newTestRegistry(t)
	body := NewList([]Value{NewWord("s6a_undefined_word")})
	stk, adds := RunCarrierBodyWithDefs(r, body)
	if stk != nil {
		t.Errorf("an erroring body yields a nil stack, got %v", stk)
	}
	if len(adds) != 0 {
		t.Errorf("an erroring body yields no def additions, got %v", adds)
	}
	found := false
	for _, d := range r.Check.Diagnostics {
		if d.Code == "branch_error" {
			found = true
		}
	}
	if !found {
		t.Error("expected a branch_error diagnostic")
	}
}

// --- extractGuardClauses ---------------------------------------------------------------

func TestS6aExtractGuardClausesShortGuardFact(t *testing.T) {
	r := newTestRegistry(t)
	cond := NewValueRaw(TBoolean, GuardFactInfo{Toks: []Value{NewWord("x")}})
	if got := extractGuardClauses(r, cond); got != nil {
		t.Errorf("a 1-token guard fact has no clauses, got %v", got)
	}
}

func TestS6aExtractGuardClausesBadWordPayload(t *testing.T) {
	r := newTestRegistry(t)
	badWord := NewValueRaw(TWord, nil) // Word-parented, no WordInfo payload
	cond := NewList([]Value{badWord, NewWord("is"), NewTypeLiteral(TInteger)})
	if got := extractGuardClauses(r, cond); got != nil {
		t.Errorf("an AsWord failure skips the triplet, got %v", got)
	}
}

// --- ApplyGuardNarrowing / ApplyComplementNarrowing -------------------------------------

func TestS6aApplyGuardNarrowingInactive(t *testing.T) {
	r := newTestRegistry(t)
	restore := ApplyGuardNarrowing(r, NewList([]Value{NewWord("x")}))
	restore() // noop
}

func TestS6aApplyGuardNarrowingRedundantGuardDedup(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Defs.Push("s6ax", NewCarrier(TInteger))
	cond := NewList([]Value{NewWord("s6ax"), NewWord("is"), NewTypeLiteral(TInteger)})

	restore := ApplyGuardNarrowing(r, cond)
	restore()
	n1 := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "redundant_guard" {
			n1++
		}
	}
	if n1 != 1 {
		t.Fatalf("first narrowing: %d redundant_guard diags, want 1", n1)
	}

	restore = ApplyGuardNarrowing(r, cond) // second: dedup arm
	restore()
	n2 := 0
	for _, d := range r.Check.Diagnostics {
		if d.Code == "redundant_guard" {
			n2++
		}
	}
	if n2 != 1 {
		t.Errorf("second narrowing must dedup, got %d diags", n2)
	}
}

func TestS6aApplyComplementNarrowingGuards(t *testing.T) {
	// Inactive check state: noop.
	r := newTestRegistry(t)
	restore := ApplyComplementNarrowing(r, NewList([]Value{NewWord("x")}))
	restore()

	// Active, but the clause's name is unbound: skipped.
	done := r.Check.Begin()
	defer done()
	cond := NewList([]Value{NewWord("s6a_nobind"), NewWord("is"), NewTypeLiteral(TInteger)})
	restore = ApplyComplementNarrowing(r, cond)
	restore()
	if _, ok := r.Defs.Top("s6a_nobind"); ok {
		t.Error("an unbound clause name must stay unbound")
	}
}

// --- refineRecursiveSummary ---------------------------------------------------------------

func TestS6aRefineRecursiveSummaryJoinsRounds(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.FnSummaries = map[string][]Value{}
	result := []Value{NewCarrier(TInteger)}
	got := refineRecursiveSummary(r, "s6akey", 0, result, func() []Value {
		return []Value{NewCarrier(TString)}
	})
	if len(got) != 1 {
		t.Fatalf("summary len = %d, want 1", len(got))
	}
	// The refined hypothesis must cover BOTH rounds' types.
	if got[0].Parent.Equal(TInteger) && !IsDisjunct(got[0]) {
		t.Errorf("summary did not widen past round 1: %+v", got[0])
	}
}

// --- runFnBodyOnce / AnalyseFnBody -----------------------------------------------------------

func TestS6aRunFnBodyOnceErrorMarksUncompilable(t *testing.T) {
	r := newTestRegistry(t)
	es := armEmit(r)
	got := runFnBodyOnce(r, "s6afn", nil, []Value{NewWord("s6a_undef_body_word")}, nil, nil, false)
	if got != nil {
		t.Errorf("an erroring body returns nil, got %v", got)
	}
	if es.Compilable {
		t.Error("an erroring body under an armed recorder must mark the program uncompilable")
	}
}

func TestS6aAnalyseFnBodyInflightBailArmedRecorder(t *testing.T) {
	r := newTestRegistry(t)
	es := armEmit(r)
	es.fnArm = true // keep recording live through the fn-body guard
	body := []Value{NewWord("s6a_rec_body")}
	key := FnAnalysisKey(r.AnalysisScopeID(), "s6arec", nil, nil, body)
	r.Check.FnInflight = map[string]bool{key: true}
	out := AnalyseFnBody(r, "s6arec", nil, body, nil, nil, nil, false)
	if len(out) != 1 || !out[0].Carrier || !out[0].Parent.Equal(TAny) || out[0].Dynamic {
		t.Errorf("armed in-flight bail must be a strict Any carrier, got %+v", out)
	}
}

// --- isDeferredWordList ------------------------------------------------------------------------

func TestS6aIsDeferredWordListShapes(t *testing.T) {
	// Parent=TList with a non-list payload: AsList fails.
	bad := NewValueRaw(TList, IntPayload{N: 1})
	bad.Eval = true
	if IsDeferredWordList(bad) {
		t.Error("a non-list payload must decline")
	}

	// A nested list containing a word: deferred.
	inner := NewList([]Value{NewWord("w")})
	inner.Eval = true
	outer := NewList([]Value{inner})
	outer.Eval = true
	if !IsDeferredWordList(outer) {
		t.Error("a nested word-bearing list is deferred")
	}

	// No words anywhere: not deferred.
	plain := NewList([]Value{NewInteger(1), NewList([]Value{NewInteger(2)})})
	plain.Eval = true
	if IsDeferredWordList(plain) {
		t.Error("a word-free list is not deferred")
	}
}

// A CONCRETE fn-valued arg (FnDefInfo in .Data) under a NON-Function sig slot
// reaches the args-data guard (past the ArgTypes-Function check) and declines
// the dyn-out bake / the poly record.
func TestS6aDynOutNativeOKFnValueDataDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := &Signature{Args: []*Type{TAny}}
	args := []Value{NewValueRaw(TFunction, FnDefInfo{Name: "s6afn"})}
	outs := []Value{NewDynamicCarrier(TAny)}
	if dynOutNativeOK(r, "s6adynv", sig, args, outs) {
		t.Error("a concrete fn-valued arg must refuse the dyn-out bake")
	}
}

func TestS6aTryRecordPolyFnValueDataDeclines(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	r.RegisterNativeFunc(NativeFunc{
		Name: "s6apolyv",
		Signatures: []Signature{
			{Args: []*Type{TAny}, Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil }), Returns: []*Type{TAny}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return nil, nil }), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	})
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	sig := &r.Lookup("s6apolyv").Signatures[0]
	args := []Value{NewValueRaw(TFunction, FnDefInfo{Name: "s6afn2"})}
	if tryRecordPoly(r, "s6apolyv", sig, args, []Value{NewDynamicCarrier(TAny)}, SrcPos{}, false, nil, false, nil) {
		t.Error("a concrete fn-valued arg must not poly")
	}
}
