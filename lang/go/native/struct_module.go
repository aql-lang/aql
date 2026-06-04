package native

// StructModuleNatives holds the voxgig-struct data-manipulation words that
// were moved out of the core registry into the loadable `aql:struct` module
// (namespace `Struct`). They are registered ONLY into that module's
// sub-registry by modules.BuildStructModule — deliberately absent from the
// global registry (they are no longer in the core Natives slice in
// natives.go). The handlers themselves still live in their feature files
// (clone.go, merge.go, walk.go, transform.go, …) in this package.
//
// Each word is a thin wrapper over a github.com/voxgig/struct utility:
//
//	clone     deep copy of any value
//	getpath   read a dotted path out of a structure
//	setpath   write a value at a dotted path (returns a new structure)
//	inject    resolve `$`-path references embedded in a structure
//	merge     deep, index-wise merge of two structures
//	walk      structural walk, optionally with before/after transforms
//	items     key/value pairs of a map (or index/value of a list)
//	transform shape one structure into another via a spec
//	validate  validate a structure against a shape spec
//	selector  query-select out of a structure
//	jsonify   serialise a value to a jsonic/JSON string
//	nodify    project a value to its Node/Scalar form (voxgig/struct)
//
// All words are all-forward eligible (BarrierPos -1) so they dispatch in
// both forward (`StructUtil.clone {a:1}`) and stack (`{a:1} StructUtil.clone`) form
// through the namespace — the recommended shape for module words (see the
// "Module FnDef Wrappers" note in lang/go/CLAUDE.md). clone/walk were
// stack-only (0) as core words; -1 is a pure dispatch widening (the inner
// native only ever runs via the trivial-delegation short-circuit, where
// there are no forward tokens, so -1 and 0 behave identically there).
var StructModuleNatives = []NativeFunc{
	{
		Name: "transform",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: transformHandler, BarrierPos: -1},
		},
	},
	{
		Name: "merge",
		Signatures: []NativeSig{
			{Args: []*Type{TList, TMap}, Handler: mergeListMapHandler, BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Handler: mergeMapListHandler, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Handler: mergeHandler, BarrierPos: -1},
		},
	},
	{
		Name: "validate",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: validateHandler, BarrierPos: -1},
		},
	},
	{
		Name: "getpath",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny}, Handler: getpathHandler, BarrierPos: -1},
		},
	},
	{
		Name: "setpath",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny, TAny}, Handler: setpathHandler, BarrierPos: -1},
			{Args: []*Type{TAny, TString, TAny}, Handler: setpathHandler, BarrierPos: -1},
		},
	},
	{
		Name: "inject",
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TAny}, Handler: injectHandler, BarrierPos: -1},
		},
	},
	{
		Name: "clone",
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: cloneHandler, BarrierPos: -1},
		},
	},
	{
		Name: "walk",
		Signatures: []NativeSig{
			{Args: []*Type{TFunction, TFunction, TAny}, Handler: walkBeforeAfterHandler, BarrierPos: -1},
			{Args: []*Type{TFunction, TAny}, Handler: walkBeforeHandler, BarrierPos: -1},
			{Args: []*Type{TAny}, Handler: walkHandler, BarrierPos: -1},
		},
	},
	{
		Name: "selector",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: selectorHandler, BarrierPos: -1},
		},
	},
	{
		Name: "items",
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: itemsHandler, BarrierPos: -1},
		},
	},
	{
		Name: "jsonify",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Handler: jsonifyFlagsHandler, BarrierPos: -1},
			{Args: []*Type{TAny}, Handler: jsonifyDefaultHandler, BarrierPos: -1},
		},
	},
	{
		Name: "nodify",
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Handler: nodifyHandler, BarrierPos: -1},
		},
	},
}
