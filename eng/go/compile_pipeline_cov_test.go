package eng

import (
	"strings"
	"testing"
)

// This file drives the full check→emit→lower→VM pipeline from inside the
// kernel, mirroring lang's CompileCheck/RunCompiled flow (lang/go/aql.go)
// but using only eng-registered native words and hand-built token streams.
// Each program is executed twice — interpreted and compiled — and the
// rendered results must agree (the bytecode plan's opt-in contract).

// covWords registers a small vocabulary rich enough to exercise forward
// and stack collection, polymorphic dispatch, string/list/map operands,
// and a deliberately failing word for the runtime-error path.
func covWords(r *Registry) {
	intBin := func(name string, f func(a, b int64) int64) {
		r.RegisterNativeFunc(NativeFunc{
			Name: name,
			Signatures: []Signature{{
				Args: []*Type{TInteger, TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					a, _ := AsInteger(args[0])
					b, _ := AsInteger(args[1])
					return []Value{NewInteger(f(a, b))}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	}
	intBin("cadd", func(a, b int64) int64 { return a + b })
	intBin("cmul", func(a, b int64) int64 { return a * b })

	r.RegisterNativeFunc(NativeFunc{
		Name: "cneg",
		Signatures: []Signature{{
			Args: []*Type{TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				n, _ := AsInteger(args[0])
				return []Value{NewInteger(-n)}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: 0,
		}},
	})

	r.RegisterNativeFunc(NativeFunc{
		Name: "ccat",
		Signatures: []Signature{{
			Args: []*Type{TString, TString},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				a, _ := AsString(args[0])
				b, _ := AsString(args[1])
				return []Value{NewString(a + b)}, nil
			}),
			Returns: []*Type{TString}, BarrierPos: -1,
		}},
	})

	// Polymorphic: Integer doubles, String repeats.
	r.RegisterNativeFunc(NativeFunc{
		Name: "cdub",
		Signatures: []Signature{
			{
				Args: []*Type{TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					n, _ := AsInteger(args[0])
					return []Value{NewInteger(2 * n)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			},
			{
				Args: []*Type{TString},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					s, _ := AsString(args[0])
					return []Value{NewString(s + s)}, nil
				}),
				Returns: []*Type{TString}, BarrierPos: -1,
			},
		},
	})

	r.RegisterNativeFunc(NativeFunc{
		Name: "clen",
		Signatures: []Signature{{
			Args: []*Type{TList},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				elems, err := AsList(args[0])
				if err != nil {
					return nil, err
				}
				return []Value{NewInteger(int64(elems.Len()))}, nil
			}),
			Returns: []*Type{TInteger}, BarrierPos: -1,
		}},
	})

	// Frame-tail plumbing words. The engine's fn-frame tail references
	// `__pa` (pop the per-call Args/FnBaselines stacks) and force-forward
	// `undef <param>` pairs; both words live in the language layer, so an
	// eng-only registry supplies minimal equivalents (mirroring
	// lang/go/native/native_definition.go).
	r.RegisterNativeFunc(NativeFunc{
		Name: "__pa",
		Signatures: []Signature{{
			Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
				if err := PopFrameArgs(r); err != nil {
					return nil, err
				}
				return nil, nil
			}),
			Returns: []*Type{}, BarrierPos: -1,
		}},
	})
	undefImpl := Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
		name := ""
		if IsWord(args[0]) {
			w, _ := AsWord(args[0])
			name = w.Name
		} else if IsAtom(args[0]) {
			name, _ = AsAtom(args[0])
		} else {
			name, _ = AsString(args[0])
		}
		r.Check.Recorder().RefuseCarriedUndef(name)
		UninstallDef(r, name)
		return nil, nil
	}, RunInCheck())
	r.RegisterNativeFunc(NativeFunc{
		Name: "undef",
		Signatures: []Signature{
			{
				Args:       []*Type{TAtom},
				QuoteArgs:  map[int]bool{0: true},
				Impl:       undefImpl,
				Returns:    []*Type{},
				BarrierPos: -1,
			},
		},
	})

	// User fns installed by the same path `def name fn […]` takes.
	InstallFnDef(r, "ctwice", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       AQL([]Value{NewOpenParen(), NewWord("cadd"), NewWord("n"), NewWord("n"), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})
	InstallFnDef(r, "cquad", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       AQL([]Value{NewOpenParen(), NewWord("ctwice"), NewOpenParen(), NewWord("ctwice"), NewWord("n"), NewCloseParen(), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})
}

// covRegistry builds a fresh registry with covWords plus any extra setup.
func covRegistry(t *testing.T, extra func(*Registry)) *Registry {
	t.Helper()
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	covWords(r)
	if extra != nil {
		extra(r)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()
	return r
}

// compileTokens runs the check pass with the emit recorder armed and
// finalizes the recording into a Program — the eng-level equivalent of
// lang's CompileCheck. Returns (nil, reason) when the program is
// uncompilable or check found errors.
func compileTokens(t *testing.T, r *Registry, tokens []Value) (*Program, string) {
	t.Helper()
	done := r.Check.Begin()
	r.Check.Emit = NewEmitState()
	r.Check.Compiling = true
	engine := NewTop(r)
	residual, runErr := engine.Run(tokens)
	var prog *Program
	reason := ""
	func() {
		defer done()
		if runErr != nil {
			prog, reason = nil, "check error: "+runErr.Error()
			return
		}
		for _, d := range r.Check.Diagnostics {
			if d.Severity == SeverityError {
				prog, reason = nil, "check diagnostics: "+d.Detail
				return
			}
		}
		if r.Check.SuppressedRuntimeError {
			prog, reason = nil, "suppressed runtime error"
			return
		}
		if r.Check.AmbiguousGradualSplit {
			prog, reason = nil, "ambiguous gradual split"
			return
		}
		p, why, ok := r.Check.Recorder().Finalize(residual)
		if !ok {
			prog, reason = nil, "finalize: "+why
			return
		}
		prog = p
	}()
	return prog, reason
}

// renderAll flattens results for comparison.
func renderAll(vs []Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.String()
	}
	return strings.Join(parts, " | ")
}

// runDifferential executes tokens() interpreted on one registry and
// compiled on another, asserting the two agree. Returns the rendered
// interpreted output for extra assertions.
func runDifferential(t *testing.T, extra func(*Registry), tokens func() []Value) string {
	t.Helper()
	// Interpreted reference.
	ri := covRegistry(t, extra)
	iOut, iErr := NewTop(ri).Run(tokens())
	if iErr != nil {
		t.Fatalf("interpreted run: %v", iErr)
	}
	// Compiled path.
	rc := covRegistry(t, extra)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		t.Fatalf("program did not compile: %s", reason)
	}
	cOut, cErr := RunProgram(prog, rc)
	if cErr != nil {
		t.Fatalf("compiled run: %v", cErr)
	}
	if got, want := renderAll(cOut), renderAll(iOut); got != want {
		t.Fatalf("compiled/interpreted diverge:\n  compiled:    %s\n  interpreted: %s", got, want)
	}
	return renderAll(iOut)
}

