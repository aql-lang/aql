package eng

import (
	"errors"
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

// TestGuardedNativeCallContract pins the VM-level guarded CALL_NATIVE — the sound
// compiled form of a single-overload native dispatch the checker could not
// statically commit (the concrete-mismatch / Any-carrier recovery, e.g. the
// boru:test framework's test-invoke). A SigRef with Guard=true re-checks the
// concrete args against the committed sig: a matching arg dispatches the handler
// (== the interpreter's sole-overload dispatch), a mismatching one raises the
// byte-identical signature_error (== the interpreter finding no overload). This is
// the soundness boundary the poly re-match crossed (it optimistically dispatched a
// sibling the interpreter rejects); the guard never re-matches, so for a
// single-overload word it mirrors the interpreter exactly.
func TestGuardedNativeCallContract(t *testing.T) {
	newReg := func() *core.Registry {
		r, err := core.NewRegistry()
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		// A single-overload native: dbl [[Integer][Integer]] → n*2.
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "dbl",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(a []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					n, _ := core.AsInteger(a[0])
					return []core.Value{core.NewInteger(n * 2)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			}},
		})
		if err := r.Err(); err != nil {
			t.Fatalf("registration: %v", err)
		}
		r.InitRootContext()
		return r
	}

	sig := func(r *core.Registry) *core.Signature {
		fn := r.Lookup("dbl")
		if fn == nil || len(fn.Signatures) == 0 {
			t.Fatal("dbl not registered")
		}
		return &fn.Signatures[0]
	}

	// MATCH: a guarded call over a concrete Integer dispatches the handler.
	t.Run("match dispatches", func(t *testing.T) {
		r := newReg()
		p := &compiler.Program{
			Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNative, Arg: 0}},
			Debug:  []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
			Consts: []core.Value{core.NewInteger(21)},
			Sigs:   []compiler.SigRef{{Word: "dbl", Sig: sig(r), Guard: true}},
		}
		out, err := RunProgram(p, r)
		if err != nil {
			t.Fatalf("guarded match raised: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("want 1 result, got %d", len(out))
		}
		if n, _ := core.AsInteger(out[0]); n != 42 {
			t.Errorf("guarded dbl 21 = %d, want 42", n)
		}
	})

	// MISMATCH: a guarded call over a concrete String raises signature_error —
	// the byte-identical error the interpreter raises (no overload matches).
	t.Run("mismatch raises signature_error", func(t *testing.T) {
		r := newReg()
		p := &compiler.Program{
			Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNative, Arg: 0}},
			Debug:  []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
			Consts: []core.Value{core.NewString("nope")},
			Sigs:   []compiler.SigRef{{Word: "dbl", Sig: sig(r), Guard: true}},
		}
		_, err := RunProgram(p, r)
		if err == nil {
			t.Fatal("guarded mismatch dispatched instead of raising")
		}
		var ae *core.BoruError
		if !errors.As(err, &ae) || ae.Code != "signature_error" {
			t.Fatalf("guarded mismatch = %v (%T), want code signature_error", err, err)
		}
	})

	// CONTROL: the SAME mismatching call WITHOUT the guard dispatches the handler
	// (the unguarded committed-sig call trusts the static match) — proving the
	// guard is what supplies the runtime soundness check.
	t.Run("unguarded mismatch does NOT raise (control)", func(t *testing.T) {
		r := newReg()
		p := &compiler.Program{
			Code:   []compiler.Instr{{Op: compiler.OpPushConst, Arg: 0}, {Op: compiler.OpCallNative, Arg: 0}},
			Debug:  []core.SrcPos{{Row: 1, Col: 1}, {Row: 1, Col: 1}},
			Consts: []core.Value{core.NewInteger(5)},
			Sigs:   []compiler.SigRef{{Word: "dbl", Sig: sig(r), Guard: false}},
		}
		if _, err := RunProgram(p, r); err != nil {
			t.Fatalf("unguarded match raised: %v", err)
		}
	})
}
