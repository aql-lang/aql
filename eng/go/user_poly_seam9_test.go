package eng

import "testing"

// W9 user_poly.go coverage: the refusal gates of the user-fn poly compile,
// driven by direct calls to the pure helpers (userPolyArmShapeOK,
// findOwningFnDef) and by tryCompileUserPolyArms / compileUserPolyArm with
// crafted signatures. Every gate keeps compile == interpret, so each arm
// exercises a "keep the refusal" path.

func TestW9UserPolyArmShapeOK(t *testing.T) {
	body := AQL([]Value{NewWord("p")})

	// Empty body → false.
	if userPolyArmShapeOK(&Signature{}, nil) {
		t.Error("empty body should refuse")
	}
	// Quote/type/form arg slots → false.
	if userPolyArmShapeOK(&Signature{Impl: body, QuoteArgs: map[int]bool{0: true}}, nil) {
		t.Error("a QuoteArgs slot should refuse")
	}
	// A quoted param → false.
	if userPolyArmShapeOK(&Signature{
		Impl:   body,
		Params: []FnParam{{Name: "p", Quote: true}},
	}, []*Type{TInteger}) {
		t.Error("a quoted param should refuse")
	}
	// Returns length mismatch → false.
	if userPolyArmShapeOK(&Signature{
		Impl:    body,
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{TInteger, TInteger},
	}, []*Type{TInteger}) {
		t.Error("a Returns arity mismatch should refuse")
	}
	// nil/non-nil Returns mismatch → false.
	if userPolyArmShapeOK(&Signature{
		Impl:    body,
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{nil},
	}, []*Type{TInteger}) {
		t.Error("a nil vs concrete Returns slot should refuse")
	}
	// Matching shape → true.
	if !userPolyArmShapeOK(&Signature{
		Impl:    body,
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{TInteger},
	}, []*Type{TInteger}) {
		t.Error("a matching arm shape should pass")
	}
}

func TestW9FindOwningFnDef(t *testing.T) {
	r := newTestRegistry(t)
	// nil impl → not found.
	if _, ok := findOwningFnDef(r, "w", nil); ok {
		t.Error("nil impl must not resolve an owner")
	}
	// A non-FnDefInfo binding above the word: the scan skips it (continue)
	// and, with no fn entry present, returns not-found.
	r.Defs.Push("w9poly", NewInteger(7))
	impl := AQL([]Value{NewWord("x")})
	if _, ok := findOwningFnDef(r, "w9poly", impl); ok {
		t.Error("a non-fn binding must be skipped and yield not-found")
	}
}

func TestW9TryCompileUserPolyEarlyReturns(t *testing.T) {
	r := newTestRegistry(t)
	args := []Value{NewInteger(1)}

	// The early-return guard: an inactive recorder OR empty args refuses
	// before any lookup.
	if tryCompileUserPolyArms(r, inactiveEmitState(), "w", args, []*Type{TInteger}) != nil {
		t.Error("inactive recorder should refuse")
	}
	if tryCompileUserPolyArms(r, NewEmitState(), "w", nil, []*Type{TInteger}) != nil {
		t.Error("empty args should refuse")
	}
	// Empty committedReturns is ADMITTED (REFUSAL-CLOSURE.0 §6a) — an
	// unknown word still refuses through the agg==nil gate.
	if tryCompileUserPolyArms(r, NewEmitState(), "w", args, nil) != nil {
		t.Error("unknown word should refuse (zero-return contract is admitted)")
	}
	// Unknown word → agg==nil → nil.
	if tryCompileUserPolyArms(r, NewEmitState(), "w9unknown", args, []*Type{TInteger}) != nil {
		t.Error("unknown word should refuse")
	}
	// A single same-arity overload → len(sigIdx) < 2 → nil.
	InstallFnDef(r, "w9one", FnDefInfo{
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "n", Type: TInteger}},
			Returns: []*Type{TInteger},
			Impl:    AQL([]Value{NewWord("n")}),
		}},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9one", args, []*Type{TInteger}) != nil {
		t.Error("a single overload should refuse (single-overload path handles it)")
	}
}

