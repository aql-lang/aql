package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// BuildStructModule creates the "boru:struct-util" native module. It registers the
// voxgig-struct data-manipulation words (formerly core words) into an
// isolated sub-registry and returns a ModuleDesc with a "Struct" export
// containing FnDef wrappers for each word.
//
// After import, the words are accessed via dot notation:
//
//	import "boru:struct-util"
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
	// The struct words are deliberately absent from the global registry, so
	// the module's fresh sub-registry gains them here explicitly.
	return buildDelegatingModule(parent, "boru:struct-util", "StructUtil", native.StructModuleNatives)
}
