package eng

import (
	"strings"
	"testing"
)

// Coverage for the dynamic fn-value apply seam across the pipeline:
// emit.go resolveDynamicApply / trailingApply / trailingWindowApplyShape /
// mixedDynamicApplyShape / anyDynamicTail / anyFnOrDynamicTail /
// RecordMakeMap, vm.go callDynamic / callDynamicOp / callDynamicMixed /
// invokeClosure / vmMakeMap / callPoly / matchUserPoly, and engine.go
// checkModeAssumeSig's plain-check summaries (argTypeSummary /
// sigTypeSummary). Reuses the harness from compile_pipeline_cov_test.go.

// registerDynWords adds:
//   - cany:  0-arg, declared Any (a dynamic carrier statically), returns 7.
//   - cmk1:  0-arg factory returning a 1-arity closure (adds 100).
//   - cmk2:  0-arg factory returning a 2-arity closure (csub-like, order
//     sensitive: returns args[1]-args[0]).
//   - upoly: user fn with Integer and String overloads.
func registerDynWords(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "cany",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{NewInteger(7)}, nil
			}),
			Returns: []*Type{TAny}, BarrierPos: -1,
		}},
	})
	mkClosure := func(params []FnParam, body []Value) Value {
		return NewFunction(FnDefInfo{
			Anonymous: true,
			Signatures: []Signature{{
				Params:     params,
				Returns:    []*Type{TInteger},
				Impl:       BORU(body),
				BarrierPos: BarrierAllForward,
			}},
		})
	}
	r.RegisterNativeFunc(NativeFunc{
		Name: "cmk1",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{mkClosure(
					[]FnParam{{Name: "x", Type: TInteger}},
					parenBody(NewWord("cadd"), NewWord("x"), NewInteger(100)),
				)}, nil
			}),
			Returns: []*Type{TFunction}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "cmk2",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{mkClosure(
					[]FnParam{{Name: "a", Type: TInteger}, {Name: "b", Type: TInteger}},
					parenBody(NewWord("cmul"), NewWord("a"), NewInteger(10),
						NewEnd(), NewWord("cadd"), NewWord("b")),
				)}, nil
			}),
			Returns: []*Type{TFunction}, BarrierPos: -1,
		}},
	})
	InstallFnDef(r, "upoly", FnDefInfo{
		Signatures: []Signature{
			{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       BORU(parenBody(NewWord("cadd"), NewWord("n"), NewInteger(1))),
				BarrierPos: BarrierAllForward,
			},
			{
				Params:     []FnParam{{Name: "s", Type: TString}},
				Returns:    []*Type{TString},
				Impl:       BORU(parenBody(NewWord("ccat"), NewWord("s"), NewString("!"))),
				BarrierPos: BarrierAllForward,
			},
		},
	})
}

// runTolerant executes tokens interpreted, asserts the expected render,
// then compiles; a refusal is tolerated (logged), a compiled program must
// agree with the interpreter.
func runTolerant(t *testing.T, extra func(*Registry), tokens func() []Value, want string) {
	t.Helper()
	ri := covRegistry(t, extra)
	iOut, iErr := NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted: %v", iErr)
	}
	if got := renderAll(iOut); got != want {
		t.Fatalf("interpreted = %q, want %q", got, want)
	}
	rc := covRegistry(t, extra)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Logf("compile refused (%s)", reason)
		return
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled: %v", cErr)
	}
	if got := renderAll(cOut); got != want {
		t.Errorf("compiled = %q, want %q", got, want)
	}
}

// --- leading / trailing / mixed dynamic apply -------------------------------------

func TestDynApplyLeadingClosure(t *testing.T) {
	// Factory result applied to a following static arg: OpCallDynamic.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewWord("cmk1"), NewInteger(5)}
	}, "105")
}

func TestDynApplyTrailingClosure(t *testing.T) {
	// Fn value lands ON TOP of its single arg: OpCallDynamicTrailing.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewInteger(5), NewWord("cmk1")}
	}, "105")
}

func TestDynApplyLeadingNonCallable(t *testing.T) {
	// A dynamic Any value that is NOT callable: the value stays below the
	// arg on both engines.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewWord("cany"), NewInteger(5)}
	}, "7 | 5")
}

func TestDynApplyTrailingNonCallable(t *testing.T) {
	// Trailing non-callable: rotation puts the value back on top.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewInteger(5), NewWord("cany")}
	}, "5 | 7")
}

func TestDynApplyMixedWindow(t *testing.T) {
	// Interior fn value between static args: `3 (cmk1) 2` — the closure
	// takes its forward arg; the leading 3 stays.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewInteger(3), NewWord("cmk1"), NewInteger(2)}
	}, "3 | 102")
}

func TestDynApplyTrailingWindow(t *testing.T) {
	// Two static args below one trailing 2-arity closure: `10 3 (cmk2)`.
	// Closure body: (a mul 10); (add b) with a=top=3, b=10 → 40.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewInteger(10), NewInteger(3), NewWord("cmk2")}
	}, "40")
}

