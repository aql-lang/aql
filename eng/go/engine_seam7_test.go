package eng

// Wave-7 coverage for engine.go: the interpreter step loop, dispatch,
// and error/edge arms. Tests are white-box and in-package: helper
// functions and *Engine methods are exercised directly with hand-built
// tape/registry state where a full Run() would be a roundabout way to
// reach a guard arm, and driven through Run() where the arm only fires
// as part of real execution. See design/TEST-SEAMS.10.md.

import (
	"strings"
	"testing"
)

// engWithTape builds a top-level engine over a fresh cov registry with
// the given tape values and pointer set to the given index.
func engWithTape(t *testing.T, vals []Value, pointer int) *Engine {
	t.Helper()
	r := covRegistry(t, nil)
	e := NewTop(r)
	e.tape = NewTape(vals, stackHeadroom)
	e.pointer = pointer
	return e
}

// --- reorderHintFor -------------------------------------------------------

func TestS7ReorderHintForNilFn(t *testing.T) {
	if got := reorderHintFor("x", nil, nil); got != "" {
		t.Errorf("reorderHintFor(nil fn) = %q, want empty", got)
	}
}

func TestS7ReorderHintForUnnamedParams(t *testing.T) {
	// A legacy-Args sig (no Params) so the param-naming falls into the
	// else branch that emits the bare type leaf. Sig wants (String,
	// Integer); written (Integer, String) fails identity but matches
	// under a permutation, so a reorder hint is produced.
	fn := &FnDefInfo{Signatures: []Signature{{
		Args:       []*Type{TString, TInteger},
		Returns:    []*Type{TInteger},
		BarrierPos: -1,
	}}}
	written := []Value{NewInteger(1), NewString("x")}
	got := reorderHintFor("swap", fn, written)
	if got == "" {
		t.Fatal("expected a reorder hint")
	}
	if !strings.Contains(got, "did you swap the arguments?") {
		t.Errorf("hint = %q, want swap advice", got)
	}
	if !strings.Contains(got, "swap String Integer") {
		t.Errorf("hint = %q, want unnamed leaf params 'String Integer'", got)
	}
}

func TestS7ReorderHintForNamedParams(t *testing.T) {
	// Named params take the other arm (params[i]=name:type).
	fn := &FnDefInfo{Signatures: []Signature{{
		Params:     []FnParam{{Name: "policy", Type: TString}, {Name: "n", Type: TInteger}},
		Returns:    []*Type{TInteger},
		BarrierPos: -1,
	}}}
	written := []Value{NewInteger(1), NewString("x")}
	got := reorderHintFor("wp", fn, written)
	if !strings.Contains(got, "policy:String") || !strings.Contains(got, "n:Integer") {
		t.Errorf("hint = %q, want named params", got)
	}
}

// --- assignsPositionally --------------------------------------------------

func TestS7AssignsPositionallyIdentityAndPermutation(t *testing.T) {
	sig := &Signature{Args: []*Type{TString, TInteger}, BarrierPos: -1}
	written := []Value{NewInteger(1), NewString("x")}
	if assignsPositionally(written, sig, true) {
		t.Error("identity match should fail for swapped args")
	}
	if !assignsPositionally(written, sig, false) {
		t.Error("permutation match should succeed for swapped args")
	}
	// No permutation exists for two Integers against (String, Integer).
	both := []Value{NewInteger(1), NewInteger(2)}
	if assignsPositionally(both, sig, false) {
		t.Error("no assignment should satisfy (String,Integer) from two Integers")
	}
}

// --- sigError reorder branch ----------------------------------------------

