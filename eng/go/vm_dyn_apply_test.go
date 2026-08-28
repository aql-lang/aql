package eng

import (
	"strings"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// dynApplyFn builds a fn VALUE whose single sig carries a compiled ref to
// prog.Fns[unit] — the shape stampFnConst produces for a fn-value const.
func dynApplyFn(params []core.FnParam, prog *compiler.Program, unit int) core.Value {
	impl := core.Boru([]core.Value{core.NewInteger(0)})
	impl.Compiled = &compiler.CompiledFnRef{Prog: prog, Unit: unit}
	fd := core.FnDefInfo{Signatures: []core.Signature{{
		Params: params, Returns: []*core.Type{core.TAny}, BarrierPos: core.BarrierAllForward,
		Impl: impl,
	}}}
	return core.Value{Parent: core.TFunction, Data: fd}
}

// The declines: dynApplyEnter answers nil for anything it cannot enter as a
// frame of THIS program, and the island then owns the apply exactly as before.
// Each arm below is a different reason, and every one of them keeps the answer
// the interpreter's.
func TestDynApplyEnterDeclines(t *testing.T) {
	vc, _ := foreignVC(t, oneConstProg(1))
	prog := oneConstProg(7)

	t.Run("not a fn value", func(t *testing.T) {
		if ent := vc.dynApplyEnter(core.NewInteger(5), nil); ent != nil {
			t.Fatalf("a non-fn value is not a callee, got %v", ent)
		}
	})

	t.Run("quoted fn is data", func(t *testing.T) {
		fn := dynApplyFn(nil, prog, 0)
		fn.Quoted = true
		if ent := vc.dynApplyEnter(fn, nil); ent != nil {
			t.Fatal("a QUOTED fn is data — entering it turns `((mk) 2)` from " +
				"[fn (Integer) 2] into [3], a silent wrong answer")
		}
	})

	t.Run("unit belongs to another program", func(t *testing.T) {
		// A DETACHED ref: its unit index means nothing against vc.p, and
		// hosting it nested would reintroduce the per-body context bracket a
		// fn application must not have.
		fn := dynApplyFn(nil, oneConstProg(9), 0)
		if ent := vc.dynApplyEnter(fn, nil); ent != nil {
			t.Fatal("a ref from another program cannot be a frame of this one")
		}
	})

	t.Run("unit index outside the table", func(t *testing.T) {
		fn := dynApplyFn(nil, vc.p, 99)
		if ent := vc.dynApplyEnter(fn, nil); ent != nil {
			t.Fatal("an out-of-range unit is compile/run drift, not a callee")
		}
	})

	t.Run("unit shape disagrees with the match", func(t *testing.T) {
		// The sig matched one arg; the unit declares none. Entering on a
		// mismatch would bind the wrong locals silently.
		fn := dynApplyFn([]core.FnParam{{Name: "n", Type: core.TAny}}, vc.p, 0)
		if ent := vc.dynApplyEnter(fn, []core.Value{core.NewInteger(1)}); ent != nil {
			t.Fatalf("NParams %d vs 1 arg must decline, got %v", vc.p.Fns[0].NParams, ent)
		}
	})
}

// A LIST argument arrives QUOTED, so body references read it as data — the
// compiled mirror of the interpreter's binding rule, and the same delivery
// OpCallUserPoly performs. Without it a body that names its list param would
// re-step the list's contents.
func TestDynApplyEnterQuotesListParams(t *testing.T) {
	vc, r := foreignVC(t, &compiler.Program{
		Fns: []compiler.CompiledFn{{
			Name: "takesList", NParams: 1, NLocals: 1, NArgs: 1,
			Code:  []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug: make([]core.SrcPos, 2),
		}},
	})
	_ = r
	fn := dynApplyFn([]core.FnParam{{Name: "l", Type: core.TList}}, vc.p, 0)
	lst := core.NewList([]core.Value{core.NewInteger(1), core.NewInteger(2)})
	if lst.Quoted {
		t.Fatal("fixture list arrived already quoted — the assertion below would be vacuous")
	}
	ent := vc.dynApplyEnter(fn, []core.Value{lst})
	if ent == nil {
		t.Fatal("a matching list param must enter")
	}
	if !ent.locals[0].Quoted {
		t.Error("a LIST param must be delivered QUOTED, as OpCallUserPoly delivers it")
	}
}

// The param-contract re-check at the frame push. MatchFnSig selects on the
// SIGNATURE's types; the unit records its own declared Params, and a drift
// between the two must raise the interpreter's no-match rather than bind a
// value the body's declaration rejects.
func TestDynApplyFramePushChecksParamContract(t *testing.T) {
	r := stampReg(t)
	prog := &compiler.Program{
		Consts: []core.Value{core.NewString("nope")},
		Fns: []compiler.CompiledFn{{
			Name: "wantsInteger", NParams: 1, NLocals: 1, NArgs: 1,
			Params: []*core.Type{core.TInteger}, // the unit demands Integer …
			Code:   []compiler.Instr{{Op: compiler.OpPushLocal, Arg: 0}, {Op: compiler.OpRet, Arg: 0}},
			Debug:  make([]core.SrcPos, 2),
		}},
	}
	// … while the SIG matched Any, so a String reaches the frame push.
	fn := dynApplyFn([]core.FnParam{{Name: "n", Type: core.TAny}}, prog, 0)
	prog.Consts = append(prog.Consts, fn)
	prog.Code = []compiler.Instr{
		{Op: compiler.OpPushConst, Arg: 1}, // the fn
		{Op: compiler.OpPushConst, Arg: 0}, // "nope"
		{Op: compiler.OpCallDynamic, Arg: 1},
	}
	prog.Debug = make([]core.SrcPos, 3)

	_, err := RunProgram(prog, r)
	if err == nil {
		t.Fatal("a String at an Integer-declared unit param must raise, not bind")
	}
	if !strings.Contains(err.Error(), "wantsInteger") {
		t.Fatalf("err = %v, want the callee's no-match", err)
	}
}
