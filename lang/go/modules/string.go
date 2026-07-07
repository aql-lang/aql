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
	return buildDelegatingModule(parent, "StringUtil", native.StringModuleNatives)
}
