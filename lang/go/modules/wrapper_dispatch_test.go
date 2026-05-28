package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// TestModuleWrapperInnerSigBarrierPos locks in the dispatch
// invariant for module FnDef wrappers: when the wrapper's body
// is a single word that calls a native registered in the wrapper's
// sub-registry (the standard `aql:math` / `aql:bin` / `aql:type`
// pattern), the inner native's signature MUST allow forward
// dispatch (BarrierPos != 0). Otherwise the wrapper FnDef's
// swap-form auto-invoke fails to match because matchSignature
// consults the inner native's sig (via reg.Lookup(fnDef.Name)).
//
// See lang/go/CLAUDE.md "Module FnDef wrappers" for the design
// note. Before the fix, a sub-registry native with BarrierPos=0
// (stack-only) silently broke swap-form callers of the wrapper
// — the FnDef would just sit on the stack with the args around
// it, never invoked.
func TestModuleWrapperInnerSigBarrierPos(t *testing.T) {
	cases := []struct {
		name       string
		innerBP    int
		wantInvoke bool
	}{
		{"inner BarrierPos=-1 (all-forward eligible) dispatches", -1, true},
		{"inner BarrierPos=0 (stack-only) breaks swap-form dispatch", 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := buildProbeRegistry(c.innerBP)
			tokens, err := parser.Parse("5 probe.op 3")
			if err != nil {
				t.Fatal(err)
			}
			e := native.NewTop(r)
			result, err := e.Run(tokens)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			invoked := len(result) == 1 && result[0].Parent.Equal(native.TInteger)
			if invoked != c.wantInvoke {
				t.Errorf("with inner BarrierPos=%d, wantInvoke=%v, gotInvoke=%v (result=%v)",
					c.innerBP, c.wantInvoke, invoked, result)
			}
		})
	}
}

// buildProbeRegistry constructs a one-word module whose inner
// native uses the requested BarrierPos. Module name is "probe",
// word name is "op".
func buildProbeRegistry(innerBarrier int) *native.Registry {
	r, _ := native.DefaultRegistry()
	r.SetParseFunc(parser.Parse)

	subReg, _ := native.DefaultRegistry()
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "op",
		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TAny, native.TAny},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				a, _ := args[0].AsConcreteInteger()
				b, _ := args[1].AsConcreteInteger()
				return []native.Value{native.NewInteger(a + b)}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: innerBarrier,
		}},
	})

	wrapper := native.NewFnDef(native.FnDefInfo{
		Name: "op",
		Sigs: []native.FnSig{{
			Params:  []native.FnParam{{Type: native.TAny}, {Type: native.TAny}},
			Returns: []*native.Type{native.TInteger},
			Body:    []native.Value{native.NewWord("op")}, BarrierPos: -1,
		}},
		Registry: subReg,
	})
	exports := native.NewOrderedMap()
	exports.Set("op", wrapper)
	r.Defs.Push("probe", native.NewMap(exports))
	return r
}
