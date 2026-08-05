package eng

import (
	"strings"
	"testing"
)

// emit_dynscope_test.go pins the dynamic-scope emit/lowering arms the corpus
// rows don't reach: the fragment-tree bind detector's loop arm, the rescue's
// FnNameStack reader fallback, and lowerDynBind's promoted / literal-bake /
// non-inert arms.

func dsBindEvent(name string) emitEvent {
	return emitEvent{kind: evDynBind, dyn: &emitDynBind{name: name, srcSeq: -1}}
}

func TestEventsBindDynScopeWalksFragments(t *testing.T) {
	names := map[string]bool{"dsn": true}
	// Loop body fragment carries the bind.
	loopEv := emitEvent{kind: evLoop, loop: &emitLoop{
		body: &EmitFragment{events: []emitEvent{dsBindEvent("dsn")}},
	}}
	if !eventsBindDynScope([]emitEvent{loopEv}, names) {
		t.Error("a bind inside a loop body fragment must be detected")
	}
	// Branch arm fragment carries it.
	brEv := emitEvent{kind: evBranch, br: &emitBranch{
		els: &EmitFragment{events: []emitEvent{dsBindEvent("dsn")}},
	}}
	if !eventsBindDynScope([]emitEvent{brEv}, names) {
		t.Error("a bind inside a branch arm fragment must be detected")
	}
	// A bind of an UNREAD name is invisible.
	if eventsBindDynScope([]emitEvent{dsBindEvent("other")}, names) {
		t.Error("a bind of a name no lookup reads must not be detected")
	}
	if eventsBindDynScope([]emitEvent{loopEv}, nil) {
		t.Error("an empty name set detects nothing")
	}
}

func TestDynScopeRescueReaderFallback(t *testing.T) {
	r := seam7Reg(t)
	r.Check.Mode = true
	// Binder model: fn "binder" binds "dsv" and reaches fn "reader".
	r.Check.FnBinders = map[string]map[string]bool{"dsv": {"binder": true}}
	r.Check.FnCallGraph = map[string]map[string]bool{"binder": {"reader": true}}
	es := NewEmitState()
	es.bindRegistry(r)
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}},
		&emitUnit{localByID: map[string]int{}})
	// unitNames stays EMPTY (the fallback under test); the check's
	// FnNameStack carries the reader.
	r.Check.FnNameStack = []string{"reader"}
	v := NewCarrier(TAny)
	es.NoteDefRead(v.ID, "dsv")
	op, ok := es.dynScopeRescue(v)
	if !ok || op.kind != opDynScope {
		t.Fatalf("rescue via FnNameStack fallback: op=%+v ok=%v", op, ok)
	}
	if !es.dynScopeNames["dsv"] {
		t.Error("the rescued name must join dynScopeNames")
	}
}