func TestDynApplyTwoArgLeading(t *testing.T) {
	// Leading 2-arity closure over two static forward args.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{NewWord("cmk2"), NewInteger(3), NewInteger(10)}
	}, "40")
}

func TestDynApplyChainedFactories(t *testing.T) {
	// Two independent applications in one program.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewWord("cmk1"), NewInteger(1), NewEnd(),
			NewWord("cmk1"), NewInteger(2),
		}
	}, "101 | 102")
}

// --- poly dispatch over dynamic operands --------------------------------------------

func TestPolyDispatchDynamicOperandInt(t *testing.T) {
	// cdub has Integer and String overloads; its arg is a dynamic Any
	// (cany). The compiled program re-matches at run time.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewWord("cdub"),
			NewOpenParen(), NewWord("cany"), NewCloseParen(),
		}
	}, "14")
}

func TestUserPolyDispatchDynamicOperand(t *testing.T) {
	// A user fn with two overloads over a dynamic operand: the VM
	// re-derives the overload subset and matches at run time.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewWord("upoly"),
			NewOpenParen(), NewWord("cany"), NewCloseParen(),
		}
	}, "8")
}

func TestUserSingleSigDynamicOperand(t *testing.T) {
	// Single-overload user fn over a dynamic operand: guarded CALL_USER.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewWord("ctwice"),
			NewOpenParen(), NewWord("cany"), NewCloseParen(),
		}
	}, "14")
}

// --- computed map assembly (RecordMakeMap / vmMakeMap) --------------------------------

