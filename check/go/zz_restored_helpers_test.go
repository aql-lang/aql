package check

// Helpers restored from the pre-carve tree.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Helpers restored from the pre-carve tree.

// covRegistry builds a fresh registry with covWords plus any extra setup.
func covRegistry(t *testing.T, extra func(*core.Registry)) *core.Registry {
	t.Helper()
	r, err := core.NewRegistry()
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

// engWithTape builds a top-level engine over a fresh cov registry with
// the given tape values and pointer set to the given index.
func engWithTape(t *testing.T, vals []core.Value, pointer int) *core.Engine {
	t.Helper()
	r := covRegistry(t, nil)
	e := core.NewTop(r)
	e.Tape = core.NewTape(vals, core.StackHeadroom)
	e.Pointer = pointer
	return e
}

// xmlReg builds a cov registry plus a carrier-bound name (dynv) and a
// flow-control-raising word (cbreak) for driving the dynamic / flow arms.
func xmlReg(t *testing.T) *core.Registry {
	t.Helper()
	r := covRegistry(t, func(r *core.Registry) {
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "cfailx",
			Signatures: []core.Signature{{
				Args: []*core.Type{core.TInteger},
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
					return nil, &core.BoruError{Code: "runtime_error", Detail: "xboom"}
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			}},
		})
		r.RegisterNativeFunc(core.NativeFunc{
			Name: "cbreak",
			Signatures: []core.Signature{{
				Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, reg *core.Registry) ([]core.Value, error) {
					reg.FlowCtrl = core.FlowBreak
					return []core.Value{core.NewInteger(0)}, nil
				}),
				Returns: []*core.Type{core.TInteger}, BarrierPos: -1,
			}},
		})
	})
	r.Defs.Push("dynv", core.NewCarrier(core.TAny))
	return r
}
