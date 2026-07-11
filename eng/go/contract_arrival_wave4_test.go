package eng

import (
	"strings"
	"testing"
)

// Coverage for the runtime fn-return contract in both engines (engine.go
// validateReturnTypes / returnCountError, vm.go checkReturnContract /
// vmReturnTypeErr / vmReturnCountErr), the forward-collection ARRIVAL
// loop (speculative word operands, multi-value arrivals, the
// type-mismatch implicit end — engine.go stepLiteral collection block /
// implicitEnd / curryOrStack / commitBarrierForward), and void argument
// groups (voidArgErrorFor). Reuses the harness from
// compile_pipeline_cov_test.go.

// registerContractWords adds:
//   - clie:    0-arg, declared Any, returns a String (a "lying" dynamic).
//   - cvar2:   0-arg, declared 1×Any, returns TWO Integers.
//   - cbool:   0-arg → true.
//   - cbools2: 0-arg → true, false (two values).
//   - cpairany: (Any, Integer) → args[1].
//   - cvoid:   0-arg → no values.
func registerContractWords(r *Registry) {
	zeroArg := func(name string, rets []*Type, vals func() []Value) {
		r.RegisterNativeFunc(NativeFunc{
			Name: name,
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return vals(), nil
				}),
				Returns: rets, BarrierPos: -1,
			}},
		})
	}
	zeroArg("clie", []*Type{TAny}, func() []Value { return []Value{NewString("zzz")} })
	zeroArg("cvar2", []*Type{TAny}, func() []Value { return []Value{NewInteger(1), NewInteger(2)} })
	zeroArg("cbool", []*Type{TBoolean}, func() []Value { return []Value{NewBoolean(true)} })
	zeroArg("cbools2", []*Type{TBoolean, TBoolean}, func() []Value {
		return []Value{NewBoolean(true), NewBoolean(false)}
	})
	zeroArg("cvoid", []*Type{}, func() []Value { return nil })
	r.RegisterNativeFunc(NativeFunc{
		Name: "cpairany",
		Signatures: []Signature{{
			Args: []*Type{TAny, TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[1]}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
}

// --- fn return contract at runtime (interpreted AND compiled) ----------------------

func TestFnReturnTypeDivergenceParity(t *testing.T) {
	// wantint declares [Integer] but its body result is only known at run
	// time (clie is declared Any, returns a String): the static pass
	// accepts optimistically, and BOTH engines raise the identical
	// return-type error at run time.
	setup := func(r *Registry) {
		registerContractWords(r)
		InstallFnDef(r, "wantint", FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "x", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       AQL(parenBody(NewOpenParen(), NewWord("clie"), NewCloseParen())),
				BarrierPos: BarrierAllForward,
			}},
		})
	}
	runErrParity(t, setup, func() []Value {
		return []Value{NewWord("wantint"), NewInteger(1)}
	}, "return value 1: expected Integer")
}

func TestFnReturnCountDivergenceParity(t *testing.T) {
	// The body nets TWO values at run time where the fn declares one:
	// both engines raise the identical return-count error.
	setup := func(r *Registry) {
		registerContractWords(r)
		InstallFnDef(r, "one", FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "x", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       AQL(parenBody(NewOpenParen(), NewWord("cvar2"), NewCloseParen())),
				BarrierPos: BarrierAllForward,
			}},
		})
	}
	runErrParity(t, setup, func() []Value {
		return []Value{NewWord("one"), NewInteger(1)}
	}, "expected 1 return value(s), got 2")
}

// --- forward-collection arrival loop ---------------------------------------------------

