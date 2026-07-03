package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildStructModule creates the "aql:struct-util" native module. It registers the
// voxgig-struct data-manipulation words (formerly core words) into an
// isolated sub-registry and returns a ModuleDesc with a "Struct" export
// containing FnDef wrappers for each word.
//
// After import, the words are accessed via dot notation:
//
//	import "aql:struct-util"
//	{a:1 b:2} {b:3 c:4} StructUtil.merge      # deep merge
//	{a:1} StructUtil.clone                    # deep copy
//	"a.b" {a:{b:42}} StructUtil.getpath        # path read
//	{a:1} StructUtil.items                    # [['a' 1]]
//
// These words were moved OUT of the core registry (see
// native/struct_module.go); they are no longer available unqualified. The
// handlers still live in the native package, but only this module registers
// them, so all access is through the Struct namespace.
func BuildStructModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Isolated sub-registry holding the module's Go words. The struct words
	// are deliberately absent from the global registry, so a fresh
	// DefaultRegistry does not contain them; we add them here explicitly.
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	for _, n := range native.StructModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.StructModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		Src:     subReg,
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"StructUtil": exports},
	}
	return desc, nil
}

// makeModuleFnDef builds a FnDef value wrapping a moved-out native word. Each
// signature delegates via a trivial body [Word(name)], which execFnDefLiteral
// short-circuits to a direct dispatch of the inner native in the
// sub-registry. The wrapper FnSigs mirror the inner native's Signatures
// exactly — same arg types, returns, NoEvalArgs, and BarrierPos — so dispatch
// behaviour is identical to the former core word (dot-access dispatch keys
// off the inner native's signatures; see the "Module FnDef Wrappers" note in
// lang/go/CLAUDE.md). Shared by the aql:struct and aql:io modules.
func makeModuleFnDef(n native.NativeFunc, subReg *native.Registry) native.Value {
	fnSigs := make([]native.FnSig, len(n.Signatures))
	for i, s := range n.Signatures {
		params := make([]native.FnParam, len(s.Args))
		for j, t := range s.Args {
			params[j] = native.FnParam{Type: t}
		}
		fnSigs[i] = native.FnSig{
			Params:     params,
			Returns:    s.Returns,
			Impl:       native.AQL([]native.Value{native.NewWord(n.Name)}),
			NoEvalArgs: s.NoEvalArgs,
			BarrierPos: s.BarrierPos,
			// Carry the inner native's check-mode ReturnsFn onto the wrapper
			// so static analysis of a dotted module call uses it instead of
			// the fixed Returns shape — e.g. the body-running debug words
			// analyse their quoted body for type errors.
			ReturnsFn: s.ReturnsFn,
		}
	}
	return native.NewFnDef(native.FnDefInfo{
		Name:       n.Name,
		Signatures: fnSigs,
		Registry:   subReg,
	})
}
