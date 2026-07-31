package modules

import (
	"github.com/boru-lang/boru/lang/go/native"
)

// BuildArrayModule creates the "boru:array-util" native module. It registers the
// Go-implemented array words into an isolated sub-registry and returns a
// ModuleDesc with an "array" export containing FnDef wrappers for each word.
//
// After import, words are accessed via dot notation: ArrayUtil.shape,
// ArrayUtil.reshape, ArrayUtil.where, etc.
//
// The everyday array words remain built-in and do NOT require this module:
// the constructors iota/range, the basic slicing words take/shed/reverse,
// and the higher-order combinators each/fold/scan/outer/inner. This module
// holds the specialised APL-style data vocabulary — shape/structure,
// selection/ordering, membership/grouping, and neighborhood words.
//
// Per ADR-001 (ADR.md in the repo root) no export here shadows a core word.
// Deep flatten is not duplicated here — it is the core `flatten -1`. The
// list-membership lookup lives here as `ArrayUtil.indices` (for each
// needle, its index in the haystack, or -1 when absent); it is a distinct
// name, not a shadow of the string word `indexof` (which is string-only,
// in boru:string-util).
func BuildArrayModule(parent *native.Registry) (native.ModuleDesc, error) {
	// Create an isolated sub-registry with the specialised array words.
	// They are deliberately absent from the global registry (see
	// native_array.go).
	subReg, err := newModuleRegistry("boru:array-util", native.ArrayModuleNatives)
	if err != nil {
		return native.ModuleDesc{}, err
	}

	exports := native.NewOrderedMap()
	for _, w := range arrayExports {
		sigs := make([]wrapSig, len(w.sigs))
		for i, s := range w.sigs {
			sigs[i] = typeSig(s.params, s.returns...)
			sigs[i].noEval = w.noEval
		}
		exports.Set(w.export, makeWrapFnDef(w.internal, subReg, sigs...))
	}

	return moduleDesc(parent, "ArrayUtil", subReg, exports), nil
}

// arrSig describes one signature of an array word: argument types (in sig
// order, top-of-stack first) and the static return type(s).
type arrSig struct {
	params  []*native.Type
	returns []*native.Type
}

// arrWord maps a module export name to the internal native word it
// delegates to, with one or more signatures. noEval marks the sig
// positions that hold a quoted code body (mirrors the inner native's
// NoEvalArgs) so the wrapper does not auto-evaluate them during forward
// collection.
type arrWord struct {
	export   string
	internal string
	sigs     []arrSig
	noEval   map[int]bool
}

// arrayExports is the export table for boru:array. export is the clean
// namespaced name (array.<export>); internal is the underlying native
// word registered in the sub-registry — identical except for the three
// collision-avoiding "arr-" words, which reclaim their clean names here.
var arrayExports = []arrWord{
	// --- shape / structure ---
	{export: "shape", internal: "shape", sigs: sig1(native.TList, native.TList)},
	{export: "rank", internal: "rank", sigs: sig1(native.TList, native.TInteger)},
	{export: "reshape", internal: "reshape", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "transpose", internal: "transpose", sigs: sig1(native.TList, native.TList)},

	// --- selection / ordering ---
	{export: "where", internal: "where", sigs: sig1(native.TList, native.TList)},
	{export: "grade", internal: "grade", sigs: sig1(native.TList, native.TList)},
	{export: "at", internal: "at", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "sortby", internal: "sortby", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "replicate", internal: "replicate", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "expand", internal: "expand", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "compress", internal: "compress", sigs: sig2(native.TList, native.TList, native.TList)},

	// --- rank polymorphism (quoted code body at position 1) ---
	{export: "eachrank", internal: "eachrank",
		sigs:   []arrSig{{[]*native.Type{native.TInteger, native.TList, native.TList}, []*native.Type{native.TList}}},
		noEval: map[int]bool{1: true}},
	{export: "foldaxis", internal: "foldaxis",
		sigs:   []arrSig{{[]*native.Type{native.TInteger, native.TList, native.TList}, []*native.Type{native.TList}}},
		noEval: map[int]bool{1: true}},

	// --- editing (copy-returning single-element edits) ---
	{export: "insert-at", internal: "insert-at",
		sigs: []arrSig{{[]*native.Type{native.TInteger, native.TAny, native.TList}, []*native.Type{native.TList}}}},
	{export: "remove-at", internal: "remove-at", sigs: sig2(native.TInteger, native.TList, native.TList)},

	// --- membership / grouping ---
	{export: "member", internal: "member", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "unique", internal: "unique", sigs: sig1(native.TList, native.TList)},
	// indices: forward form is `indices <needles> <haystack>` — the
	// haystack (the larger reference collection) is the final argument.
	{export: "indices", internal: "indices", sigs: sig2(native.TList, native.TList, native.TList)},
	{export: "group", internal: "group", sigs: []arrSig{
		{[]*native.Type{native.TList, native.TList}, []*native.Type{native.TMap}},
		{[]*native.Type{native.TList}, []*native.Type{native.TMap}},
	}},

	// --- neighborhoods ---
	{export: "window", internal: "window", sigs: sig2(native.TInteger, native.TList, native.TList)},
	{export: "pairs", internal: "pairs", sigs: sig1(native.TList, native.TList)},
}

// sig1 builds a single one-argument signature: [a] -> [ret].
func sig1(a, ret *native.Type) []arrSig {
	return []arrSig{{[]*native.Type{a}, []*native.Type{ret}}}
}

// sig2 builds a single two-argument signature: [a, b] -> [ret].
func sig2(a, b, ret *native.Type) []arrSig {
	return []arrSig{{[]*native.Type{a, b}, []*native.Type{ret}}}
}
