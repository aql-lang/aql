package compiler

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

// Test blocks re-homed by compiler-driven triage at the carve.

import (
	core "github.com/boru-lang/boru/core/go"
)

// Test blocks re-homed by compiler-driven triage at the carve.

// registerDynWords adds:
//   - cany:  0-arg, declared Any (a dynamic carrier statically), returns 7.
//   - cmk1:  0-arg factory returning a 1-arity closure (adds 100).
//   - cmk2:  0-arg factory returning a 2-arity closure (csub-like, order
//     sensitive: returns args[1]-args[0]).
//   - upoly: user fn with Integer and String overloads.
func registerDynWords(r *core.Registry) {
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cany",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{core.NewInteger(7)}, nil
			}),
			Returns: []*core.Type{core.TAny}, BarrierPos: -1,
		}},
	})
	mkClosure := func(params []core.FnParam, body []core.Value) core.Value {
		return core.NewFunction(core.FnDefInfo{
			Anonymous: true,
			Signatures: []core.Signature{{
				Params:     params,
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(body),
				BarrierPos: core.BarrierAllForward,
			}},
		})
	}
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cmk1",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{mkClosure(
					[]core.FnParam{{Name: "x", Type: core.TInteger}},
					parenBody(core.NewWord("cadd"), core.NewWord("x"), core.NewInteger(100)),
				)}, nil
			}),
			Returns: []*core.Type{core.TFunction}, BarrierPos: -1,
		}},
	})
	r.RegisterNativeFunc(core.NativeFunc{
		Name: "cmk2",
		Signatures: []core.Signature{{
			Impl: core.Go(func(_ []core.Value, _ map[string]core.Value, _ []core.Value, _ *core.Registry) ([]core.Value, error) {
				return []core.Value{mkClosure(
					[]core.FnParam{{Name: "a", Type: core.TInteger}, {Name: "b", Type: core.TInteger}},
					parenBody(core.NewWord("cmul"), core.NewWord("a"), core.NewInteger(10),
						core.NewEnd(), core.NewWord("cadd"), core.NewWord("b")),
				)}, nil
			}),
			Returns: []*core.Type{core.TFunction}, BarrierPos: -1,
		}},
	})
	core.InstallFnDef(r, "upoly", core.FnDefInfo{
		Signatures: []core.Signature{
			{
				Params:     []core.FnParam{{Name: "n", Type: core.TInteger}},
				Returns:    []*core.Type{core.TInteger},
				Impl:       core.Boru(parenBody(core.NewWord("cadd"), core.NewWord("n"), core.NewInteger(1))),
				BarrierPos: core.BarrierAllForward,
			},
			{
				Params:     []core.FnParam{{Name: "s", Type: core.TString}},
				Returns:    []*core.Type{core.TString},
				Impl:       core.Boru(parenBody(core.NewWord("ccat"), core.NewWord("s"), core.NewString("!"))),
				BarrierPos: core.BarrierAllForward,
			},
		},
	})
}

// z9Member builds a named trivial-delegation fn VALUE over inner natives
// registered in r (the shaped-instance-method class).
func z9Member(name string, r *core.Registry, sigs ...core.Signature) core.Value {
	return core.NewFunction(core.FnDefInfo{Name: name, Registry: r, Signatures: sigs})
}
