package eng

import (
	"strings"
	"testing"
)

// A minimal `cif` conditional word registered from inside the kernel,
// mirroring lang's if3 (native_control.go): the runtime handler splices
// the chosen body, the check-mode ReturnsFn runs both arms through the
// carrier machinery and records a BranchRecord so the emit → lower → VM
// pipeline exercises its branch/join paths.

func cifSplice(v Value) []Value {
	if v.Parent.Equal(TList) && v.Data != nil && !IsTypedList(v) && !IsTableType(v) {
		elems, _ := AsList(v)
		out := make([]Value, 0, elems.Len()+2)
		out = append(out, NewOpenParen())
		out = append(out, elems.Slice()...)
		out = append(out, NewCloseParen())
		return out
	}
	return []Value{v}
}

func cifReturns(args []Value, r *Registry) []Value {
	es := r.Check
	if lit, ok := LiteralCondValue(args[0]); ok {
		// Statically-known condition: capture only the taken arm.
		var stk []Value
		var defs map[string]Value
		if lit {
			restore := ApplyGuardNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = RunCarrierBodyWithDefs(r, args[1])
			restore()
			InstallJoinedDefs(r, defs, nil)
		} else {
			restore := ApplyComplementNarrowing(r, args[0])
			es.Recorder().ArmBranchCapture()
			stk, defs = RunCarrierBodyWithDefs(r, args[2])
			restore()
			InstallJoinedDefs(r, nil, defs)
		}
		frag := es.Recorder().TakeFragment()
		if len(stk) == 0 {
			es.Recorder().MarkUncompilable("cif: branch produces no value")
			return nil
		}
		out := stk[len(stk)-1]
		taken := lit
		es.Recorder().RecordBranch(BranchRecord{
			ConstCond: &taken, HasElse: true,
			Then: frag, ThenStk: stk, Out: out, Pos: args[0].Pos(),
		})
		return []Value{out}
	}

	armOf := func(arg Value, then bool) (*EmitFragment, []Value, map[string]Value, *Value) {
		if IsConcrete(arg) && arg.Parent.ConformsTo(TList) {
			var restore func()
			if then {
				restore = ApplyGuardNarrowing(r, args[0])
			} else {
				restore = ApplyComplementNarrowing(r, args[0])
			}
			es.Recorder().ArmBranchCapture()
			stk, defs := RunCarrierBodyWithDefs(r, arg)
			frag := es.Recorder().TakeFragment()
			restore()
			return frag, stk, defs, nil
		}
		v := arg
		return nil, []Value{v}, nil, &v
	}
	thenFrag, thenStk, thenDefs, thenValue := armOf(args[1], true)
	elseFrag, elseStk, elseDefs, elseValue := armOf(args[2], false)
	InstallJoinedDefs(r, thenDefs, elseDefs)
	joined := JoinCarrierStacks(thenStk, elseStk)
	if len(joined) == 0 {
		out := NewCarrier(TNone)
		es.Recorder().RecordBranch(BranchRecord{
			Cond: args[0], HasElse: true,
			Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
			ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
		})
		if !es.Recorder().Active() {
			return nil
		}
		return []Value{out}
	}
	if !es.Recorder().Active() {
		if v, ok := FoldVariadicArms(thenStk, elseStk); ok {
			return []Value{v}
		}
	}
	out := joined[len(joined)-1]
	es.Recorder().RecordBranch(BranchRecord{
		Cond: args[0], HasElse: true,
		Then: thenFrag, Els: elseFrag, ThenStk: thenStk, ElsStk: elseStk,
		ThenValue: thenValue, ElsValue: elseValue, Out: out, Pos: args[0].Pos(),
	})
	return []Value{out}
}

