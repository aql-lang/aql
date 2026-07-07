package eng

import (
	"strings"
	"testing"
)

// bytecode_seam8_test.go covers the residual formatting/constructor arms of
// bytecode.go: the unknown-opcode name fallback, the closure constructor, and
// the disassembler cases (guarded native, fallback, user-poly) plus the
// userpolys summary line. The disassembler is a pure formatter, so a
// hand-built Program with the relevant opcodes drives every arm directly.

func TestW8OpcodeStringUnknown(t *testing.T) {
	// An opcode past the name table renders its numeric fallback.
	got := Opcode(250).String()
	if got != "OP(250)" {
		t.Fatalf("unknown opcode name = %q, want OP(250)", got)
	}
}

func TestW8NewClosureValue(t *testing.T) {
	v := NewClosure(3, []Value{NewInteger(1)})
	cp, ok := v.Data.(ClosurePayload)
	if !ok {
		t.Fatalf("NewClosure did not carry a ClosurePayload: %v", v)
	}
	if cp.Unit != 3 || len(cp.Captures) != 1 {
		t.Fatalf("closure payload = %+v, want unit 3 with 1 capture", cp)
	}
	if !v.Parent.Equal(TFunction) {
		t.Fatalf("closure value parent = %v, want Function", v.Parent)
	}
}

func TestW8DisassembleResidualOps(t *testing.T) {
	// One program exercising the guarded-native, fallback, and user-poly
	// disasm cases and the userpolys summary line.
	sig := Signature{Args: []*Type{TInteger}, BarrierPos: -1}
	p := &Program{
		Code: []Instr{
			{Op: OpCallNative, Arg: 0},
			{Op: OpFallback, Arg: 0},
			{Op: OpCallUserPoly, Arg: 0},
		},
		Debug:     make([]SrcPos, 3),
		Sigs:      []SigRef{{Word: "gw", Sig: &sig, Guard: true}},
		Fallbacks: []FallbackSpan{{Desc: "fb", NIn: 1}},
		UserPolys: []UserPolyRef{{Word: "up", Arity: 1, Units: []int{0}}},
	}
	out := p.Disassemble()
	for _, want := range []string{
		"[guarded]",       // guarded CALL_NATIVE (bytecode.go:734)
		"fb (nin=1)",      // OpFallback (bytecode.go:744)
		"up/1 (user poly", // OpCallUserPoly (bytecode.go:754)
		"userpolys=1",     // summary line (bytecode.go:713)
	} {
		if !strings.Contains(out, want) {
			t.Errorf("disassembly missing %q in:\n%s", want, out)
		}
	}
}
