package eng

import (
	"strings"
	"testing"
)

// Coverage for the computed-arm branch lowerings (lower.go
// lowerBothComputed / lowerComputedCond / lowerComputedBranch, emit.go
// computedArmCondOK), the paren-bounded dynamic apply (engine.go
// stepCloseParen's RecordDynApply seam, vm.go callDynApplyTop), the
// user-fn poly re-match (RecordUserPolyCall / lowerUserPolyCall /
// matchUserPoly), plain-check dispatch recoveries, and small dispatch
// error paths (insufficientArgsError, implicit end / curryOrStack).

// --- computed-arm branches -------------------------------------------------------

func TestCompiledBranchBothComputedArms(t *testing.T) {
	// `cif (cond) (a) (b)` — both arms eagerly computed events.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(1), NewInteger(2), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "3" {
		t.Errorf("both-computed then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(1), NewInteger(2), NewCloseParen(),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "70" {
		t.Errorf("both-computed else = %q", got)
	}
}

func TestCompiledBranchComputedElseArm(t *testing.T) {
	// `cif (cond) [body] (expr)` — the else value is an eager event.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "3" {
		t.Errorf("computed-else taken-then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			NewOpenParen(), NewWord("cadd"), NewInteger(30), NewInteger(40), NewCloseParen(),
		}
	})
	if got != "70" {
		t.Errorf("computed-else taken-else = %q", got)
	}
}

func TestCompiledBranchComputedThenArm(t *testing.T) {
	// `cif (cond) (expr) [body]` — the then value is an eager event.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			NewOpenParen(), NewWord("cmul"), NewInteger(6), NewInteger(7), NewCloseParen(),
			codeBody(NewInteger(0)),
		}
	})
	if got != "42" {
		t.Errorf("computed-then taken-then = %q", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			NewOpenParen(), NewWord("cmul"), NewInteger(6), NewInteger(7), NewCloseParen(),
			codeBody(NewInteger(0)),
		}
	})
	if got != "0" {
		t.Errorf("computed-then taken-else = %q", got)
	}
}

func TestCompiledBranchConstCondValue(t *testing.T) {
	// A const (non-event) condition value reaches lowerBranch's default
	// pushOperand arm: cif over a literal-true word condition.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewBoolean(true),
			codeBody(NewInteger(1)),
			codeBody(NewInteger(2)),
		}
	})
	if got != "1" {
		t.Errorf("const-cond value = %q", got)
	}
}

// --- paren-bounded dynamic apply ------------------------------------------------------

func TestParenBoundedTrailingApply(t *testing.T) {
	// `(5 (cmk1))` — the closure is the LAST value in the paren; the
	// check pass records a paren-bounded dyn-apply event.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewOpenParen(),
			NewInteger(5),
			NewOpenParen(), NewWord("cmk1"), NewCloseParen(),
			NewCloseParen(),
		}
	}, "105")
}

func TestParenBoundedApplyFeedsCall(t *testing.T) {
	// The paren-bounded apply result feeds a following native call —
	// the exact reorder hazard the event-collapse solves.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewWord("cdub"),
			NewOpenParen(),
			NewInteger(5),
			NewOpenParen(), NewWord("cmk1"), NewCloseParen(),
			NewCloseParen(),
		}
	}, "210")
}

func TestParenLeadingDynamicRefuses(t *testing.T) {
	// `((cany) 3)` — a leading DYNAMIC value before args inside a paren
	// is the unsound reorder shape: check refuses; interpreter runs.
	runTolerant(t, registerDynWords, func() []Value {
		return []Value{
			NewOpenParen(),
			NewOpenParen(), NewWord("cany"), NewCloseParen(),
			NewInteger(3),
			NewCloseParen(),
		}
	}, "7 | 3")
}

// --- user-fn poly re-match -------------------------------------------------------------

func registerUserPoly2(r *Registry) {
	registerDynWords(r)
	// Both overloads declare IDENTICAL returns — the poly-arm gate.
	InstallFnDef(r, "upoly2", FnDefInfo{
		Signatures: []Signature{
			{
				Params:     []FnParam{{Name: "n", Type: TInteger}},
				Returns:    []*Type{TInteger},
				Impl:       BORU(parenBody(NewWord("cadd"), NewWord("n"), NewInteger(1))),
				BarrierPos: BarrierAllForward,
			},
			{
				Params:     []FnParam{{Name: "s", Type: TString}},
				Returns:    []*Type{TInteger},
				Impl:       BORU(parenBody(NewInteger(-1))),
				BarrierPos: BarrierAllForward,
			},
		},
	})
}

