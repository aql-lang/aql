package eng

import (
	"strings"
	"testing"
)

// Error-path and edge-branch coverage for the interpreter loop
// (engine.go), CheckState bookkeeping (check.go), index bounds checks
// (indexcheck.go), and the registry's dispatch helpers (registry.go).
// These are the paths lang's integration suites rarely reach.

// --- fn return contract ------------------------------------------------------

func TestFnReturnTypeMismatchErrors(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		// Body yields a String where the sig declares Integer.
		InstallFnDef(r, "badtype", FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       BORU([]Value{NewString("oops")}),
				BarrierPos: BarrierAllForward,
			}},
		})
	})
	_, err := NewTop(r).Run([]Value{NewWord("badtype"), NewInteger(1)})
	if err == nil {
		t.Fatal("wrong-typed return succeeded")
	}
	if !strings.Contains(err.Error(), "return") && !strings.Contains(err.Error(), "type") {
		t.Errorf("err = %v, want a return-type complaint", err)
	}
}

func TestFnReturnCountMismatchErrors(t *testing.T) {
	r := covRegistry(t, func(r *Registry) {
		// Body yields nothing where the sig declares one Integer.
		InstallFnDef(r, "badcount", FnDefInfo{
			Signatures: []Signature{{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       BORU([]Value{}),
				BarrierPos: BarrierAllForward,
			}},
		})
	})
	_, err := NewTop(r).Run([]Value{NewWord("badcount"), NewInteger(1)})
	if err == nil {
		t.Fatal("under-returning fn succeeded")
	}
}

// --- resource ceilings --------------------------------------------------------

func TestEvalLimitTripsCleanly(t *testing.T) {
	r := newTestRegistry(t)
	InstallFnDef(r, "spin2", FnDefInfo{
		Signatures: []Signature{{
			Params:     []FnParam{{Name: "n", Type: TInteger}},
			Returns:    []*Type{TInteger},
			Impl:       BORU([]Value{NewOpenParen(), NewWord("spin2"), NewWord("n"), NewCloseParen()}),
			BarrierPos: BarrierAllForward,
		}},
	})
	e := NewTop(r)
	e.stepLimit = 500
	_, err := e.Run([]Value{NewWord("spin2"), NewInteger(0)})
	if err == nil {
		t.Fatal("runaway loop returned nil error")
	}
	if !strings.Contains(err.Error(), "evaluation_limit") {
		t.Errorf("err = %v, want evaluation_limit", err)
	}
}

// --- malformed token streams ---------------------------------------------------

func TestUnbalancedCloseParenErrors(t *testing.T) {
	r := covRegistry(t, nil)
	_, err := NewTop(r).Run([]Value{NewInteger(1), NewCloseParen()})
	if err == nil {
		t.Fatal("stray close paren accepted")
	}
}

func TestUnterminatedOpenParen(t *testing.T) {
	// An open paren with no close must not panic; error or residual both
	// prove the loop handled it deliberately.
	r := covRegistry(t, nil)
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("unterminated paren panicked: %v", rec)
		}
	}()
	_, _ = NewTop(r).Run([]Value{NewOpenParen(), NewInteger(1)})
}

func TestEndTokenSeparatesStatements(t *testing.T) {
	r := covRegistry(t, nil)
	out, err := NewTop(r).Run([]Value{NewInteger(1), NewEnd(), NewInteger(2)})
	if err != nil {
		t.Fatalf("end token: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d results, want 2", len(out))
	}
}

func TestSpliceExpandsIntoStream(t *testing.T) {
	r := covRegistry(t, nil)
	// A splice of a plain list contributes its elements: cadd receives 2 3.
	out, err := NewTop(r).Run([]Value{
		NewSplice(NewList([]Value{NewInteger(2), NewInteger(3)})),
		NewWord("cadd"),
	})
	if err != nil {
		t.Fatalf("splice run: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %v, want one value", out)
	}
	if got, _ := AsInteger(out[0]); got != 5 {
		t.Errorf("spliced add = %v, want 5", out[0])
	}
	// A splice of a non-list contributes itself.
	out, err = NewTop(r).Run([]Value{NewSplice(NewInteger(9))})
	if err != nil {
		t.Fatalf("scalar splice: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 9 {
		t.Errorf("scalar splice = %v, want 9", out[0])
	}
}

func TestInterpStringEvaluatesParts(t *testing.T) {
	r := covRegistry(t, nil)
	is := NewInterpString([]InterpPart{
		{Lit: "sum="},
		{Expr: []Value{NewWord("cadd"), NewInteger(1), NewInteger(2)}},
		{Lit: "!"},
	})
	out, err := NewTop(r).Run([]Value{is})
	if err != nil {
		t.Fatalf("interp string: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %v, want one string", out)
	}
	s, err := AsString(out[0])
	if err != nil {
		t.Fatalf("interp result not a string: %v", err)
	}
	if s != "sum=3!" {
		t.Errorf("interp = %q, want sum=3!", s)
	}

	// Negative: an expression part that errors surfaces the error.
	bad := NewInterpString([]InterpPart{{Expr: []Value{NewWord("no_such_word")}}})
	if _, err := NewTop(r).Run([]Value{bad}); err == nil {
		t.Error("interp string with undefined word succeeded")
	}
}

func TestMoveToMissingMarkErrors(t *testing.T) {
	r := covRegistry(t, nil)
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("move without mark panicked: %v", rec)
		}
	}()
	_, err := NewTop(r).Run([]Value{NewMove("nowhere", "test")})
	if err == nil {
		t.Error("move to a missing mark succeeded silently")
	}
}

