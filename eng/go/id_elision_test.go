package eng

// Tests for the mode-gated Value-ID elision (design/
// INTERPRETER-PYTHON-PARITY.10.md Phase B / F2): runtime concrete mints
// carry no ID, any live check/compile pass (or an explicit mint scope)
// re-arms minting process-wide, and the emit layer treats an empty ID as
// "no identity" — skip / refuse / rescue, never a map key.

import "testing"

func TestIDElisionMatrix(t *testing.T) {
	// Runtime concrete mint: elided.
	if CheckPassActive() {
		t.Fatal("no pass should be live at test entry")
	}
	if v := NewInteger(7); v.ID != "" {
		t.Errorf("runtime concrete mint carries ID %q, want elided", v.ID)
	}
	// Bare lattice values always mint: type literal and carrier.
	if v := NewTypeLiteral(TInteger); v.ID == "" {
		t.Error("type literal must always carry an ID")
	}
	if v := NewCarrier(TList); v.ID == "" {
		t.Error("carrier must always carry an ID (RegisterLocal keys on it)")
	}

	// Inside a pass: minted.
	c := &CheckState{}
	done := c.Begin()
	if !CheckPassActive() {
		t.Error("CheckPassActive must report a live pass")
	}
	if v := NewInteger(7); v.ID == "" {
		t.Error("mint inside a pass must carry an ID")
	}
	// Nested pass: still minted after the INNER done.
	c2 := &CheckState{}
	done2 := c2.Begin()
	done2()
	if v := NewInteger(7); v.ID == "" {
		t.Error("outer pass still live — mint must carry an ID")
	}
	// done called TWICE must not drive the counter negative: after the
	// single outer done the counter is zero…
	done2()
	done()
	if v := NewInteger(7); v.ID != "" {
		t.Errorf("all passes ended — mint carries ID %q, want elided", v.ID)
	}
	// …and a later legitimate pass still mints (the double-dec guard).
	c3 := &CheckState{}
	done3 := c3.Begin()
	if v := NewInteger(7); v.ID == "" {
		t.Error("later pass after a double-done must still mint IDs")
	}
	done3()

	// Explicit mint scope (the parser's arm).
	end := BeginIDMintScope()
	if v := NewString("x"); v.ID == "" {
		t.Error("mint scope must arm ID minting")
	}
	end()
	end() // second call is a no-op
	if v := NewString("x"); v.ID != "" {
		t.Errorf("after scope end mint carries ID %q, want elided", v.ID)
	}
	// nil CheckState: Begin is a no-op closure, counter untouched.
	var nilC *CheckState
	nilDone := nilC.Begin()
	nilDone()
}

func TestEmitEmptyIDGuards(t *testing.T) {
	es := NewEmitState()

	// setProducedAt with an identity-less value: no entry — a later ""
	// lookup is a guaranteed miss, and no other value can false-hit it.
	blank := NewInteger(1) // runtime mint → "" ID
	if blank.ID != "" {
		t.Fatal("test premise: runtime mint should be identity-less")
	}
	es.setProducedAt(blank, 3, 0)
	if _, ok := es.producedBy[""]; ok {
		t.Error("setProducedAt keyed the provenance map on \"\"")
	}
	// Positive twin: a minted value registers normally.
	end := BeginIDMintScope()
	minted := NewInteger(2)
	end()
	es.setProducedAt(minted, 4, 0)
	if pr, ok := es.producedBy[minted.ID]; !ok || pr.seq != 4 {
		t.Error("minted value did not register in producedBy")
	}

	// RegisterLocal: "" refuses with -1 and never inserts; two distinct
	// identity-less values must NOT collapse onto one slot.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}})
	if slot := es.RegisterLocal(""); slot != -1 {
		t.Errorf("RegisterLocal(\"\") = %d, want -1", slot)
	}
	if slot := es.RegisterLocal(""); slot != -1 {
		t.Errorf("second RegisterLocal(\"\") = %d, want -1", slot)
	}
	if _, ok := es.units[len(es.units)-1].localByID[""]; ok {
		t.Error("RegisterLocal inserted a \"\" slot")
	}
	// Positive twin: real IDs get distinct slots.
	a := es.RegisterLocal("S_aaaaaaaaaaaa")
	b := es.RegisterLocal("S_bbbbbbbbbbbb")
	if a == -1 || b == -1 || a == b {
		t.Errorf("real IDs got slots %d/%d, want distinct non-negative", a, b)
	}
}

