package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildDBModule creates the "aql:db" native module. It registers the
// external-database words into an isolated sub-registry and returns a
// ModuleDesc with a "DB" export of FnDef wrappers.
//
// After import, the words are accessed via dot notation:
//
//	"aql:db" import
//	def pg (DB.open "postgres://user@host:5432/app")
//	pg DB.sql "select id, name from users where age > 18"   # raw SQL → table
//	pg DB.tables                                            # list tables
//	pg DB.describe "users"                                  # column schema
//	pg DB.close
//
// The wrappers dispatch to the inner native's handler via the
// trivial-delegation short-circuit against the LIVE engine registry, so
// the words reach the host database/network policy exactly as a core
// capability would. External connectivity is unavailable in the wasm
// build (DB.open returns a not-supported error there).
func BuildDBModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}
	for _, n := range native.DBModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, n := range native.DBModuleNatives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"DB": exports},
	}
	return desc, nil
}