func TestUndefinedWordErrorMentionsName(t *testing.T) {
	r := covRegistry(t, nil)
	_, err := NewTop(r).Run([]Value{NewWord("definitely_not_defined")})
	if err == nil {
		t.Fatal("undefined word succeeded")
	}
	if !strings.Contains(err.Error(), "definitely_not_defined") {
		t.Errorf("err %v does not name the missing word", err)
	}
}

// --- CallBORU -------------------------------------------------------------------

func TestCallBORUDirect(t *testing.T) {
	r := covRegistry(t, nil)
	sig := &FnSig{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    BORU([]Value{NewOpenParen(), NewWord("cadd"), NewWord("n"), NewWord("n"), NewCloseParen()}),
	}
	out, err := r.CallBORU(sig, []Value{NewInteger(6)}, nil)
	if err != nil {
		t.Fatalf("CallBORU: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("CallBORU results = %v", out)
	}
	if got, _ := AsInteger(out[0]); got != 12 {
		t.Errorf("CallBORU = %v, want 12", out[0])
	}

	// Captures are visible to the body and shadowed by params.
	capSig := &FnSig{
		Params:  []FnParam{{Name: "n", Type: TInteger}},
		Returns: []*Type{TInteger},
		Impl:    BORU([]Value{NewOpenParen(), NewWord("cadd"), NewWord("n"), NewWord("base"), NewCloseParen()}),
	}
	out, err = r.CallBORU(capSig, []Value{NewInteger(1)}, []CapturedBinding{{Name: "base", Value: NewInteger(100)}})
	if err != nil {
		t.Fatalf("CallBORU with capture: %v", err)
	}
	if got, _ := AsInteger(out[0]); got != 101 {
		t.Errorf("captured call = %v, want 101", out[0])
	}

	// Negative: a body that errors propagates.
	badSig := &FnSig{
		Params: []FnParam{{Name: "n", Type: TInteger}},
		Impl:   BORU([]Value{NewWord("no_such_word")}),
	}
	if _, err := r.CallBORU(badSig, []Value{NewInteger(1)}, nil); err == nil {
		t.Error("erroring body succeeded via CallBORU")
	}
}

// --- registry helpers ------------------------------------------------------------

func TestRegistryGensymAndNames(t *testing.T) {
	r := covRegistry(t, nil)
	a, b := r.NextGensym(), r.NextGensym()
	if a == b {
		t.Errorf("NextGensym repeated %q", a)
	}
	names := r.RegisteredWordNames()
	found := false
	for _, n := range names {
		if n == "cadd" {
			found = true
			break
		}
	}
	if !found {
		t.Error("RegisteredWordNames missing cadd")
	}
	r.MarkReady()

	// No engine running: no current stack.
	if _, ok := r.CurrentStack(); ok {
		t.Error("CurrentStack claims a stack outside a run")
	}
}

func TestContextStore(t *testing.T) {
	r := newTestRegistry(t)
	if _, ok := ContextStoreLookup(r, "missing"); ok {
		t.Error("empty context store found a key")
	}
	r.ContextSet("greeting", NewString("hi"))
	v, ok := ContextStoreLookup(r, "greeting")
	if !ok {
		t.Fatal("context key lost")
	}
	if s, _ := AsString(v); s != "hi" {
		t.Errorf("context value = %v", v)
	}
}

func TestStoreKeyKinds(t *testing.T) {
	if k := StoreKey(NewString("s")); k == "" {
		t.Error("StoreKey(string) empty")
	}
	if k := StoreKey(NewAtom("a")); k == "" {
		t.Error("StoreKey(atom) empty")
	}
	if StoreKey(NewString("x")) == StoreKey(NewString("y")) {
		t.Error("distinct strings share a store key")
	}
}

func TestNumOpNativeBuilders(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.RegisterNativeFunc(UnaryNumOpNative("cabs", func(f float64) float64 {
		if f < 0 {
			return -f
		}
		return f
	}))
	r.RegisterNativeFunc(BinaryNumOpNative("cdiv", func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, MakeBoruError("division_by_zero", "cdiv by zero", "cdiv", "", "")
		}
		return a / b, nil
	}))
	r.RegisterNativeFunc(BinaryIntOpNative("cmod", func(a, b int64) (int64, error) {
		if b == 0 {
			return 0, MakeBoruError("division_by_zero", "cmod by zero", "cmod", "", "")
		}
		return a % b, nil
	}))
	if err := r.Err(); err != nil {
		t.Fatalf("registration: %v", err)
	}
	r.InitRootContext()

	out, err := NewTop(r).Run([]Value{NewWord("cabs"), NewFloat(-2.5)})
	if err != nil {
		t.Fatalf("cabs: %v", err)
	}
	if f, _ := AsFloat(out[0]); f != 2.5 {
		t.Errorf("cabs = %v", out[0])
	}
	out, err = NewTop(r).Run([]Value{NewWord("cdiv"), NewFloat(6), NewFloat(3)})
	if err != nil {
		t.Fatalf("cdiv: %v", err)
	}
	if f, _ := AsFloat(out[0]); f != 2 {
		t.Errorf("cdiv = %v", out[0])
	}
	out, err = NewTop(r).Run([]Value{NewWord("cmod"), NewInteger(7), NewInteger(4)})
	if err != nil {
		t.Fatalf("cmod: %v", err)
	}
	if n, _ := AsInteger(out[0]); n != 3 {
		t.Errorf("cmod = %v", out[0])
	}
	// The error path propagates.
	if _, err := NewTop(r).Run([]Value{NewWord("cmod"), NewInteger(7), NewInteger(0)}); err == nil {
		t.Error("cmod by zero succeeded")
	}
}