func TestLowerDynBindArms(t *testing.T) {
	names := map[string]bool{"dsl": true}
	newLW := func(es *EmitState, promoted map[int]int) (*lowerer, *CompiledFn) {
		cf := &CompiledFn{}
		return &lowerer{es: es, p: &Program{}, code: &cf.Code, debug: &cf.Debug,
			sigIdx: map[*Signature]int{}, variadic: map[int]bool{}, promoted: promoted}, cf
	}
	es := NewEmitState()
	es.dynScopeNames = names

	// Promoted computed value: pushes the promoted local, then the bind.
	lw, cf := newLW(es, map[int]int{7: 3})
	ev := emitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: 7}}
	if reason := lw.lowerDynBind(&ev); reason != "" {
		t.Fatalf("promoted bind refused: %s", reason)
	}
	if len(cf.Code) != 2 || cf.Code[0].Op != OpPushLocal || cf.Code[0].Arg != 3 || cf.Code[1].Op != OpBindDynScope {
		t.Errorf("promoted bind code = %v, want PUSH_LOCAL 3 + BIND_DYN_SCOPE", cf.Code)
	}

	// Unpromoted computed value: refuses.
	lw, _ = newLW(es, map[int]int{})
	ev = emitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: 9}}
	if reason := lw.lowerDynBind(&ev); !strings.Contains(reason, "unpromoted computed value") {
		t.Errorf("unpromoted bind reason = %q", reason)
	}

	// Literal binding: bakes the recorded value verbatim (unpooled).
	lw, cf = newLW(es, nil)
	ev = emitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: -1, val: NewInteger(5)}}
	if reason := lw.lowerDynBind(&ev); reason != "" {
		t.Fatalf("literal bind refused: %s", reason)
	}
	if len(cf.Code) != 2 || cf.Code[0].Op != OpPushConst || cf.Code[1].Op != OpBindDynScope {
		t.Errorf("literal bind code = %v, want PUSH_CONST + BIND_DYN_SCOPE", cf.Code)
	}

	// A non-inert (tape-coupled) value refuses.
	lw, _ = newLW(es, nil)
	ev = emitEvent{kind: evDynBind, dyn: &emitDynBind{name: "dsl", srcSeq: -1, val: NewWord("live")}}
	if reason := lw.lowerDynBind(&ev); !strings.Contains(reason, "unknown provenance") {
		t.Errorf("non-inert bind reason = %q", reason)
	}

	// A name no lookup reads lowers to nothing.
	lw, cf = newLW(es, nil)
	ev = emitEvent{kind: evDynBind, dyn: &emitDynBind{name: "unread", srcSeq: -1}}
	if reason := lw.lowerDynBind(&ev); reason != "" || len(cf.Code) != 0 {
		t.Errorf("unread bind must be a no-op: reason=%q code=%v", reason, cf.Code)
	}
}

// The active-token MAP const gate: inside a compiled fn frame a map operand
// bearing a word (autoEvalMap re-evaluates it per dispatch against the LIVE
// frame) must not bake — it routes to the dyn-scope rescue and, with no read
// provenance, refuses. Lists stay bakeable (code-as-data), and module scope
// keeps the map bake.
func TestActiveTokenMapConstGate(t *testing.T) {
	r := seam7Reg(t)
	// Begin (not a bare Mode write) so the values this test mints carry
	// compile identities — runtime mints elide IDs (value.go
	// checkPassDepth), and an identity-less compound is deliberately NOT
	// bakeable inside a fn unit.
	defer r.Check.Begin()()
	mkMap := func() Value {
		om := NewOrderedMap()
		om.Set("line", NewWord("src"))
		return NewMap(om)
	}
	es := NewEmitState()
	es.bindRegistry(r)
	// NewEmitState seeds the module unit; one more puts us inside a fn unit.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}})
	if _, ok := es.resolveOperand(mkMap()); ok {
		t.Error("an active-token map const must not bake inside a fn unit")
	}
	lst := NewList([]Value{NewWord("dup"), NewWord("mul")})
	if _, ok := es.resolveOperand(lst); !ok {
		t.Error("a code-as-data LIST const must stay bakeable inside a fn unit")
	}
	// Module scope (the seeded single unit) keeps the map bake.
	es2 := NewEmitState()
	es2.bindRegistry(r)
	if _, ok := es2.resolveOperand(mkMap()); !ok {
		t.Error("module scope must keep the active-token map bake")
	}
}

func TestBearsActiveTokensArms(t *testing.T) {
	if !BearsActiveTokens(NewList([]Value{NewInteger(1), NewWord("w")})) {
		t.Error("a list member word must count as an active token")
	}
	if BearsActiveTokens(NewMap(nil)) {
		t.Error("a nil-backed map bears nothing")
	}
	om := NewOrderedMap()
	om.Set("k", NewInteger(2))
	if BearsActiveTokens(NewMap(om)) {
		t.Error("an all-data map bears no active tokens")
	}
}
