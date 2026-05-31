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

// TestModuleWrapperRebindPreservesArgHandling locks in the rebinding
// invariant (InstallDef's module-wrapper branch): when a trivial-
// delegation wrapper is rebound to a bare name — via `def w pkg.word`
// or `unpack [word] pkg` — bare-word dispatch MUST behave exactly like
// dot-access (`pkg.word`), preserving the inner native's QuoteArgs and
// NoEvalArgs. Before the fix, InstallFnDef built a body-splice handler
// that re-dispatched the inner word in the WRONG registry and dropped
// QuoteArgs/NoEvalArgs entirely (FnSig has no QuoteArgs field), so a
// rebound `from`-style word couldn't quote its bare name argument.
//
// The probe word `qop` takes a /q-quoted Atom name and a code body it
// does NOT evaluate: it returns the captured name as a string and the
// raw (un-run) body length, so a regression in either flag is visible.
func TestModuleWrapperRebindPreservesArgHandling(t *testing.T) {
	r, _ := native.DefaultRegistry()
	r.SetParseFunc(parser.Parse)

	subReg, _ := native.DefaultRegistry()
	subReg.RegisterNativeFunc(native.NativeFunc{
		Name: "qop",
		Signatures: []native.NativeSig{{
			Args:       []*native.Type{native.TAtom, native.TList},
			QuoteArgs:  map[int]bool{0: true},
			NoEvalArgs: map[int]bool{1: true},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				name, _ := args[0].AsConcreteAtom()
				body, _ := native.AsList(args[1])
				// Returning the un-run body length proves NoEvalArgs held
				// (a sub-Run of [zzz gt 1] would error on undefined zzz).
				return []native.Value{native.NewString(name), native.NewInteger(int64(body.Len()))}, nil
			},
			Returns: []*native.Type{native.TString, native.TInteger}, BarrierPos: -1,
		}},
	})
	wrapper := native.NewFnDef(native.FnDefInfo{
		Name: "qop",
		Sigs: []native.FnSig{{
			Params:     []native.FnParam{{Type: native.TAtom}, {Type: native.TList}},
			Returns:    []*native.Type{native.TString, native.TInteger},
			Body:       []native.Value{native.NewWord("qop")},
			NoEvalArgs: map[int]bool{1: true},
			BarrierPos: -1,
		}},
		Registry: subReg,
	})
	exports := native.NewOrderedMap()
	exports.Set("qop", wrapper)
	r.Defs.Push("probe", native.NewMap(exports))

	// Rebind to a bare name via def, then invoke with a bare name arg
	// (needs QuoteArgs) and an unevaluated code body (needs NoEvalArgs).
	// zzz is undefined: if NoEvalArgs were dropped the body would be
	// sub-Run and error; if QuoteArgs were dropped, `myname` would error
	// as an undefined word.
	tokens, err := parser.Parse(`def qq probe.qop  qq myname [zzz gt 1]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := native.NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("rebound wrapper dispatch failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	if got, _ := result[0].AsConcreteString(); got != "myname" {
		t.Errorf("QuoteArgs lost: expected captured name %q, got %q", "myname", got)
	}
	if got, _ := result[1].AsConcreteInteger(); got != 3 {
		t.Errorf("NoEvalArgs lost: expected un-run body length 3, got %d", got)
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