// --- check-state bookkeeping -------------------------------------------------------

func TestUnusedDefDiagnostics(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	r.Check.RecordDef("neverused", SrcPos{Row: 3, Col: 1})
	r.Check.RecordDef("used", SrcPos{Row: 4, Col: 1})
	r.Check.RecordUse("used")
	r.Check.EmitUnusedDefDiagnostics()
	diags := r.Check.Diagnostics
	done()

	foundUnused := false
	for _, d := range diags {
		if strings.Contains(d.Detail, "neverused") {
			foundUnused = true
		}
		if strings.Contains(d.Detail, `"used"`) {
			t.Errorf("used def flagged: %+v", d)
		}
	}
	if !foundUnused {
		t.Errorf("unused def not flagged; diags = %+v", diags)
	}

	// Underscore-prefixed names are exempt.
	done2 := r.Check.Begin()
	r.Check.RecordDef("_internal", SrcPos{})
	r.Check.EmitUnusedDefDiagnostics()
	for _, d := range r.Check.Diagnostics {
		if strings.Contains(d.Detail, "_internal") {
			t.Errorf("underscore def flagged: %+v", d)
		}
	}
	done2()
}

func TestTruncateDiagnostics(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "no_signature", Detail: "one"})
	r.Check.AddDiagnostic(CheckDiagnostic{Code: "no_signature", Detail: "two"})
	if len(r.Check.Diagnostics) != 2 {
		t.Fatalf("diag count = %d", len(r.Check.Diagnostics))
	}
	r.Check.TruncateDiagnostics(1)
	if len(r.Check.Diagnostics) != 1 || r.Check.Diagnostics[0].Detail != "one" {
		t.Errorf("truncate result = %+v", r.Check.Diagnostics)
	}
	// Out-of-range truncations are no-ops.
	r.Check.TruncateDiagnostics(5)
	r.Check.TruncateDiagnostics(-1)
	if len(r.Check.Diagnostics) != 1 {
		t.Errorf("no-op truncate changed diagnostics: %+v", r.Check.Diagnostics)
	}
}

func TestCheckListIndexBounds(t *testing.T) {
	r := newTestRegistry(t)
	done := r.Check.Begin()
	defer done()
	list := NewList([]Value{NewInteger(10), NewInteger(20)})

	// In-range: silent.
	CheckListIndex(r, NewInteger(1), list, "get")
	if len(r.Check.Diagnostics) != 0 {
		t.Fatalf("in-range index flagged: %+v", r.Check.Diagnostics)
	}
	// Provably out of range: flagged.
	CheckListIndex(r, NewInteger(9), list, "get")
	if len(r.Check.Diagnostics) == 0 {
		t.Fatal("out-of-range index not flagged")
	}

	n := len(r.Check.Diagnostics)
	// `at` over a list of indices: one bad index flags once.
	CheckAtIndices(r, NewList([]Value{NewInteger(0), NewInteger(7)}), list, "at")
	if len(r.Check.Diagnostics) != n+1 {
		t.Errorf("at-indices diagnostics = %d, want %d", len(r.Check.Diagnostics), n+1)
	}
	// Non-concrete indices are skipped without panic.
	CheckAtIndices(r, NewTypeLiteral(TList), list, "at")
}

func TestSkipsSideEffect(t *testing.T) {
	r := newTestRegistry(t)
	if r.Check.SkipsSideEffect() {
		t.Error("side effects suppressed outside check mode")
	}
	done := r.Check.Begin()
	if !r.Check.SkipsSideEffect() {
		t.Error("side effects not suppressed in check mode")
	}
	done()
}