func registerMapWords(r *Registry) {
	registerDynWords(r)
	r.RegisterNativeFunc(NativeFunc{
		Name: "cmsize",
		Signatures: []Signature{{
			Args: []*Type{TMap},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				m, err := AsMap(args[0])
				if err != nil {
					return nil, err
				}
				return []Value{NewInteger(int64(len(m.Keys())))}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
	// A pure identity named `print` — exprHasEffect treats the NAME as
	// effectful, so the container const-fold declines and the map records
	// an OpMakeMap assembly instead of baking a const.
	r.RegisterNativeFunc(NativeFunc{
		Name: "print",
		Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				return []Value{args[0]}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})
}

func TestCompiledComputedMapArg(t *testing.T) {
	got := runDifferential(t, registerMapWords, func() []Value {
		om := NewOrderedMap()
		om.Set("a", NewParenExpr([]Value{NewWord("print"), NewInteger(5)}))
		om.Set("b", NewInteger(2))
		mv := NewMap(om)
		mv.Eval = true
		return []Value{NewWord("cmsize"), mv}
	})
	if got != "2" {
		t.Errorf("computed map size = %q", got)
	}
}

func TestCompiledConstFoldedMapArg(t *testing.T) {
	// A deterministic computed value const-folds at the top frame
	// (constFoldContainerVal / concreteEvalOnce / constFoldAgrees) and
	// the map bakes as a const.
	got := runDifferential(t, registerMapWords, func() []Value {
		om := NewOrderedMap()
		om.Set("a", NewParenExpr([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}))
		mv := NewMap(om)
		mv.Eval = true
		return []Value{NewWord("cmsize"), mv}
	})
	if got != "1" {
		t.Errorf("folded map size = %q", got)
	}
}

func TestCompiledMapResidual(t *testing.T) {
	// A computed map as the PROGRAM residual (not consumed): parity.
	runTolerant(t, registerMapWords, func() []Value {
		om := NewOrderedMap()
		om.Set("k", NewParenExpr([]Value{NewWord("print"), NewInteger(9)}))
		mv := NewMap(om)
		mv.Eval = true
		return []Value{mv}
	}, "{k:9}")
}

// --- loop-body list assembly (recordMakeListInner) --------------------------------------

func TestCompiledLoopBodyListArg(t *testing.T) {
	// A computed list consumed by clen INSIDE a loop body re-assembles
	// per iteration (OpMakeList in the loop unit).
	setup := func(r *Registry) {
		registerLoopWords(r)
	}
	runTolerant(t, setup, func() []Value {
		lv := NewList([]Value{NewWord("i"), NewInteger(5)})
		lv.Eval = true
		return []Value{
			NewWord("cfor"), NewInteger(3),
			codeBody(NewWord("clen"), lv),
		}
	}, "2 | 2 | 2")
}

// --- plain-check diagnostics (argTypeSummary / sigTypeSummary) ---------------------------

func TestPlainCheckNoSignatureSummaries(t *testing.T) {
	// A plain check pass (Mode on, NOT compiling) over a concrete
	// mismatched dispatch emits the got/nearest summary diagnostic.
	r := covRegistry(t, nil)
	done := r.Check.Begin()
	_, err := NewTop(r).Run([]Value{NewWord("cadd"), NewString("a"), NewString("b")})
	if err != nil {
		t.Fatalf("plain check errored hard: %v", err)
	}
	var found string
	for _, d := range r.Check.Diagnostics {
		if d.Code == "no_signature" && strings.Contains(d.Detail, "cadd") {
			found = d.Detail
			break
		}
	}
	done()
	if found == "" {
		t.Fatal("no no_signature diagnostic for cadd")
	}
	if !strings.Contains(found, "got (") || !strings.Contains(found, "nearest [") {
		t.Errorf("summary missing got/nearest: %q", found)
	}
}

// --- carrier no-match dispatch: interpreter-fallback parity ------------------------------

func TestCompiledCarrierDisjointTrapParity(t *testing.T) {
	// The failing operand is a CARRIER (a ccat result — String) in an
	// Integer-only slot. A carrier is not concrete at compile time, so a
	// trap built here could not carry a diagnostic matching the
	// interpreter's runtime (concrete-value) one; the compile therefore
	// DECLINES to trap and the program falls back to the interpreter. Both
	// engines raise the identical signature_error either way (phase 7).
	runErrParity(t, nil, func() []Value {
		return []Value{
			NewWord("cadd"),
			NewOpenParen(), NewWord("ccat"), NewString("a"), NewString("b"), NewCloseParen(),
			NewInteger(2),
		}
	}, "signature_error")
}

func TestCompiledConcreteTrapVoidGroup(t *testing.T) {
	// A void argument group (a paren producing no value) starves the
	// dispatch: the trap carries the void-group override taxonomy.
	setup := func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "cvoid",
			Signatures: []Signature{{
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
	}
	ri := covRegistry(t, setup)
	_, iErr := NewTop(ri).Run([]Value{
		NewWord("cneg"),
		NewOpenParen(), NewWord("cvoid"), NewCloseParen(),
	})
	if iErr == nil {
		t.Fatal("void arg group did not error")
	}
	rc := covRegistry(t, setup)
	prog, reason := compileTokens(t, rc, []Value{
		NewWord("cneg"),
		NewOpenParen(), NewWord("cvoid"), NewCloseParen(),
	})
	if prog == nil {
		t.Logf("compile refused (%s)", reason)
		return
	}
	_, cErr := RunProgram(prog, rc)
	if cErr == nil {
		t.Fatal("compiled void arg group did not error")
	}
}

// --- direct VM helper probes -----------------------------------------------------------

func TestInvokeClosureRawTokens(t *testing.T) {
	// invokeClosure with a NON-closure body value runs the raw tokens in
	// a sub-engine (the island path).
	r := covRegistry(t, nil)
	prog, reason := compileTokens(t, r, []Value{NewInteger(1)})
	if prog == nil {
		t.Fatalf("trivial compile refused: %s", reason)
	}
	vc := &vmContext{p: prog, r: r}
	body := NewList([]Value{NewWord("cadd")})
	out, err := vc.invokeClosure(r, body, []Value{NewInteger(2), NewInteger(3)})
	if err != nil {
		t.Fatalf("invokeClosure raw: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("raw closure out = %v", out)
	}
	if n, _ := AsInteger(out[0]); n != 5 {
		t.Errorf("raw closure result = %v", out[0])
	}
}

func TestIsAppliableFnShapes(t *testing.T) {
	fn := NewFunction(FnDefInfo{Name: "f"})
	if !isAppliableFn(fn) {
		t.Error("Function value not appliable")
	}
	if isAppliableFn(NewInteger(1)) {
		t.Error("integer appliable")
	}
	carrier := NewCarrier(TFunction)
	if !isAppliableFn(carrier) {
		t.Error("Function-typed carrier not appliable")
	}
	if !isFnTypedCarrier(carrier) {
		t.Error("Function carrier not fn-typed")
	}
	if isFnTypedCarrier(NewCarrier(TInteger)) {
		t.Error("Integer carrier fn-typed")
	}
	if !isFnValueResidual(NewFnDef(FnDefInfo{Name: "g"})) {
		t.Error("FnDef value not a fn residual")
	}
	if isFnValueResidual(NewString("s")) {
		t.Error("string a fn residual")
	}
}

func TestAnyDynamicTailHelpers(t *testing.T) {
	dyn := NewDynamicCarrier(TAny)
	static := NewInteger(1)
	if anyDynamicTail([]Value{dyn, static}) {
		t.Error("static tail read as dynamic")
	}
	if !anyDynamicTail([]Value{static, dyn}) {
		t.Error("dynamic tail missed")
	}
	fnv := NewFunction(FnDefInfo{Name: "f"})
	if !anyFnOrDynamicTail([]Value{static, fnv}) {
		t.Error("fn tail missed")
	}
	if anyFnOrDynamicTail([]Value{fnv, static}) {
		t.Error("leading fn read as tail")
	}
}
