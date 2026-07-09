package eng

import "testing"

// inClosureUnit — the closure-body args-projection gate's classifier.
// White-box: the nil/empty/inconsistent shapes must all report false
// (only a live, well-formed closure unit may decline the projection),
// and a closure-flagged innermost rec reports true.
func TestInClosureUnit(t *testing.T) {
	// nil receiver and empty open-unit stack: not inside any fn unit.
	var nilES *EmitState
	if nilES.inClosureUnit() {
		t.Errorf("nil EmitState: want false")
	}
	es := NewEmitState()
	if es.inClosureUnit() {
		t.Errorf("empty openUnitRecs: want false")
	}

	// Defensive: an out-of-range rec index (never produced by
	// StartFnCompile's paired push, but the guard keeps a future
	// misalignment from panicking) reports false rather than indexing.
	es.openUnitRecs = []int{5}
	if es.inClosureUnit() {
		t.Errorf("out-of-range rec index: want false")
	}
	es.openUnitRecs = []int{-1}
	if es.inClosureUnit() {
		t.Errorf("negative rec index: want false")
	}

	// A real fn unit (closure=false) does not decline; a closure unit does.
	es = NewEmitState()
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "f"})
	es.openUnitRecs = []int{0}
	if es.inClosureUnit() {
		t.Errorf("plain fn unit: want false")
	}
	es.fnRecs[0].closure = true
	if !es.inClosureUnit() {
		t.Errorf("closure unit: want true")
	}

	// Innermost wins: a fn unit nested inside a closure unit is NOT a
	// closure context (its args frame is its own), and vice versa.
	es.fnRecs = append(es.fnRecs, &fnUnitRec{name: "inner"})
	es.openUnitRecs = []int{0, 1}
	if es.inClosureUnit() {
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
