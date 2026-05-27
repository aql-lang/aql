package modules

import (
	"fmt"

	"github.com/aql-lang/aql/lang/go/native"
	"github.com/aql-lang/aql/lang/go/policy"
)

// resolveFn is the module resolver used by sub-engines. It's
// initialised in init() to break the package-level cycle between
// the modules map and BuildVMModule → Resolve.
var resolveFn func(name string, parent *native.Registry) (native.ModuleDesc, error)

func init() {
	resolveFn = Resolve
}

// BuildVMModule creates the "aql:vm" native module. The module
// exposes one canonical operation — run code in a sub-engine with
// an explicit policy — and a small surface of conveniences:
//
//	vm.run          code             # default sandbox profile
//	vm.run-with     code policy-map  # explicit policy as data
//	vm.run-sandbox  code             # built-in 'sandbox' profile
//	vm.run-compute  code             # built-in 'compute' profile
//
// Each handler:
//
//  1. Resolves the policy (built-in name or constructed from the
//     supplied map).
//  2. Validates attenuation: child must be a subset of the parent
//     engine's effective policy. The vm word cannot grant itself
//     anything the parent doesn't already have.
//  3. Constructs a fresh native.Registry with the resolved policy.
//  4. Parses the source via parent.ParseFunc (single parser).
//  5. Runs it through a fresh engine.
//  6. Returns the last value on the residual stack to the parent.
func BuildVMModule(parent *native.Registry) (native.ModuleDesc, error) {
	if parent.ParseFunc == nil {
		return native.ModuleDesc{}, fmt.Errorf("vm: parser not configured")
	}

	// Construct a sub-registry for the module's words. Module exports
	// are dispatched through this sub-registry; the parent's policy
	// is consulted for module.call gating elsewhere.
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	natives := vmNatives(parent)
	for _, n := range natives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	exports.Set("run", makeRunFnDef("vm-run", subReg))
	exports.Set("run-with", makeRunWithFnDef("vm-run-with", subReg))
	exports.Set("run-sandbox", makeRunFnDef("vm-run-sandbox", subReg))
	exports.Set("run-compute", makeRunFnDef("vm-run-compute", subReg))

	modID := parent.Modules.NextID()
	return native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"vm": exports},
	}, nil
}

// vmNatives builds the NativeFunc slice for the vm module. Defined
// as a closure-returning function because the handlers need to
// reference parent for policy / parser access.
func vmNatives(parent *native.Registry) []native.NativeFunc {
	return []native.NativeFunc{
		{
			Name: "vm-run",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := policy.Load("sandbox")
					if err != nil {
						return nil, fmt.Errorf("vm.run: load sandbox: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			Name: "vm-run-sandbox",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := policy.Load("sandbox")
					if err != nil {
						return nil, fmt.Errorf("vm.run-sandbox: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			Name: "vm-run-compute",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := policy.Load("compute")
					if err != nil {
						return nil, fmt.Errorf("vm.run-compute: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
		{
			Name: "vm-run-with",
			Signatures: []native.NativeSig{{
				Args: []*native.Type{native.TString, native.TMap},
				Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
					code, err := args[0].AsConcreteString()
					if err != nil {
						return nil, err
					}
					pol, err := policyFromMapValue(args[1])
					if err != nil {
						return nil, fmt.Errorf("vm.run-with: %w", err)
					}
					return runInSubEngine(parent, code, pol)
				},
				Returns:    []*native.Type{native.TAny},
				BarrierPos: -1,
			}},
		},
	}
}

// runInSubEngine builds a fresh registry under pol, parses src via
// the parent's parse func, runs it, and returns the last residual
// stack value to the parent. The parent's policy is consulted for
// attenuation: pol must be a subset.
func runInSubEngine(parent *native.Registry, src string, pol policy.Policy) ([]native.Value, error) {
	if parentPol := native.HostPolicy(parent); parentPol != nil {
		if err := policy.RequireSubset(pol, parentPol); err != nil {
			return nil, err
		}
	}
	subReg, err := native.DefaultRegistryWithPolicy(pol)
	if err != nil {
		return nil, fmt.Errorf("vm: init sub-engine: %w", err)
	}
	subReg.SetParseFunc(parent.ParseFunc)
	// Indirect through resolveFn (initialised in init()) so the
	// package-level modules map can reference BuildVMModule without
	// creating an initialisation cycle through Resolve.
	if resolveFn != nil {
		subReg.Modules.Resolver = resolveFn
	}
	native.EnableDynamicHelp(subReg)
	subReg.MarkReady()

	tokens, err := parent.ParseFunc(src)
	if err != nil {
		return nil, fmt.Errorf("vm: parse: %w", err)
	}
	subReg.Source = src
	eng := native.NewTop(subReg)
	eng.SetSource(src)
	stack, err := eng.Run(tokens)
	if err != nil {
		return nil, err
	}
	if len(stack) == 0 {
		return []native.Value{native.NewNone()}, nil
	}
	// Return the last value as the result on the parent stack.
	return []native.Value{stack[len(stack)-1]}, nil
}

// policyFromMapValue converts an AQL Map value (as received in a
// vm.run-with arg) into a policy.Policy.
func policyFromMapValue(v native.Value) (policy.Policy, error) {
	if !native.IsConcrete(v) {
		return nil, fmt.Errorf("policy map cannot be a type literal or carrier")
	}
	m, err := native.RequireConcreteMap(v, "vm.run-with policy")
	if err != nil {
		return nil, err
	}
	// Convert the AQL map to a generic map[string]any for the
	// policy loader. We use AQL's value-to-any conversion path,
	// which already handles maps, lists, strings, numbers, bools.
	raw := native.ValueToAny(v)
	asMap, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("policy map: expected map, got %T (size=%d)", raw, m.Len())
	}
	return policy.FromMap(asMap)
}

// makeRunFnDef creates a FnDef wrapper for a single-arg vm-run word.
func makeRunFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params:     []native.FnParam{{Type: native.TString}},
			Returns:    []*native.Type{native.TAny},
			Body:       []native.Value{native.NewWord(wordName)},
			BarrierPos: -1,
		}},
		Registry: subReg,
	})
}

// makeRunWithFnDef creates a FnDef wrapper for vm-run-with (code +
// policy map). The FnDef's params take the call's stack args by
// position and pass them through to the native vm-run-with.
//
// The body uses TAny params (rather than the underlying TString +
// TMap signature) because the dotted-form dispatch via `vm.run-with`
// matches against the FnDef's own signature, and we want the dotted
// form to dispatch against any-typed args (the native validates
// types itself).
func makeRunWithFnDef(wordName string, subReg *native.Registry) native.Value {
	return native.NewFnDef(native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{{
			Params: []native.FnParam{
				{Type: native.TAny},
				{Type: native.TAny},
			},
			Returns:    []*native.Type{native.TAny},
			Body:       []native.Value{native.NewWord(wordName)},
			BarrierPos: 0,
		}},
		Registry: subReg,
	})
}
