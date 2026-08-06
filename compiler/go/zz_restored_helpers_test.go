package compiler

// Helpers restored from the pre-carve tree.

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// Helpers restored from the pre-carve tree.

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

func z9Engine(t *testing.T, r *core.Registry, tape []core.Value) *core.Engine {
	t.Helper()
	e := core.NewTop(r)
	e.Tape = core.NewTape(tape, 8)
	e.Pointer = 0
	return e
}
