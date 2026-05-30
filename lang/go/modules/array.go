package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildArrayModule creates the "aql:array" native module. It registers the
// Go-implemented array words into an isolated sub-registry and returns a
// ModuleDesc with an "array" export containing FnDef wrappers for each word.
//
// After import, words are accessed via dot notation: array.shape,
// array.reshape, array.where, etc.
//
// The everyday array words remain built-in and do NOT require this module:
// the constructors iota/range, the basic slicing words take/shed/reverse,
// and the higher-order combinators each/fold/scan/outer/inner. This module
// holds the specialised APL-style data vocabulary — shape/structure,
// selection/ordering, membership/grouping, and neighborhood words.
//
// Per ADR-001 (ADR.md in the repo root) no export here shadows a core word. The two
// operations that overlap a core word are therefore NOT in this module:
// deep flatten is the core `flatten -1`, and list lookup is a [List, List]
// overload of the core `indexof`. Only `transpose` (which has no core
// counterpart) remains, under its plain name.
func BuildArrayModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Create an isolated sub-registry for the module's Go words.
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// Register the specialised array words into the sub-registry. They are
	// deliberately absent from the global registry (see native_array.go).
	for _, n := range native.ArrayModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()
	for _, w := range arrayExports {
		exports.Set(w.export, makeArrayFnDef(w.internal, w.sigs, subReg))
	}

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"array": exports},
	}
	return desc, nil
}

// arrSig describes one signature of an array word: argument types (in sig
// order, top-of-stack first) and the static return type(s).
type arrSig struct {
	params  []*native.Type
	returns []*native.Type
}

// arrWord maps a module export name to the internal native word it
// delegates to, with one or more signatures.
type arrWord struct {
	export   string
	internal string
	sigs     []arrSig
}

// arrayExports is the export table for aql:array. export is the clean
// namespaced name (array.<export>); internal is the underlying native
// word registered in the sub-registry — identical except for the three
// collision-avoiding "arr-" words, which reclaim their clean names here.
var arrayExports = []arrWord{
	// --- shape / structure ---
	{"shape", "shape", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TList}}}},
	{"rank", "rank", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TInteger}}}},
	{"reshape", "reshape", []arrSig{{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TList}}}},
	{"transpose", "transpose", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TList}}}},

	// --- selection / ordering ---
	{"where", "where", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TList}}}},
	{"grade", "grade", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TList}}}},
	{"at", "at", []arrSig{{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TList}}}},
	{"sortby", "sortby", []arrSig{{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TList}}}},
	{"replicate", "replicate", []arrSig{{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TList}}}},
	{"expand", "expand", []arrSig{{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TList}}}},

	// --- membership / grouping ---
	{"member", "member", []arrSig{{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TList}}}},
	{"unique", "unique", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TList}}}},
	{"group", "group", []arrSig{
		{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TMap}},
		{[]*native.Type{native.TList}, []*native.Type{native.TMap}},
	}},

	// --- neighborhoods ---
	{"window", "window", []arrSig{{[]*native.Type{native.TInteger, native.TList}, []*native.Type{native.TList}}}},
	{"pairs", "pairs", []arrSig{{[]*native.Type{native.TList}, []*native.Type{native.TList}}}},
}

// makeArrayFnDef builds a FnDef value that wraps an internal array word.
// Each signature delegates via a trivial body [Word(internalName)], which
// execFnDefLiteral short-circuits to a direct dispatch of the inner native
// in the sub-registry. BarrierPos: -1 keeps the swap form dispatchable
// (see the "Module FnDef Wrappers" note in lang/go/CLAUDE.md).
func makeArrayFnDef(internalName string, sigs []arrSig, subReg *native.Registry) native.Value {
	fnSigs := make([]native.FnSig, len(sigs))
	for i, s := range sigs {
		params := make([]native.FnParam, len(s.params))
		for j, t := range s.params {
			params[j] = native.FnParam{Type: t}
		}
		fnSigs[i] = native.FnSig{
			Params:     params,
			Returns:    s.returns,
			Body:       []native.Value{native.NewWord(internalName)},
			BarrierPos: -1,
		}
	}
	return native.NewFnDef(native.FnDefInfo{
		Name:     internalName,
		Sigs:     fnSigs,
		Registry: subReg,
	})
}
