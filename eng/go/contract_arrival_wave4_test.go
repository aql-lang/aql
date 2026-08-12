package eng

import (
	"strings"
	"testing"

	core "github.com/boru-lang/boru/core/go"
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
func registerContractWords(r *core.Registry) {
	zeroArg := func(name string, rets []*core.Type, vals func() []core.Value) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: name,
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return vals(), nil
				}),
				Returns: rets, BarrierPos: -1,
			}},
		})
	}
	zeroArg("clie", []*core.Type{core.TAny}, func() []core.Value { return []core.Value{core.NewString("zzz")} })
	zeroArg("cvar2", []*core.Type{core.TAny}, func() []core.Value { return []core.Value{core.NewInteger(1), core.NewInteger(2)} })
	zeroArg("cbool", []*core.Type{core.TBoolean}, func() []core.Value { return []core.Value{core.NewBoolean(true)} })
	zeroArg("cbools2", []*core.Type{core.TBoolean, core.TBoolean}, func() []core.Value {
		return []core.Value{core.NewBoolean(true), core.NewBoolean(false)}
	})
	zeroArg("cvoid", []*core.Type{}, func() []core.Value { return nil })
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cpairany",
		Signatures: []core.Signature{{
			Args: []*core.Type{core.TAny, core.TInteger},
			Impl: core.Go(func(args []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{args[1]}, nil
			}),
			Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
		}},
	})
}

// --- fn return contract at runtime (interpreted AND compiled) ----------------------

func TestFnReturnTypeDivergenceParity(t *testing.T) {
	// wantint declares [Integer] but its body result is only known at run
	// time (clie is declared Any, returns a String): the static pass
	// accepts optimistically, and BOTH engines raise the identical
	// return-type error at run time.
	setup := func(r *core.Registry) {
		registerContractWords(r)
		core.InstallFnDef(r, "wantint", core.FnDefInfo{
			Signatures: []core.Signature{{
				Params:     []core.FnParam{{Name: "x", Type: core.TInteger}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewOpenParen(), core.NewWord("clie"), core.NewCloseParen())),
				BarrierPos: core.BarrierAllForward,
			}},
		})
	}
	runErrParity(t, setup, func() []core.Value {
		return []core.Value{core.NewWord("wantint"), core.NewInteger(1)}
	}, "return value 1: expected Integer")
}

func TestFnReturnCountDivergenceParity(t *testing.T) {
	// The body nets TWO values at run time where the fn declares one:
	// both engines raise the identical return-count error.
	setup := func(r *core.Registry) {
		registerContractWords(r)
		core.InstallFnDef(r, "one", core.FnDefInfo{
			Signatures: []core.Signature{{
				Params:     []core.FnParam{{Name: "x", Type: core.TInteger}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewOpenParen(), core.NewWord("cvar2"), core.NewCloseParen())),
				BarrierPos: core.BarrierAllForward,
			}},
		})
	}
	runErrParity(t, setup, func() []core.Value {
		return []core.Value{core.NewWord("one"), core.NewInteger(1)}
	}, "expected 1 return value(s), got 2")
}

// --- forward-collection arrival loop ---------------------------------------------------

func TestSpeculativeWordArrivalCompletes(t *testing.T) {
	// `cpairany (cbool) 5` — a grouped call's RESULT arrives at the parked
	// forward. (Under the strict forward-barrier a bare `cbool` word would
	// be a barrier and strand; the paren makes its Boolean the operand.)
	r := covRegistry(t, registerContractWords)
	out, err := core.NewTop(r).Run([]core.Value{
		core.NewWord("cpairany"), core.NewOpenParen(), core.NewWord("cbool"), core.NewCloseParen(), core.NewInteger(5),
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
	out, err := core.NewTop(r).Run([]core.Value{
		core.NewInteger(9), core.NewWord("cpairany"), core.NewOpenParen(), core.NewWord("cbools2"), core.NewCloseParen(),
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
	_, err := core.NewTop(r).Run([]core.Value{core.NewWord("cpairany"), core.NewWord("cbools2"), core.NewInteger(7)})
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
	out, err := core.NewTop(r).Run([]core.Value{
		core.NewWord("cadd"),
		core.NewOpenParen(), core.NewWord("cvoid"), core.NewCloseParen(),
		core.NewInteger(5), core.NewInteger(6),
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
	_, err := core.NewTop(r).Run([]core.Value{
		core.NewWord("ccat"),
		core.NewOpenParen(), core.NewWord("cvoid"), core.NewCloseParen(),
		core.NewString("a"), core.NewInteger(5),
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
	pat := mapOf("k", core.NewTypeLiteral(core.TInteger))
	fnv := core.NewFunction(core.FnDefInfo{
		Anonymous: true,
		Signatures: []core.Signature{{
			Params:     []core.FnParam{{Name: "m", Type: core.TMap, Pattern: &pat}},
			Returns:    []*core.Type{core.TInteger},
			Impl:       core.Boru(parenBody(core.NewInteger(1))),
			BarrierPos: 0,
		}},
	})
	done := r.Check.Begin()
	out, err := core.NewTop(r).Run([]core.Value{mapOf("k", core.NewInteger(5)), fnv})
	done()
	if err != nil {
		t.Fatalf("check-mode pattern dispatch: %v", err)
	}
	if got := renderAll(out); got != "1" {
		t.Errorf("check-mode pattern dispatch = %q", got)
	}
	out, err = core.NewTop(r).Run([]core.Value{mapOf("k", core.NewInteger(5)), fnv})
	if err != nil {
		t.Fatalf("runtime pattern dispatch: %v", err)
	}
	if got := renderAll(out); got != "1" {
		t.Errorf("runtime pattern dispatch = %q", got)
	}

	// A non-conforming map (wrong value type) does not dispatch — the
	// values stay on the stack as data.
	out, err = core.NewTop(r).Run([]core.Value{mapOf("k", core.NewString("x")), fnv})
	if err != nil {
		t.Fatalf("mismatched pattern: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("mismatched pattern dispatched anyway: %s", renderAll(out))
	}
}