func TestW9TryCompileUserPolyOwnerRefusal(t *testing.T) {
	r := newTestRegistry(t)
	// Two same-arity overloads whose owning def is a MACRO: the owner gate
	// (owner.Macro) refuses, keeping the interpreter in charge.
	InstallFnDef(r, "w9macro", FnDefInfo{
		Macro: true,
		Signatures: []Signature{
			{Params: []FnParam{{Name: "a", Type: TInteger}}, Returns: []*Type{TInteger}, Impl: AQL([]Value{NewWord("a")})},
			{Params: []FnParam{{Name: "b", Type: TInteger}}, Returns: []*Type{TInteger}, Impl: AQL([]Value{NewWord("b")})},
		},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9macro", []Value{NewInteger(1)}, []*Type{TInteger}) != nil {
		t.Error("a macro-owned poly set should refuse")
	}
}

func TestW9TryCompileUserPolyArmCompileFails(t *testing.T) {
	r := newTestRegistry(t)
	// Two same-arity overloads that pass the shape/owner gates but whose
	// bodies are deferred-param-list residuals: compileUserPolyArm refuses,
	// so the whole poly set is kept in the interpreter.
	def := func(p string) Signature {
		inner := NewList([]Value{NewWord(p)})
		inner.Eval = true
		return Signature{
			Params:  []FnParam{{Name: p, Type: TInteger}},
			Returns: []*Type{TInteger},
			Impl:    AQL([]Value{inner}),
		}
	}
	InstallFnDef(r, "w9defer", FnDefInfo{
		Signatures: []Signature{def("a"), def("b")},
	})
	if tryCompileUserPolyArms(r, NewEmitState(), "w9defer", []Value{NewInteger(1)}, []*Type{TInteger}) != nil {
		t.Error("a poly set with an uncompilable arm should refuse")
	}
}

func TestW9UnitNetsZero(t *testing.T) {
	// Bounds guards: nil recorder / out-of-range unit → false.
	var nilES *EmitState
	if nilES.unitNetsZero(0) {
		t.Error("nil EmitState must report false")
	}
	es := NewEmitState()
	if es.unitNetsZero(-1) || es.unitNetsZero(0) {
		t.Error("out-of-range unit must report false")
	}
	// A 0-residual unit qualifies; a residual-carrying or variadic one does not.
	es.fnRecs = append(es.fnRecs,
		&fnUnitRec{},
		&fnUnitRec{outOps: []emitOperand{constOperand(0)}},
		&fnUnitRec{variadic: true},
	)
	if !es.unitNetsZero(0) {
		t.Error("an empty-residual unit must net zero")
	}
	if es.unitNetsZero(1) {
		t.Error("a residual-carrying unit must not net zero")
	}
	if es.unitNetsZero(2) {
		t.Error("a variadic unit must not net zero")
	}
	// The inactive recorder's stub answer.
	if (inactiveEmit{}).unitNetsZero(0) {
		t.Error("inactive recorder must report false")
	}
}

func TestW9CompileUserPolyArmRefusals(t *testing.T) {
	r := newTestRegistry(t)

	// Empty body → -1,false.
	if _, ok := compileUserPolyArm(r, NewEmitState(), "w", &Signature{}, FnDefInfo{}); ok {
		t.Error("empty body arm should refuse")
	}

	// A deferred-param-list body (eval list referencing a param) → -1,false.
	inner := NewList([]Value{NewWord("p")})
	inner.Eval = true
	deferredSig := &Signature{
		Params: []FnParam{{Name: "p", Type: TInteger}},
		Impl:   AQL([]Value{inner}),
	}
	if _, ok := compileUserPolyArm(r, NewEmitState(), "w", deferredSig, FnDefInfo{}); ok {
		t.Error("a deferred-param-list body should refuse")
	}

	// A valid body but an INACTIVE recorder → StartFnCompile fails → -1,false.
	plainSig := &Signature{
		Params:  []FnParam{{Name: "p", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    AQL([]Value{NewWord("p")}),
	}
	if _, ok := compileUserPolyArm(r, inactiveEmitState(), "w", plainSig, FnDefInfo{}); ok {
		t.Error("an inactive recorder should fail StartFnCompile")
	}
}