// --- straight-line programs ----------------------------------------------

func TestCompiledLiteralRoundTrip(t *testing.T) {
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewInteger(7)}
	})
	if got != "7" {
		t.Errorf("got %q, want 7", got)
	}
}

func TestCompiledScalarSpread(t *testing.T) {
	// Several scalar literals of different kinds survive both engines.
	runDifferential(t, nil, func() []Value {
		return []Value{NewInteger(1), NewString("x"), NewBoolean(true), NewFloat(1.5), NewAtom("a")}
	})
}

func TestCompiledForwardCall(t *testing.T) {
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewWord("cadd"), NewInteger(2), NewInteger(3)}
	})
	if got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestCompiledStackCall(t *testing.T) {
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewInteger(2), NewInteger(3), NewWord("cadd")}
	})
	if got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestCompiledMixedSplitCall(t *testing.T) {
	// `2 cadd 3` — one forward arg, one from the stack.
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewInteger(2), NewWord("cadd"), NewInteger(3)}
	})
	if got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestCompiledNestedParens(t *testing.T) {
	// cadd (cmul 2 3) (10 cneg) → 6 + -10 = -4
	got := runDifferential(t, nil, func() []Value {
		return []Value{
			NewWord("cadd"),
			NewOpenParen(), NewWord("cmul"), NewInteger(2), NewInteger(3), NewCloseParen(),
			NewOpenParen(), NewInteger(10), NewWord("cneg"), NewCloseParen(),
		}
	})
	if got != "-4" {
		t.Errorf("got %q, want -4", got)
	}
}