// registerBranchWords adds `cif` (3-arg conditional) and `cgt`
// (integer greater-than) to a covRegistry.
func registerBranchWords(r *Registry) {
	r.RegisterNativeFunc(NativeFunc{
		Name: "cif",
		Signatures: []Signature{{
			Args:       []*Type{TAny, TAny, TAny},
			NoEvalArgs: map[int]bool{0: true, 1: true, 2: true},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				condVal := args[0]
				// A single-element list condition ([true] — the form
				// LiteralCondValue folds statically) unwraps at runtime.
				if lst, err := AsList(condVal); err == nil && lst.Len() == 1 {
					condVal = lst.Get(0)
				}
				cond, _ := AsBoolean(condVal)
				if cond {
					return cifSplice(args[1]), nil
				}
				return cifSplice(args[2]), nil
			}),
			ReturnsFn: cifReturns, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(NativeFunc{
		Name: "cgt",
		Signatures: []Signature{{
			Args: []*Type{TInteger, TInteger},
			Impl: Go(func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
				// Convention: args[1] OP args[0] for natural forward reading.
				a, _ := AsInteger(args[0])
				b, _ := AsInteger(args[1])
				return []Value{NewBoolean(a > b)}, nil
			}),
			Returns: []*Type{TBoolean}, BarrierPos: -1,
		}},
	})
}

func codeBody(tokens ...Value) Value {
	return NewList(tokens)
}

func TestCompiledBranchTakesThen(t *testing.T) {
	// cif (cgt 5 3) [cadd 1 2] [cadd 30 40] → 3
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			codeBody(NewWord("cadd"), NewInteger(30), NewInteger(40)),
		}
	})
	if got != "3" {
		t.Errorf("got %q, want 3", got)
	}
}

func TestCompiledBranchTakesElse(t *testing.T) {
	// cif (cgt 3 5) [cadd 1 2] [cadd 30 40] → 70
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(3), NewInteger(5), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			codeBody(NewWord("cadd"), NewInteger(30), NewInteger(40)),
		}
	})
	if got != "70" {
		t.Errorf("got %q, want 70", got)
	}
}

func TestCompiledBranchValueArms(t *testing.T) {
	// Value (non-body) arms: cif (cgt 1 2) 10 20 → 20.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(1), NewInteger(2), NewCloseParen(),
			NewInteger(10),
			NewInteger(20),
		}
	})
	if got != "20" {
		t.Errorf("got %q, want 20", got)
	}
}

func TestCompiledBranchMixedArms(t *testing.T) {
	// Then is a body, else a plain value.
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(9), NewInteger(2), NewCloseParen(),
			codeBody(NewWord("cmul"), NewInteger(6), NewInteger(7)),
			NewInteger(0),
		}
	})
	if got != "42" {
		t.Errorf("got %q, want 42", got)
	}
}

func TestCompiledBranchConstCondFolds(t *testing.T) {
	// A literal list-form condition ([true]) folds statically: only the
	// taken arm is captured (BranchRecord.ConstCond).
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			codeBody(NewBoolean(true)),
			codeBody(NewWord("cadd"), NewInteger(20), NewInteger(22)),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(1)),
		}
	})
	if got != "42" {
		t.Errorf("got %q, want 42", got)
	}
	got = runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			codeBody(NewBoolean(false)),
			codeBody(NewWord("cadd"), NewInteger(20), NewInteger(22)),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(1)),
		}
	})
	if got != "2" {
		t.Errorf("got %q, want 2", got)
	}
}

func TestCompiledNestedBranches(t *testing.T) {
	inner := codeBody(
		NewWord("cif"),
		NewOpenParen(), NewWord("cgt"), NewInteger(1), NewInteger(2), NewCloseParen(),
		codeBody(NewWord("cadd"), NewInteger(1), NewInteger(1)),
		codeBody(NewWord("cadd"), NewInteger(2), NewInteger(2)),
	)
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			inner,
			codeBody(NewWord("cadd"), NewInteger(9), NewInteger(9)),
		}
	})
	if got != "4" {
		t.Errorf("got %q, want 4 (inner else)", got)
	}
}

