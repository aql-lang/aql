package native

// def keyword-form synthesis (registerDefKeywordForms / defFormVia /
// synthDefKeywordSig): positive forms are pinned corpus-wide by the
// spec suite; these tests pin the ERROR arms and the registration
// guards, per the pair-positive-with-negative discipline.

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func runDefForm(t *testing.T, src string) ([]Value, error) {
	t.Helper()
	return b2Run(b2Reg(t), src)
}

func TestDefGenChainGenErrorPropagates(t *testing.T) {
	// gen's own failure (a non-word placeholder) surfaces from the
	// chain form, not a swallow.
	_, err := runDefForm(t, "def X gen [5] fn [[x:Integer][Integer][x]]")
	if err == nil {
		t.Fatal("a non-word gen placeholder must error through the chain form")
	}
}

func TestDefGenChainTailEvalErrorPropagates(t *testing.T) {
	// The deferred re-evaluation of a tail operand propagates its
	// error (a schema field whose paren raises).
	_, err := runDefForm(t, "def B gen [T] class {v:(1 div 0)}")
	if err == nil || !strings.Contains(err.Error(), "zero") {
		t.Fatalf("map field eval error must propagate, got %v", err)
	}
	_, err = runDefForm(t, "def C gen [T] refine Record [x:(1 div 0)]")
	if err == nil || !strings.Contains(err.Error(), "zero") {
		t.Fatalf("list field eval error must propagate, got %v", err)
	}
}

func TestRegisterDefKeywordFormsGuards(t *testing.T) {
	// A registry with NO blessed constructors registered: every
	// Lookup misses and synthesis is a no-op (the fnDef==nil guard).
	bare, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerDefKeywordForms(bare)
	if fn := bare.Lookup("def"); fn != nil {
		t.Fatal("no constructors registered → no def overloads synthesized")
	}

	// A constructor whose aggregate carries a Fallback sig: the
	// Fallback is skipped, real sigs mirror.
	r2, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r2.Register("fn",
		Signature{Fallback: true, Returns: []*Type{}, BarrierPos: -1},
		Signature{Args: []*Type{TList}, NoEvalArgs: map[int]bool{0: true},
			Impl: Go(fnHandler, RunInCheck()), Returns: []*Type{TFunction}, BarrierPos: -1},
	)
	registerDefKeywordForms(r2)
	def := r2.Lookup("def")
	if def == nil {
		t.Fatal("fn registered → def keyword overloads synthesized")
	}
	for i := range def.Signatures {
		if def.Signatures[i].TotalArgs() == 2 && def.Signatures[i].Fallback {
			t.Fatal("a Fallback base sig must not be mirrored")
		}
	}
	// Exactly the real fn sig mirrors, each in an Atom-name and a
	// String-name variant (def accepts a word OR a string as the name):
	// the plain form (3 slots) and the gen chain (5 slots), ×2 = 4 sigs.
	counts := map[int]int{}
	for i := range def.Signatures {
		counts[def.Signatures[i].TotalArgs()]++
	}
	if len(def.Signatures) != 4 || counts[3] != 2 || counts[5] != 2 {
		t.Fatalf("want 4 mirrored sigs (plain + gen chain, Atom + String name), got %v", counts)
	}
}

func TestInsideForwardSwapFormMismatch(t *testing.T) {
	// A swap-form dispatch INSIDE a pending forward (print is parked)
	// whose stack operand mismatches the signature must reject the
	// mixed forward+stack split rather than mis-bind.
	if _, err := runDefForm(t, "print true div 3"); err == nil {
		t.Fatal("mismatched swap-form inside a pending forward must fail")
	}
}

func TestStackPhaseTypeMismatch(t *testing.T) {
	// /s forces the stack phase; a slot-type mismatch there must fail
	// the signature rather than mis-bind.
	if _, err := runDefForm(t, "1 true div/s"); err == nil {
		t.Fatal("mismatched stack operands must fail dispatch")
	}
}

func TestHasStructuralSlots(t *testing.T) {
	if hasStructuralSlots(&Signature{Args: []*Type{TList}}) {
		t.Error("plain typed sig has no structural slots")
	}
	if !hasStructuralSlots(&Signature{Args: []*Type{TAtom}, QuoteArgs: map[int]bool{0: true}}) {
		t.Error("a /q slot is structural")
	}
	if !hasStructuralSlots(&Signature{Args: []*Type{TAny}, RawParens: map[int]bool{0: true}}) {
		t.Error("a raw-paren slot is structural")
	}
	if !hasStructuralSlots(&Signature{Args: []*Type{TAny}, FormArgs: map[int]bool{0: true}}) {
		t.Error("a form slot is structural")
	}
	if !hasStructuralSlots(&Signature{Args: []*Type{TAny}, TypeArgs: map[int]bool{0: true}}) {
		t.Error("a type-literal slot is structural")
	}
}

func TestShiftPosFlags(t *testing.T) {
	if shiftPosFlags(nil, 2) != nil {
		t.Error("nil in → nil out")
	}
	got := shiftPosFlags(map[int]bool{0: true, 1: false}, 2)
	if !got[2] || got[3] {
		t.Errorf("flags must shift by offset, got %v", got)
	}
}