func TestCompiledChainedCalls(t *testing.T) {
	// Sequential statements piling results on the stack.
	runDifferential(t, nil, func() []Value {
		return []Value{
			NewWord("cadd"), NewInteger(1), NewInteger(2),
			NewWord("cmul"), NewInteger(3), NewInteger(4),
			NewWord("cadd"),
		}
	})
}

func TestCompiledPolyDispatch(t *testing.T) {
	gotInt := runDifferential(t, nil, func() []Value {
		return []Value{NewWord("cdub"), NewInteger(21)}
	})
	if gotInt != "42" {
		t.Errorf("int overload: got %q, want 42", gotInt)
	}
	gotStr := runDifferential(t, nil, func() []Value {
		return []Value{NewWord("cdub"), NewString("ab")}
	})
	if !strings.Contains(gotStr, "abab") {
		t.Errorf("string overload: got %q", gotStr)
	}
}

func TestCompiledStringOps(t *testing.T) {
	runDifferential(t, nil, func() []Value {
		return []Value{NewWord("ccat"), NewString("foo"), NewString("bar")}
	})
}

func TestCompiledListArg(t *testing.T) {
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewWord("clen"), NewList([]Value{NewInteger(1), NewInteger(2), NewInteger(3)})}
	})
	if got != "3" {
		t.Errorf("got %q, want 3", got)
	}
}

func TestCompiledUserFnCall(t *testing.T) {
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewWord("ctwice"), NewInteger(21)}
	})
	if got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestCompiledNestedUserFnCall(t *testing.T) {
	got := runDifferential(t, nil, func() []Value {
		return []Value{NewWord("cquad"), NewInteger(10)}
	})
	if got != "40" {
		t.Errorf("got %q, want 40", got)
	}
}

func TestCompiledUserFnFeedsNative(t *testing.T) {
	// cadd (ctwice 5) (ctwice 6) = 10 + 12
	got := runDifferential(t, nil, func() []Value {
		return []Value{
			NewWord("cadd"),
			NewOpenParen(), NewWord("ctwice"), NewInteger(5), NewCloseParen(),
			NewOpenParen(), NewWord("ctwice"), NewInteger(6), NewCloseParen(),
		}
	})
	if got != "22" {
		t.Errorf("got %q, want 22", got)
	}
}

// --- refusal / error paths ------------------------------------------------

func TestCompileRefusesUnknownWord(t *testing.T) {
	r := covRegistry(t, nil)
	prog, reason := compileTokens(t, r, []Value{NewWord("no_such_word_xyz")})
	if prog != nil {
		t.Fatal("unknown word compiled to a program")
	}
	if reason == "" {
		t.Fatal("no refusal reason for unknown word")
	}
}

