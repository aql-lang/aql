package eng

import "testing"

// W9 coverage for assorted small-file arms: FreshenDefault, CheckState
// guards, scalarFoldOperand, joinCarriersInner's >cap collapse,
// collectBodyLocalDefs, boundNode, IsSigTypeValue, and canonFnDef.

func TestW9CheckStateGuards(t *testing.T) {
	// Begin on a nil check returns a no-op closure that is safe to call.
	done := (*CheckState)(nil).Begin()
	done()

	c := &CheckState{}
	// recordCallEdge with an empty endpoint is a no-op.
	c.recordCallEdge("", "callee")

	// RecordFnBinder with an empty top-of-stack fn name returns early.
	c.Mode = true
	c.FnNameStack = []string{""}
	c.RecordFnBinder("x")
	if c.FnBinders != nil {
		t.Error("an empty enclosing fn must not record a binder")
	}
}

func TestW9ScalarFoldOperand(t *testing.T) {
	// A plain concrete integer is inert-const → the fast path.
	if !scalarFoldOperand(NewInteger(1)) {
		t.Error("a concrete integer is a scalar fold operand")
	}
	// A carrier whose payload is a value-scalar reaches the carrier-tolerant
	// switch arm (not inert-const, not dynamic, scalar payload).
	scalarCarrier := Value{Parent: TInteger, Data: IntPayload{N: 1}, Carrier: true}
	if !scalarFoldOperand(scalarCarrier) {
		t.Error("a scalar-payload carrier is a fold operand")
	}
	// A compound (ChildTypeInfo) carrier declines.
	if scalarFoldOperand(NewCarrier(TInteger)) {
		t.Error("a compound carrier is not a scalar fold operand")
	}
}

func TestW9JoinCarriersInnerOverCap(t *testing.T) {
	// More than CarrierDisjunctCap distinct alternatives collapse to a single
	// common-ancestor carrier.
	first := []Value{
		NewTypeLiteral(TInteger), NewTypeLiteral(TString), NewTypeLiteral(TBoolean),
		NewTypeLiteral(TAtom), NewTypeLiteral(TList),
	}
	second := []Value{
		NewTypeLiteral(TMap), NewTypeLiteral(TWord), NewTypeLiteral(TFunction),
		NewTypeLiteral(TError), NewTypeLiteral(TType),
	}
	out := joinCarriersInner(NewDisjunct(first), NewDisjunct(second))
	if !out.Carrier {
		t.Errorf("an over-cap join should collapse to a carrier, got %v", out)
	}
	if IsDisjunct(out) {
		t.Error("an over-cap join must not remain a disjunct")
	}
}

func TestW9CollectBodyLocalDefs(t *testing.T) {
	locals := map[string]bool{}
	quoted := NewInteger(1)
	quoted.Quoted = true
	// Body: a quoted token (skipped) and a nested fn value (skipped).
	collectBodyLocalDefs([]Value{quoted, NewFunction(FnDefInfo{})}, locals)
	if len(locals) != 0 {
		t.Errorf("quoted / nested-fn tokens should contribute no locals, got %v", locals)
	}
}

func TestW9BoundNodeClassType(t *testing.T) {
	ct := w9ClassType("T_bn2")
	if got := boundNode(ct); got == nil {
		t.Error("a class type bound should surface its minted node")
	}
}

func TestW9IsSigTypeValueClassType(t *testing.T) {
	r := newTestRegistry(t)
	if !IsSigTypeValue(r, w9ClassType("T_sig")) {
		t.Error("a class type is a signature type value")
	}
}

func TestW9CanonFnDefMultiReturn(t *testing.T) {
	fd := FnDefInfo{
		Name: "w9cf",
		Signatures: []Signature{{
			Params:  []FnParam{{Name: "a", Type: TInteger}},
			Returns: []*Type{TInteger, TString},
			Impl:    Boru([]Value{NewWord("a")}),
		}},
	}
	if got := canonFnDef(fd); got == "" {
		t.Error("canonFnDef should render a non-empty canonical string")
	}
}