func TestSpeculativeWordArrivalCompletes(t *testing.T) {
	// `cpairany (cbool) 5` — a grouped call's RESULT arrives at the parked
	// forward. (Under the strict forward-barrier a bare `cbool` word would
	// be a barrier and strand; the paren makes its Boolean the operand.)
	r := covRegistry(t, registerContractWords)
	out, err := NewTop(r).Run([]Value{
		NewWord("cpairany"), NewOpenParen(), NewWord("cbool"), NewCloseParen(), NewInteger(5),
	})
	if err != nil {
		t.Fatalf("grouped arrival: %v", err)
	}
	if got := renderAll(out); got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestMultiValueArrivalFillsForward(t *testing.T) {
	// `9 cpairany (cbools2)` — the parked forward expects ONE arrival; the
	// grouped call produces two values, the first completes the dispatch and
	// the second stays on the stack.
	r := covRegistry(t, registerContractWords)
	out, err := NewTop(r).Run([]Value{
		NewInteger(9), NewWord("cpairany"), NewOpenParen(), NewWord("cbools2"), NewCloseParen(),
	})
	if err != nil {
		t.Fatalf("multi-value arrival: %v", err)
	}
	if got := renderAll(out); got != "9 | false" {
		t.Errorf("got %q, want 9 | false", got)
	}
}

func TestArrivalTypeMismatchImplicitEnd(t *testing.T) {
	// `cpairany cbools2 7` — the plan parks TWO forward slots (Any,
	// Integer); the second ARRIVAL (false) mismatches the Integer slot,
	// so the forward resolves early (implicit end) and the re-dispatch
	// raises a clean signature_error.
	r := covRegistry(t, registerContractWords)
	_, err := NewTop(r).Run([]Value{NewWord("cpairany"), NewWord("cbools2"), NewInteger(7)})
	if err == nil {
		t.Fatal("mismatched arrival did not error")
	}
	if !strings.Contains(err.Error(), "signature_error") ||
		!strings.Contains(err.Error(), "cpairany") {
		t.Errorf("wrong error: %v", err)
	}
}

// --- void argument groups ------------------------------------------------------------------

func TestVoidGroupCollectionResumes(t *testing.T) {
	// A void paren group in the argument range is skipped; collection
	// resumes with the following literals: `cadd (cvoid) 5 6` → 11.
	r := covRegistry(t, registerContractWords)
	out, err := NewTop(r).Run([]Value{
		NewWord("cadd"),
		NewOpenParen(), NewWord("cvoid"), NewCloseParen(),
		NewInteger(5), NewInteger(6),
	})
	if err != nil {
		t.Fatalf("void group resume: %v", err)
	}
	if got := renderAll(out); got != "11" {
		t.Errorf("got %q, want 11", got)
	}
}

func TestVoidGroupStarvationBlamesExpression(t *testing.T) {
	// When the starved dispatch then fails, the error names the void
	// expression (the ERRORS §3 blame shift), not a generic mismatch.
	r := covRegistry(t, registerContractWords)
	_, err := NewTop(r).Run([]Value{
		NewWord("ccat"),
		NewOpenParen(), NewWord("cvoid"), NewCloseParen(),
		NewString("a"), NewInteger(5),
	})
	if err == nil {
		t.Fatal("starved ccat did not error")
	}
	if !strings.Contains(err.Error(), "no_value_error") {
		t.Errorf("void-group blame missing: %v", err)
	}
}

// --- pattern-param dispatch ------------------------------------------------------------------

func TestAnonFnMapPatternParam(t *testing.T) {
	// An anonymous fn whose param carries a MAP PATTERN dispatches via
	// the stack-match open-unify branch, in check mode and at run time.
	r := covRegistry(t, nil)
	pat := mapOf("k", NewTypeLiteral(TInteger))
	fnv := NewFunction(FnDefInfo{
		Anonymous: true,
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "m", Type: TMap, Pattern: &pat}},
			Returns:    []*Type{TInteger},
			Impl:       AQL(parenBody(NewInteger(1))),
			BarrierPos: 0,
		}},
	})
	done := r.Check.Begin()
	out, err := NewTop(r).Run([]Value{mapOf("k", NewInteger(5)), fnv})
	done()
	if err != nil {
		t.Fatalf("check-mode pattern dispatch: %v", err)
	}
	if got := renderAll(out); got != "1" {
		t.Errorf("check-mode pattern dispatch = %q", got)
	}
	out, err = NewTop(r).Run([]Value{mapOf("k", NewInteger(5)), fnv})
	if err != nil {
		t.Fatalf("runtime pattern dispatch: %v", err)
	}
	if got := renderAll(out); got != "1" {
		t.Errorf("runtime pattern dispatch = %q", got)
	}

	// A non-conforming map (wrong value type) does not dispatch — the
	// values stay on the stack as data.
	out, err = NewTop(r).Run([]Value{mapOf("k", NewString("x")), fnv})
	if err != nil {
		t.Fatalf("mismatched pattern: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("mismatched pattern dispatched anyway: %s", renderAll(out))
	}
}
