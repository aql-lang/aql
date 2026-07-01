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
			{Args: []*Type{TMap, TAny}, Impl: Go(transformHandler), BarrierPos: -1},
		},
	},
	{
		Name: "merge",
		Signatures: []NativeSig{
			{Args: []*Type{TList, TMap}, Impl: Go(mergeListMapHandler), BarrierPos: -1},
			{Args: []*Type{TMap, TList}, Impl: Go(mergeMapListHandler), BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(mergeHandler), BarrierPos: -1},
		},
	},
	{
		Name: "validate",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Impl: Go(validateHandler), BarrierPos: -1},
		},
	},
	{
		Name: "getpath",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny}, Impl: Go(getpathHandler), BarrierPos: -1},
			// Lens form: `getpath $.a.b m` reads through a Reach (honors
			// per-segment getr strictness + computed keys, natively).
			{Args: []*Type{TReach, TAny}, Impl: Go(getpathReachHandler), BarrierPos: -1},
		},
	},
	{
		Name: "setpath",
		Signatures: []NativeSig{
			{Args: []*Type{TString, TAny, TAny}, Impl: Go(setpathHandler), BarrierPos: -1},
			{Args: []*Type{TAny, TString, TAny}, Impl: Go(setpathHandler), BarrierPos: -1},
			// Lens forms: a Reach path in the leading or middle slot
			// (`setpath $.a.b 9 m` / `setpath m $.a.b 9`).
			{Args: []*Type{TReach, TAny, TAny}, Impl: Go(setpathHandler), BarrierPos: -1},
			{Args: []*Type{TAny, TReach, TAny}, Impl: Go(setpathHandler), BarrierPos: -1},
		},
	},
	{
		Name: "inject",
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TAny}, Impl: Go(injectHandler), BarrierPos: -1},
		},
	},
	{
		Name: "clone",
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Impl: Go(cloneHandler), ReturnsFn: cloneReturnsFn, BarrierPos: -1},
		},
	},
	{
		Name: "walk",
		Signatures: []NativeSig{
			{Args: []*Type{TFunction, TFunction, TAny}, Impl: Go(walkBeforeAfterHandler), BarrierPos: -1},
			{Args: []*Type{TFunction, TAny}, Impl: Go(walkBeforeHandler), BarrierPos: -1},
			{Args: []*Type{TAny}, Impl: Go(walkHandler), BarrierPos: -1},
		},
	},
	{
		Name: "selector",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Impl: Go(selectorHandler), BarrierPos: -1},
		},
	},
	{
		Name: "items",
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Impl: Go(itemsHandler), Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "jsonify",
		Signatures: []NativeSig{
			{Args: []*Type{TMap, TAny}, Impl: Go(jsonifyFlagsHandler), Returns: []*Type{TString}, BarrierPos: -1},
			{Args: []*Type{TAny}, Impl: Go(jsonifyDefaultHandler), Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		// parse — jsonic/JSON text → data, the decode complement of
		// jsonify. DATA context (nothing evaluates: unquoted text →
		// strings, numbers → numbers, true/false → booleans); accepts
		// the jsonic superset so strict JSON parses too; malformed
		// input raises [aql/parse_error]. See design/PARSING.10.md §2.
		Name: "parse",
		Signatures: []NativeSig{
			{Args: []*Type{TString}, Impl: Go(parseTextHandler), Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		// reify — hydrate a class instance from JSON text or a Node.
		// The inverse of the instance-aware jsonify; the target is an
		// explicit class type or a tor union of classes ($class then
		// selects the member). See reify.go + design/CLASS-OBJECT.10.md §3e.
		Name: "reify",
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TMap}, Impl: Go(reifyHandler), BarrierPos: -1},
			{Args: []*Type{TAny, TString}, Impl: Go(reifyHandler), BarrierPos: -1},
		},
	},
	{
		Name: "nodify",
		Signatures: []NativeSig{
			{Args: []*Type{TAny}, Impl: Go(nodifyHandler), BarrierPos: -1},
		},
	},
}
