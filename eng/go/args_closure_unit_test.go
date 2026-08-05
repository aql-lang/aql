package eng

import "testing"

// inClosureUnit — the closure-body args-projection gate's classifier.
// White-box: the nil/empty/inconsistent shapes must all report false
// (only a live, well-formed closure unit may decline the projection),
// and a closure-flagged innermost rec reports true.
func TestInClosureUnit(t *testing.T) {
	// nil receiver and empty open-unit stack: not inside any fn unit.
	var nilES *EmitState
	if nilES.InClosureUnit() {
		t.Errorf("nil EmitState: want false")
	}
	es := NewEmitState()
	if es.InClosureUnit() {
		t.Errorf("empty openUnitRecs: want false")
	}

	// Defensive: an out-of-range rec index (never produced by
	// StartFnCompile's paired push, but the guard keeps a future
	// misalignment from panicking) reports false rather than indexing.
	es.openUnitRecs = []int{5}
	if es.InClosureUnit() {
		t.Errorf("out-of-range rec index: want false")
	}
	es.openUnitRecs = []int{-1}
	if es.InClosureUnit() {
		t.Errorf("negative rec index: want false")
	}

	// A real fn unit (closure=false) does not decline; a closure unit does.
	es = NewEmitState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "f"})
	es.openUnitRecs = []int{0}
	if es.InClosureUnit() {
		t.Errorf("plain fn unit: want false")
	}
	es.fnRecs[0].closure = true
	if !es.InClosureUnit() {
		t.Errorf("closure unit: want true")
	}

	// Innermost wins: a fn unit nested inside a closure unit is NOT a
	// closure context (its args frame is its own), and vice versa.
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "inner"})
	es.openUnitRecs = []int{0, 1}
	if es.InClosureUnit() {
		t.Errorf("fn unit nested in closure: want false (innermost wins)")
	}
}

// bodyFreeForFallback must screen out the context-dependent words
// (args / __pa): an island's sub-engine inside a compiled fn sees an
// EMPTY interpreter args stack where the interpreter sees the enclosing
// call's list. Both spellings decline; an ordinary registered-word body
// stays free (the positive pair).
func TestBodyFreeForFallbackContextWords(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for _, name := range []string{"args", "__pa"} {
		body := NewList([]Value{NewWord(name)})
		if bodyFreeForFallback(r, body) {
			t.Errorf("body [%s]: want island-decline (context-dependent word)", name)
		}
	}
	// Positive pair: a body of known literals stays island-free.
	free := NewList([]Value{NewWord("true")})
	if !bodyFreeForFallback(r, free) {
		t.Errorf("body [true]: want island-free")
	}
}

// closureResidualExact / stripResidualShapeOK — the whole-residual and
// strip-input closure screens' classifier branches. White-box: defensive
// shapes report false (never compile a unit whose runtime count could
// diverge); the admitted shapes report true.
func TestClosureResidualScreens(t *testing.T) {
	// Defensive: nil state / out-of-range unit.
	if closureResidualExact(nil, 0, 1) {
		t.Errorf("nil EmitState: want false")
	}
	if stripResidualShapeOK(nil, 0, 1) {
		t.Errorf("nil EmitState: want false")
	}
	es := NewEmitState()
	if closureResidualExact(es, 3, 1) || stripResidualShapeOK(es, 3, 1) {
		t.Errorf("out-of-range unit: want false")
	}

	// Exactness: count match required; variadic / dynamic tails decline.
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "u", outOps: []emitOperand{{kind: opConst}, {kind: opConst}}})
	if !closureResidualExact(es, 0, 2) {
		t.Errorf("2-op residual, want=2: want true")
	}
	if closureResidualExact(es, 0, 3) {
		t.Errorf("2-op residual, want=3: want false (count mismatch)")
	}
	es.fnRecs[0].variadic = true
	if closureResidualExact(es, 0, 2) {
		t.Errorf("variadic residual: want false")
	}

	// Strip shapes: 1 op admits; 2 ops admit only with the input (param
	// local 0) at the bottom; deeper/variadic/dyn shapes decline.
	rec := &fnUnitRec{name: "s", outOps: []emitOperand{{kind: opConst}}}
	es.fnRecs = append(es.fnRecs, rec)
	if !stripResidualShapeOK(es, 1, 1) {
		t.Errorf("1-op residual: want true")
	}
	if stripResidualShapeOK(es, 1, 0) {
		t.Errorf("1-op residual with want=0: want false (count disagreement)")
	}
	rec.outOps = []emitOperand{{kind: opLocal, idx: 0}, {kind: opConst}}
	if !stripResidualShapeOK(es, 1, 1) {
		t.Errorf("2-op residual with input bottom: want true")
	}
	rec.outOps = []emitOperand{{kind: opConst}, {kind: opConst}}
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("2-op residual with non-input bottom: want false")
	}
	rec.outOps = []emitOperand{{kind: opLocal, idx: 0}, {kind: opConst}, {kind: opConst}}
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("3-op residual: want false")
	}
	rec.outOps = nil
	if !stripResidualShapeOK(es, 1, 0) {
		t.Errorf("empty residual with want=0: want true (the zero-netting handler)")
	}
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("empty residual with want=1: want false")
	}
	rec.outOps = []emitOperand{{kind: opConst}}
	rec.dynTrailArity = 2
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("dynTrail residual: want false")
	}
	rec.dynTrailArity = 0
	rec.variadic = true
	if stripResidualShapeOK(es, 1, 1) {
		t.Errorf("variadic strip residual: want false")
	}
}
