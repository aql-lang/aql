package test

import (
	lang "github.com/boru-lang/boru/lang/go"
	"github.com/boru-lang/boru/lang/go/native"
)

// seedBORU installs the words that moved out of core into loadable modules
// under their bare names on a lang.BORU test instance, so behaviour/typecheck
// tests can keep using `upper`, `now`, `band`, … without an explicit import.
// Production requires `import "boru:<mod>"` (proved by the module-*.tsv specs).
func seedBORU(a *lang.BORU) {
	moved := [][]native.NativeFunc{
		native.IOModuleNativeFuncs(native.IOModuleTypes{StreamKind: native.NewStreamKind(), FileType: native.NewFileType(), Watcher: native.NewWatcherType(), File: native.NewFileHandleType(), Lock: native.NewLockType(), Mmap: native.NewMmapType()}),
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
