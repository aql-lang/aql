package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// BuildLogicModule creates the "boru:logic-util" native module (LogicUtil
// namespace). It holds the derived boolean connectives moved out of core:
// nand, nor, iff, xnor, implies. The everyday connectives (and, or, not, xor,
// any, all) stay built-in.
//
//	import "boru:logic-util"
//	true false LogicUtil.nand        # → true
//	true false LogicUtil.implies     # → false
func BuildLogicModule(parent *native.Registry) (native.ModuleDesc, error) {
	return buildDelegatingModule(parent, "boru:logic-util", "LogicUtil", native.LogicModuleNatives)
}