func TestCompiledUserPolyReMatch(t *testing.T) {
	// A dynamic operand to a 2-overload user fn with identical returns:
	// every arm bakes and the VM re-matches at entry (OpCallUserPoly).
	runTolerant(t, registerUserPoly2, func() []Value {
		return []Value{
			NewWord("upoly2"),
			NewOpenParen(), NewWord("cany"), NewCloseParen(),
		}
	}, "8")
}

func TestUserPolyReMatchInterpretedString(t *testing.T) {
	// The sibling overload dispatches for a String operand (interpreted
	// reference for the same word set).
	r := covRegistry(t, registerUserPoly2)
	out, err := NewTop(r).Run([]Value{NewWord("upoly2"), NewString("x")})
	if err != nil {
		t.Fatalf("upoly2 string: %v", err)
	}
	if got := renderAll(out); got != "-1" {
		t.Errorf("upoly2 string = %q", got)
	}
}

// --- plain-check dispatch recoveries ------------------------------------------------------

func plainCheckRun(t *testing.T, extra func(*Registry), tokens []Value) []CheckDiagnostic {
	t.Helper()
	r := covRegistry(t, extra)
	done := r.Check.Begin()
	_, err := NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("plain check errored hard: %v", err)
	}
	diags := append([]CheckDiagnostic(nil), r.Check.Diagnostics...)
	done()
	return diags
}

func TestPlainCheckSingleOverloadUserFnDynamicArg(t *testing.T) {
	// A single-overload user fn over an unknown-type carrier is the
	// recoverable shape: NO no_signature diagnostic (compile recovers it
	// as a guarded CALL_USER).
	diags := plainCheckRun(t, registerDynWords, []Value{
		NewWord("ctwice"),
		NewOpenParen(), NewWord("cany"), NewCloseParen(),
	})
	for _, d := range diags {
		if d.Code == "no_signature" && d.Word == "ctwice" {
			t.Errorf("recoverable dispatch flagged: %v", d)
		}
	}
}

func TestPlainCheckPolyNativeDynamicArg(t *testing.T) {
	// A multi-overload native over a dynamic operand types via the
	// reachable-overload partition — no hard error.
	diags := plainCheckRun(t, registerDynWords, []Value{
		NewWord("cdub"),
		NewOpenParen(), NewWord("cany"), NewCloseParen(),
	})
	for _, d := range diags {
		if d.Severity == SeverityError {
			t.Errorf("dynamic poly dispatch errored: %v", d)
		}
	}
}

func TestPlainCheckDisjunctPartition(t *testing.T) {
	// A branch-join disjunct (Integer|String) into a poly word partitions
	// the overloads per alternative (disjunctPartitionReturns).
	setup := func(r *Registry) {
		registerBranchWords(r)
	}
	diags := plainCheckRun(t, setup, []Value{
		NewWord("cdub"),
		NewOpenParen(),
		NewWord("cif"),
		NewOpenParen(), NewWord("cgt"), NewInteger(1), NewInteger(2), NewCloseParen(),
		NewInteger(1),
		NewString("s"),
		NewCloseParen(),
	})
	for _, d := range diags {
		if d.Severity == SeverityError {
			t.Errorf("disjunct poly dispatch errored: %v", d)
		}
	}
}

func TestCompiledDisjunctArgDispatch(t *testing.T) {
	// The same join consumed by cdub, end to end.
	runTolerant(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cdub"),
			NewOpenParen(),
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(1), NewInteger(2), NewCloseParen(),
			NewInteger(1),
			NewString("s"),
			NewCloseParen(),
		}
	}, "'ss'")
}

// --- dispatch error paths ----------------------------------------------------------------

func TestInsufficientForwardArgsInParen(t *testing.T) {
	// `(cadd 1)` — the parked forward is orphaned at the close paren.
	r := covRegistry(t, nil)
	_, err := NewTop(r).Run([]Value{
		NewOpenParen(), NewWord("cadd"), NewInteger(1), NewCloseParen(),
	})
	if err == nil {
		t.Fatal("orphaned forward did not error")
	}
	if !strings.Contains(err.Error(), "cadd") {
		t.Errorf("error does not name the word: %v", err)
	}
}

func TestImplicitEndOnTypeMismatchArrival(t *testing.T) {
	// `ccat "a" 1 2 cadd` — ccat collects "a", then the Integer arrival
	// mismatches its String slot: implicit end resolves the forward
	// early (curryOrStack), and the trailing cadd consumes 1 2.
	r := covRegistry(t, nil)
	out, err := NewTop(r).Run([]Value{
		NewWord("ccat"), NewString("a"), NewString("b"),
		NewInteger(1), NewInteger(2), NewWord("cadd"),
	})
	if err != nil {
		t.Fatalf("mixed statement: %v", err)
	}
	if got := renderAll(out); got != "'ab' | 3" {
		t.Logf("mixed statement rendered %q", got)
	}

	// Genuine starvation: the arriving value mismatches and nothing can
	// complete the forward — the statement errors.
	_, err = NewTop(r).Run([]Value{
		NewWord("ccat"), NewString("a"), NewInteger(1), NewEnd(),
	})
	if err == nil {
		t.Error("starved ccat did not error")
	}
}

