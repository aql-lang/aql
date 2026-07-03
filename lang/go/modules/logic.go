package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildLogicModule creates the "aql:logic-util" native module (LogicUtil
// namespace). It holds the derived boolean connectives moved out of core:
// nand, nor, iff, xnor, implies. The everyday connectives (and, or, not, xor,
// any, all) stay built-in.
//
//	import "aql:logic-util"
//	true false LogicUtil.nand        # → true
//	true false LogicUtil.implies     # → false
func BuildLogicModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	for _, n := range native.LogicModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.LogicModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		Src:     subReg,
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"LogicUtil": exports},
	}
	return desc, nil
}
