package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildStringModule creates the "aql:string-util" native module (StringUtil
// namespace). It holds the string-manipulation words moved out of core:
// upper, lower, concat, split, trim, contains, indexof, replace, changecase,
// normalize, repeat, pad, match, escape.
//
//	import "aql:string-util"
//	["a" "b" "c"] StringUtil.concat       # → 'abc'
//	"a,b,c" "," StringUtil.split           # → ['a' 'b' 'c']
//	"  hi  " StringUtil.trim               # → 'hi'
//
// `indexof` keeps its List overload here too, so StringUtil.indexof covers
// both the string and list-membership forms.
func BuildStringModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	for _, n := range native.StringModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.StringModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		Src:     subReg,
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"StringUtil": exports},
	}
	return desc, nil
}