// TestResolveOperandIdentitylessCompoundRescued pins the fn-unit rescue:
// a compound with no compile identity (a runtime mint) must route to
// dynScopeRescue — never into the ID-keyed freshen/share machinery —
// while the same compound WITH an identity resolves normally.
func TestResolveOperandIdentitylessCompoundRescued(t *testing.T) {
	r := seam7Reg(t)
	defer r.Check.Begin()()
	es := NewEmitState()
	es.bindRegistry(r)
	// One extra unit puts resolution inside a compiled fn frame.
	es.units = append(es.units, &emitUnit{localByID: map[string]int{}, capID: map[string]bool{}})

	blank := Value{Parent: TList, Data: ListPayload{Elems: []Value{NewInteger(1)}}} // no ID
	if _, ok := es.resolveOperand(blank); ok {
		t.Error("identity-less compound inside a fn unit must be rescued, not placed")
	}
	// Positive twin: the same compound minted with an identity resolves.
	minted := NewList([]Value{NewInteger(1)})
	if minted.ID == "" {
		t.Fatal("test premise: pass-minted list should carry an ID")
	}
	if _, ok := es.resolveOperand(minted); !ok {
		t.Error("pass-minted compound must still resolve inside a fn unit")
	}
}

func TestFnCompileRefusesIdentitylessCapture(t *testing.T) {
	// A closure whose capture snapshot is a runtime mint (no ID) cannot
	// be compiled: capture slots are positional, so skipping one would
	// misalign every later capture. The unit must refuse (conservative
	// interpreter fallback), never collapse slots.
	c := &CheckState{}
	defer c.BeginCompilePass()()
	es, ok := c.Emit.(*EmitState)
	if !ok {
		t.Fatal("BeginCompilePass did not install an EmitState")
	}
	blankCap := Value{Parent: TInteger, Data: IntPayload{N: 5}} // hand-built: no ID
	_, fin, started := es.StartFnCompile("k", "f", nil, nil, nil, nil,
		[]CapturedBinding{{Name: "x", Value: blankCap}}, false, SrcPos{})
	if started && fin != nil {
		fin(nil)
	}
	if es.Compilable {
		t.Error("identity-less capture must mark the program uncompilable")
	}
}

func TestSameFnConstruction(t *testing.T) {
	sigs := []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}}}
	orig := NewFunction(FnDefInfo{Signatures: sigs})
	cpy := orig // by-value copy shares the Signatures backing array
	if !sameFnConstruction(orig, cpy) {
		t.Error("copies of one construction must match")
	}
	other := NewFunction(FnDefInfo{Signatures: []Signature{{Params: []FnParam{{Name: "n", Type: TInteger}}}}})
	if sameFnConstruction(orig, other) {
		t.Error("distinct constructions must not match")
	}
	if sameFnConstruction(orig, NewInteger(1)) {
		t.Error("non-fn operand must not match")
	}
	// ID fallback arm: same ID, rebuilt (non-shared) sig backing.
	end := BeginIDMintScope()
	x := NewFunction(FnDefInfo{Signatures: []Signature{{}}})
	end()
	y := x
	y.Data = FnDefInfo{Signatures: append([]Signature(nil), x.Data.(FnDefInfo).Signatures...)}
	if !sameFnConstruction(x, y) {
		t.Error("same-ID rebuilt copy must match via the ID fallback")
	}
}
