package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// BuildStringModule creates the "boru:string-util" native module (StringUtil
// namespace). It holds the string-manipulation words moved out of core:
// upper, lower, concat, split, trim, contains, indexof, replace, changecase,
// normalize, repeat, pad, match, escape.
//
//	import "boru:string-util"
//	["a" "b" "c"] StringUtil.concat       # → 'abc'
//	"a,b,c" "," StringUtil.split           # → ['a' 'b' 'c']
//	"  hi  " StringUtil.trim               # → 'hi'
//
// `indexof` keeps its List overload here too, so StringUtil.indexof covers
// both the string and list-membership forms.
func BuildStringModule(parent *native.Registry) (native.ModuleDesc, error) {
	return buildDelegatingModule(parent, "boru:string-util", "StringUtil", native.StringModuleNatives)
}
