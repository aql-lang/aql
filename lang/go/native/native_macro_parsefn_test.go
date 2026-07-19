package native

import (
	"testing"
)

// Coverage for the `parse` macro's ParseLang-value form (parseFnExpand) —
// the arms a surface program cannot reach through sig dispatch: the matcher
// refuses a bare Function type literal for a TFunction slot, and a carrier
// operand only arrives under analysis. Driven directly, mirroring the W9
// macro seams. See design/TEST-SEAMS.10.md.

// A fn-family value whose payload is not an FnDefInfo is refused: the sig
// matcher never delivers one from surface syntax (a bare `Function` type
// literal is parented at Type and refuses every parse sig — pinned by the
// module-parselang.tsv §10 signature_error row), so the defensive guard is
// driven directly with a crafted payload, like the eng fn-value seams.
func TestParseFnExpandNonFnPayload(t *testing.T) {
	r := seam5Reg(t)
	_, err := parseHandler([]Value{NewValueRaw(TFunction, IntPayload{N: 1}), NewString("x")}, nil, nil, r)
	if err == nil {
		t.Fatal("parse: a non-FnDefInfo function payload must error")
	}
}

// A TFunction CARRIER operand (a computed parser under analysis) degrades to
// a dynamic value with the macro advisory instead of erroring.
func TestParseFnExpandCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	out, err := parseHandler([]Value{NewCarrier(TFunction), NewString("x")}, nil, nil, r)
	if err != nil {
		t.Fatalf("parse: a carrier parser under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("parse: carrier parser should yield a dynamic carrier, got %v", Canon(out))
	}
}

// A bare word BOUND to a TFunction carrier (a fn param used as the parser)
// takes the name-path carrier arm: degrade + advisory, and the binding is
// recorded as used so unused_def stays quiet.
func TestParseFnNameBoundCarrier(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Defs.Push("p", NewCarrier(TFunction))
	out, err := parseHandler([]Value{NewAtom("p"), NewString("x")}, nil, nil, r)
	if err != nil {
		t.Fatalf("parse: a carrier fn binding under analysis must degrade, got %v", err)
	}
	if len(out) != 1 || !out[0].Dynamic {
		t.Fatalf("parse: carrier fn binding should yield a dynamic carrier, got %v", Canon(out))
	}
	if !r.Check.DefsUsed["p"] {
		t.Fatal("parse: the parser binding must be recorded as used")
	}
}

// --- recordParseLangFnDispatch (compile-recorder arms; mirrors the W9
// recordDeferredParseDispatch tests) ---

func TestParseFnDispatchInstallGuard(t *testing.T) {
	// Nil registry / nil sig are no-ops (no panic) — mirror of the
	// InstallParseDeferredDispatch guard test.
	InstallParseLangFnDispatch(nil, nil)
	r := seam5Reg(t)
	InstallParseLangFnDispatch(r, nil)
}

func TestParseFnDispatchRecordNoDispatcher(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	fn := NewCarrier(TFunction)
	// No dispatcher installed → the sig lookup fails and recording declines.
	if _, ok := recordParseLangFnDispatch(r, fn, []Value{fn, NewString("x")}); ok {
		t.Fatal("recordParseLangFnDispatch must decline without an installed dispatcher")
	}
}

func TestParseFnDispatchRecordThreeArgs(t *testing.T) {
	r := seam5Reg(t)
	cleanup := r.Check.Begin()
	defer cleanup()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	InstallParseLangFnDispatch(r, &Signature{
		Args:       []*Type{TAny, TAny, TMap},
		Returns:    []*Type{TAny},
		BarrierPos: -1,
	})
	fn := NewCarrier(TFunction)
	// Three args → the opts-middle / source-last split branch.
	out, ok := recordParseLangFnDispatch(r, fn,
		[]Value{fn, NewMap(NewOrderedMap()), NewString("x + y")})
	if !ok {
		t.Fatal("recordParseLangFnDispatch should record with a dispatcher installed")
	}
	if !out.Dynamic {
		t.Fatalf("fn-dispatch result should be a dynamic carrier, got %s", out.String())
	}
}

// Outside check mode a non-concrete fn-family binding is NOT a parser: the
// name path falls through to the ordinary unknown-kind error.
func TestParseFnNameBoundTypeLiteralFallsThrough(t *testing.T) {
	r := seam5Reg(t)
	r.Defs.Push("p", NewTypeLiteral(TFunction))
	if _, err := parseHandler([]Value{NewAtom("p"), NewString("x")}, nil, nil, r); err == nil {
		t.Fatal("parse: a Function type-literal binding must fall through to unknown-kind")
	}
}