func TestS7SigErrorReorderBranch(t *testing.T) {
	// Build an engine whose stack top matches a sig under permutation
	// only, so sigError takes the reorder case.
	fn := &FnDefInfo{Signatures: []Signature{{
		Params:     []FnParam{{Name: "policy", Type: TString}, {Name: "n", Type: TInteger}},
		Returns:    []*Type{TInteger},
		BarrierPos: -1,
	}}}
	// Stack (bottom..top): Integer then String; reorderCandidates walks
	// top-first producing [String, Integer], which fails identity but
	// matches (String,Integer) under the identity — so to fail identity
	// we need the OTHER order. Put String at bottom, Integer on top.
	e := engWithTape(t, []Value{NewString("x"), NewInteger(1)}, 2)
	err := e.sigError("wp", fn, SrcPos{})
	if err == nil {
		t.Fatal("expected a sig error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "did you swap the arguments?") {
		t.Errorf("sigError = %q, want reorder hint", msg)
	}
}

// --- insufficientArgsError ------------------------------------------------

func TestS7InsufficientArgsError(t *testing.T) {
	e := engWithTape(t, []Value{NewInteger(1)}, 1)
	err := e.insufficientArgsError("need2", 2, SrcPos{Row: 3, Col: 5})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot call `need2` — it expects 2 forward arguments") {
		t.Errorf("err = %q", err.Error())
	}
}

// --- undefinedWordHint ----------------------------------------------------

func TestS7UndefinedWordHintVoidGroup(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.voidGroups = []string{"r"}
	got := e.undefinedWordHint("r")
	if !strings.Contains(got, "produced no value to bind") {
		t.Errorf("hint = %q, want void-group message", got)
	}
	// Unknown name → empty hint.
	if got := e.undefinedWordHint("other"); got != "" {
		t.Errorf("hint for non-void name = %q, want empty", got)
	}
}

func TestS7UndefinedWordHintDefBody(t *testing.T) {
	// pendingForwardFunc == "def": a Forward marker for def before pointer.
	fwd := NewForward(ForwardInfo{FuncName: "def", FuncIndex: 0})
	e := engWithTape(t, []Value{fwd, NewInteger(0)}, 2)
	got := e.undefinedWordHint("foo")
	if !strings.Contains(got, "def … (foo)") {
		t.Errorf("hint = %q, want def-body advice", got)
	}
}

// --- voidArgErrorFor ------------------------------------------------------

func TestS7VoidArgErrorForDefNamed(t *testing.T) {
	// def in voidGroups with an atom name below the pointer.
	e := engWithTape(t, []Value{NewAtom("myvar"), NewInteger(0)}, 2)
	e.voidGroups = []string{"def"}
	err := e.voidArgErrorFor("def", SrcPos{})
	if err == nil {
		t.Fatal("expected def_error")
	}
	if !strings.Contains(err.Error(), "myvar") {
		t.Errorf("err = %q, want the bound name myvar", err.Error())
	}
}

func TestS7VoidArgErrorForDefOpenParenStop(t *testing.T) {
	// An open-paren below the pointer stops the atom search: name stays <name>.
	e := engWithTape(t, []Value{NewOpenParen(), NewInteger(0)}, 2)
	e.voidGroups = []string{"def"}
	err := e.voidArgErrorFor("def", SrcPos{})
	if err == nil || !strings.Contains(err.Error(), "<name>") {
		t.Fatalf("err = %v, want <name> placeholder", err)
	}
}

func TestS7VoidArgErrorForNonDef(t *testing.T) {
	e := engWithTape(t, nil, 0)
	e.voidGroups = []string{"print"}
	err := e.voidArgErrorFor("print", SrcPos{})
	if err == nil || !strings.Contains(err.Error(), "produced no value for print") {
		t.Fatalf("err = %v, want no_value_error", err)
	}
	// Not in voidGroups → nil.
	if got := e.voidArgErrorFor("other", SrcPos{}); got != nil {
		t.Errorf("voidArgErrorFor(non-member) = %v, want nil", got)
	}
}

// --- isFnShapeTypedBindingContext -----------------------------------------

func TestS7IsFnShapeTypedBindingContextNoRegistry(t *testing.T) {
	e := &Engine{}
	if e.isFnShapeTypedBindingContext() {
		t.Error("nil registry / zero pointer must be false")
	}
}