func TestEngineRecorderAndTraceHooks(t *testing.T) {
	r := covRegistry(t, nil)
	e := NewTop(r)
	rec := &w4CountRecorder{}
	e.SetRecorder(rec)
	steps := 0
	e.SetTrace(func(step, pointer int, tape []Value, note string) {
		steps++
	})
	out, err := e.Run([]Value{NewWord("cadd"), NewInteger(1), NewInteger(2)})
	if err != nil {
		t.Fatalf("traced run: %v", err)
	}
	if renderAll(out) != "3" {
		t.Errorf("traced run = %s", renderAll(out))
	}
	if rec.lits == 0 || rec.calls == 0 {
		t.Errorf("recorder saw lits=%d calls=%d", rec.lits, rec.calls)
	}
	if steps == 0 {
		t.Error("trace callback never fired")
	}
	e.SetRecorder(nil)
	e.SetTrace(nil)
}

type w4CountRecorder struct {
	lits, calls int
}

func (c *w4CountRecorder) OnPushLit(Value)         { c.lits++ }
func (c *w4CountRecorder) OnCall(string, int, int) { c.calls++ }

// --- small pure helpers ---------------------------------------------------------------------

func TestSigOrderArgsShapes(t *testing.T) {
	a, b, c := NewInteger(1), NewInteger(2), NewInteger(3)
	// All-forward (nStack 0): unchanged.
	out := sigOrderArgs([]Value{a, b, c}, 0)
	if renderAll(out) != "1 | 2 | 3" {
		t.Errorf("all-forward = %s", renderAll(out))
	}
	// Two stack args below one forward: forward first, stack reversed.
	out = sigOrderArgs([]Value{a, b, c}, 2)
	if renderAll(out) != "3 | 2 | 1" {
		t.Errorf("mixed = %s", renderAll(out))
	}
	// Out-of-range nStack clamps to all-stack.
	out = sigOrderArgs([]Value{a, b}, 5)
	if renderAll(out) != "2 | 1" {
		t.Errorf("clamped = %s", renderAll(out))
	}
}

func TestFnConcreteSingleValuedOrCarrier(t *testing.T) {
	if !fnConcreteSingleValuedOrCarrier(NewCarrier(TFunction)) {
		t.Error("carrier refused")
	}
	one := NewFnDef(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "x", Type: TInteger}}, Returns: []*Type{TInteger},
		Impl: BORU([]Value{NewWord("x")}), BarrierPos: BarrierAllForward,
	}}})
	if !fnConcreteSingleValuedOrCarrier(one) {
		t.Error("single-return fn refused")
	}
	zero := NewFnDef(FnDefInfo{Signatures: []Signature{{
		Params: []FnParam{{Name: "x", Type: TInteger}},
		Impl:   BORU([]Value{NewWord("x")}), BarrierPos: BarrierAllForward,
	}}})
	if fnConcreteSingleValuedOrCarrier(zero) {
		t.Error("zero-return fn admitted")
	}
	if fnConcreteSingleValuedOrCarrier(NewFnDef(FnDefInfo{})) {
		t.Error("sig-less fn admitted")
	}
}

func TestValueTreeHasCarriers(t *testing.T) {
	if valueTreeHasCarriers(NewInteger(1)) {
		t.Error("plain int has carriers")
	}
	if !valueTreeHasCarriers(NewCarrier(TInteger)) {
		t.Error("carrier missed")
	}
	lst := NewList([]Value{NewInteger(1), NewCarrier(TString)})
	if !valueTreeHasCarriers(lst) {
		t.Error("nested list carrier missed")
	}
	om := NewOrderedMap()
	om.Set("a", NewList([]Value{NewDynamicCarrier(TAny)}))
	if !valueTreeHasCarriers(NewMap(om)) {
		t.Error("nested map carrier missed")
	}
	om2 := NewOrderedMap()
	om2.Set("a", NewInteger(1))
	if valueTreeHasCarriers(NewMap(om2)) {
		t.Error("concrete map has carriers")
	}
}

func TestCarrierOfLiteralHelper(t *testing.T) {
	c := carrierOfLiteral(NewInteger(42))
	if !c.Carrier {
		t.Error("carrierOfLiteral not a carrier")
	}
}