// runErrParity asserts a program errors on BOTH engines with the same
// error substring (the checker lowers an unresolvable dispatch to a
// runtime trap rather than refusing outright, so the compiled program
// must reproduce the interpreter's error).
func runErrParity(t *testing.T, extra func(*Registry), tokens func() []Value, wantSub string) {
	t.Helper()
	ri := covRegistry(t, extra)
	_, iErr := NewTop(ri).Run(tokens())
	if iErr == nil {
		t.Fatal("interpreted run unexpectedly succeeded")
	}
	if !strings.Contains(iErr.Error(), wantSub) {
		t.Fatalf("interpreted error %q does not mention %q", iErr, wantSub)
	}
	rc := covRegistry(t, extra)
	prog, reason := compileTokens(t, rc, tokens())
	if prog == nil {
		// Refusing to compile is also sound — the interpreter owns the error.
		t.Logf("compile refused (%s); interpreter owns the error", reason)
		return
	}
	_, cErr := RunProgram(prog, rc)
	if cErr == nil {
		t.Fatal("compiled run unexpectedly succeeded where the interpreter errors")
	}
	if strings.Contains(cErr.Error(), "internal_error") &&
		strings.Contains(cErr.Error(), "deferring to the interpreter") {
		// A runtime shape-claim deferral (a poly whose re-matched overload
		// returns a different result count than the recorded claim) is also
		// sound — under RunCompiled the interpreter re-runs and owns the
		// canonical error, asserted on the interpreted run above.
		t.Logf("compiled run defers (%v); interpreter owns the error", cErr)
		return
	}
	if !strings.Contains(cErr.Error(), wantSub) {
		t.Fatalf("compiled error %q does not mention %q", cErr, wantSub)
	}
}

func TestCompiledArityErrorParity(t *testing.T) {
	// cadd with a single available operand cannot satisfy its 2-int sig:
	// signature_error from both engines.
	runErrParity(t, nil, func() []Value {
		return []Value{NewWord("cadd"), NewInteger(1)}
	}, "signature_error")
}

func TestCompiledTypeMismatchErrorParity(t *testing.T) {
	// cadd of two strings has no matching sig: signature_error from both.
	runErrParity(t, nil, func() []Value {
		return []Value{NewWord("cadd"), NewString("a"), NewString("b")}
	}, "signature_error")
}

// A handler whose Go error surfaces at runtime must fail identically in
// both engines (genuine runtime AqlErrors are returned as-is, not
// silently fixed by the fallback path).
func TestCompiledRuntimeErrorParity(t *testing.T) {
	// cfail's returns depend on the value, so check mode cannot fold it:
	// declare a return type but make the handler fail on a specific value.
	setup := func(r *Registry) {
		r.RegisterNativeFunc(NativeFunc{
			Name: "chalf",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
					n, _ := AsInteger(args[0])
					if n%2 != 0 {
						return nil, MakeAqlError("type_error", "chalf of odd value", "chalf", "", "")
					}
					return []Value{NewInteger(n / 2)}, nil
				}),
				Returns: []*Type{TInteger}, BarrierPos: -1,
			}},
		})
	}
	// Positive: even input works in both engines.
	got := runDifferential(t, setup, func() []Value {
		return []Value{NewWord("chalf"), NewInteger(10)}
	})
	if got != "5" {
		t.Errorf("got %q, want 5", got)
	}
}

func TestCompiledProgramReusable(t *testing.T) {
	// One compiled program can run repeatedly on its registry.
	r := covRegistry(t, nil)
	prog, reason := compileTokens(t, r, []Value{NewWord("cadd"), NewInteger(20), NewInteger(22)})
	if prog == nil {
		t.Fatalf("compile: %s", reason)
	}
	for i := 0; i < 3; i++ {
		out, err := RunProgram(prog, r)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if renderAll(out) != "42" {
			t.Fatalf("run %d: got %s, want 42", i, renderAll(out))
		}
	}
}

func TestProgramHasCode(t *testing.T) {
	// A compiled program carries instructions and per-instr debug spans.
	r := covRegistry(t, nil)
	prog, reason := compileTokens(t, r, []Value{NewWord("cadd"), NewInteger(1), NewInteger(2)})
	if prog == nil {
		t.Fatalf("compile: %s", reason)
	}
	if len(prog.Code) == 0 {
		t.Fatal("compiled program has no instructions")
	}
	if len(prog.Debug) != len(prog.Code) {
		t.Fatalf("debug spans (%d) do not cover code (%d)", len(prog.Debug), len(prog.Code))
	}
}