func TestCompiledBranchFeedsCall(t *testing.T) {
	// The branch result becomes an argument of a following call:
	// cadd (cif (cgt 5 3) [cadd 1 2] [cadd 3 4]) 10 → 13
	got := runDifferential(t, registerBranchWords, func() []Value {
		return []Value{
			NewWord("cadd"),
			NewOpenParen(),
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			codeBody(NewWord("cadd"), NewInteger(1), NewInteger(2)),
			codeBody(NewWord("cadd"), NewInteger(3), NewInteger(4)),
			NewCloseParen(),
			NewInteger(10),
		}
	})
	if got != "13" {
		t.Errorf("got %q, want 13", got)
	}
}

func TestCompiledBranchZeroValueStatement(t *testing.T) {
	// Both arms run a 0-return word: the if is a 0-value statement
	// (zeroOut phantom); a following literal is the program's only
	// result in both engines.
	setup := func(r *Registry) {
		registerBranchWords(r)
		r.RegisterNativeFunc(NativeFunc{
			Name: "cnoop",
			Signatures: []Signature{{
				Args: []*Type{TInteger},
				Impl: Go(func(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
					return nil, nil
				}),
				Returns: []*Type{}, BarrierPos: -1,
			}},
		})
	}
	got := runDifferential(t, setup, func() []Value {
		return []Value{
			NewWord("cif"),
			NewOpenParen(), NewWord("cgt"), NewInteger(5), NewInteger(3), NewCloseParen(),
			codeBody(NewWord("cnoop"), NewInteger(1)),
			codeBody(NewWord("cnoop"), NewInteger(2)),
			NewEnd(),
			NewInteger(77),
		}
	})
	if got != "77" {
		t.Errorf("got %q, want 77", got)
	}
}

func TestCarrierJoinHelpers(t *testing.T) {
	// JoinCarrierStacks joins same-length stacks element-wise.
	a := []Value{NewCarrier(TInteger)}
	b := []Value{NewCarrier(TString)}
	joined := JoinCarrierStacks(a, b)
	if len(joined) != 1 {
		t.Fatalf("joined len = %d", len(joined))
	}
	// The join of Integer and String carriers is a wider carrier —
	// it must still admit both leaf kinds via unification.
	j := joined[0]
	if IsConcrete(j) {
		t.Errorf("join of two carriers is concrete: %v", j)
	}
	// Mismatched lengths: the shorter side wins (variadic absorption
	// is the caller's job); just assert no panic and sane length.
	joined = JoinCarrierStacks([]Value{NewCarrier(TInteger), NewCarrier(TInteger)}, nil)
	if len(joined) > 2 {
		t.Errorf("mismatched join len = %d", len(joined))
	}

	// JoinCarriers on values.
	jc := JoinCarriers(NewCarrier(TInteger), NewCarrier(TInteger))
	if !strings.Contains(jc.String(), "Integer") {
		t.Errorf("JoinCarriers(Integer,Integer) = %v", jc)
	}

	// BoolWord renders the source literal.
	if BoolWord(true) != "true" || BoolWord(false) != "false" {
		t.Error("BoolWord wrong")
	}

	// LiteralCondValue folds a single-element condition list holding a
	// concrete boolean (or a true/false word); anything else refuses.
	if v, ok := LiteralCondValue(NewList([]Value{NewBoolean(true)})); !ok || !v {
		t.Error("LiteralCondValue([true]) failed")
	}
	if v, ok := LiteralCondValue(NewList([]Value{NewWord("false")})); !ok || v {
		t.Error("LiteralCondValue([word false]) failed")
	}
	if _, ok := LiteralCondValue(NewList([]Value{NewCarrier(TBoolean)})); ok {
		t.Error("Boolean carrier misread as a literal condition")
	}
	if _, ok := LiteralCondValue(NewBoolean(true)); ok {
		t.Error("bare boolean (non-list) misread as a condition list")
	}
	if _, ok := LiteralCondValue(NewList([]Value{NewBoolean(true), NewBoolean(false)})); ok {
		t.Error("two-element condition list misread as literal")
	}
}
