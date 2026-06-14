package eng

import "testing"

// TestEmitOperandZeroValueIsNone pins the invariant the kind-enum refactor
// establishes (eng/go/CLAUDE.md "No Zero-Value Overload"): a freshly zeroed
// emitOperand is opNone — an invalid operand — and is NEVER mistaken for a
// valid "const index 0". The negative half is the point: before the refactor
// the zero value had constIdx == 0, indistinguishable from Consts[0].
func TestEmitOperandZeroValueIsNone(t *testing.T) {
	var zero emitOperand
	if zero.kind != opNone {
		t.Fatalf("zero emitOperand.kind = %d, want opNone (%d)", zero.kind, opNone)
	}
	// A zero operand must not read as any indexed kind.
	for _, k := range []operandKind{opConst, opEvent, opLocal, opType, opClosure} {
		if zero.kind == k {
			t.Fatalf("zero emitOperand.kind unexpectedly == %d", k)
		}
	}
}

// TestOperandConstructors checks each indexed constructor pairs the right kind
// with its idx, and (negative) that a const operand is not read as an event,
// local, or type — the discriminations every lowerer switch relies on.
func TestOperandConstructors(t *testing.T) {
	cases := []struct {
		name string
		op   emitOperand
		kind operandKind
		idx  int
	}{
		{"const", constOperand(7), opConst, 7},
		{"event", eventOperand(3, 0), opEvent, 3},
		{"local", localOperand(2), opLocal, 2},
		{"type", typeOperand(5), opType, 5},
	}
	for _, c := range cases {
		if c.op.kind != c.kind {
			t.Errorf("%s: kind = %d, want %d", c.name, c.op.kind, c.kind)
		}
		if c.op.idx != c.idx {
			t.Errorf("%s: idx = %d, want %d", c.name, c.op.idx, c.idx)
		}
	}
	// Negative: a const operand must not satisfy the event/local/type tests.
	co := constOperand(0)
	if co.kind == opEvent || co.kind == opLocal || co.kind == opType {
		t.Fatalf("const operand misclassified as event/local/type")
	}
}