func TestS7IsFnShapeTypedBindingContextOpenParenStop(t *testing.T) {
	e := engWithTape(t, []Value{NewOpenParen(), NewInteger(0)}, 2)
	if e.isFnShapeTypedBindingContext() {
		t.Error("open paren before pointer must stop and return false")
	}
}

func TestS7IsFnShapeTypedBindingContextNonDefForward(t *testing.T) {
	fwd := NewForward(ForwardInfo{FuncName: "notdef"})
	e := engWithTape(t, []Value{fwd, NewInteger(0)}, 2)
	if e.isFnShapeTypedBindingContext() {
		t.Error("non-def forward must return false")
	}
}

func TestS7IsFnShapeTypedBindingContextDefCollectedZero(t *testing.T) {
	// def forward whose sig has TMap at pos 0, but CollectedArgs < 1.
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 0, FuncIndex: 1})
	e := engWithTape(t, []Value{fwd, NewInteger(0)}, 2)
	if e.isFnShapeTypedBindingContext() {
		t.Error("CollectedArgs<1 must return false")
	}
}

func TestS7IsFnShapeTypedBindingContextMapIdxOutOfRange(t *testing.T) {
	// FuncIndex - CollectedArgs negative → out of range guard.
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 5, FuncIndex: 1})
	e := engWithTape(t, []Value{fwd, NewInteger(0)}, 2)
	if e.isFnShapeTypedBindingContext() {
		t.Error("out-of-range map index must return false")
	}
}

func TestS7IsFnShapeTypedBindingContextMapLenNotOne(t *testing.T) {
	// The typed-name map must have exactly one key; a two-key map fails.
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("a", NewTypeLiteral(TInteger))
	m.Set("b", NewTypeLiteral(TString))
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 1})
	e := engWithTape(t, []Value{NewMap(m), fwd, NewInteger(0)}, 3)
	if e.isFnShapeTypedBindingContext() {
		t.Error("two-key typed-name map → false")
	}
}

func TestS7IsFnShapeTypedBindingContextWordConstraintFnShape(t *testing.T) {
	// The single constraint is a Word resolving (via ResolveTypedName) to
	// an FnUndef-shaped type — the §7.2 "fn name where (quote name) meant"
	// case. Exercises the IsWord + ResolveTypedName + Equal(TFnUndef) arms.
	r := covRegistry(t, nil)
	// A value TAGGED as FnUndef (Parent == TFnUndef); the context probe
	// tests constraint.Parent.Equal(TFnUndef).
	r.Defs.Push("MyShape", NewCarrier(TFnUndef))
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("f", NewWord("MyShape"))
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 1})
	e := NewTop(r)
	e.tape = NewTape([]Value{NewMap(m), fwd, NewInteger(0)}, stackHeadroom)
	e.pointer = 3
	if !e.isFnShapeTypedBindingContext() {
		t.Error("a fn-shape typed-name constraint via a word → true")
	}
}

func TestS7IsFnShapeTypedBindingContextNonFnConstraint(t *testing.T) {
	// A one-key typed-name map whose constraint is a plain Integer type,
	// not a fn-shape → false (exercises the constraint read + Equal).
	sig := &Signature{Params: []FnParam{{Type: TMap}, {Type: TAny}}, BarrierPos: -1}
	m := NewOrderedMap()
	m.Set("x", NewTypeLiteral(TInteger))
	mapVal := NewMap(m)
	// tape: [map, forward, funcword]; CollectedArgs=1, FuncIndex=1 → mapIdx=0.
	fwd := NewForward(ForwardInfo{FuncName: "def", Sig: sig, CollectedArgs: 1, FuncIndex: 1})
	e := engWithTape(t, []Value{mapVal, fwd, NewInteger(0)}, 3)
	if e.isFnShapeTypedBindingContext() {
		t.Error("Integer-constraint typed name is not fn-shape → false")
	}
}
