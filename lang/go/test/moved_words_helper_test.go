package test

import (
	lang "github.com/aql-lang/aql/lang/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// seedAQL installs the words that moved out of core into loadable modules
// under their bare names on a lang.AQL test instance, so behaviour/typecheck
// tests can keep using `upper`, `now`, `band`, … without an explicit import.
// Production requires `import "aql:<mod>"` (proved by the module-*.tsv specs).
func seedAQL(a *lang.AQL) {
	moved := [][]native.NativeFunc{
		native.IOModuleNativeFuncs(native.NewStreamKind()),
		native.StructModuleNatives,
		native.NetModuleNatives(native.MintFetchTypes(a.NativeRegistry())),
		native.BitwiseModuleNatives,
		native.TPartialModuleNatives,
		native.TimeAsyncModuleNatives(native.MintTemporalModuleTypes(a.NativeRegistry())),
		native.LogicModuleNatives,
		native.StringModuleNatives,
	}
	for _, slice := range moved {
		for _, n := range slice {
			a.RegisterNativeFunc(n)
		}
	}
}
