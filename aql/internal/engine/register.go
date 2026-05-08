package engine

// Register installs the engine's built-in word set on the given
// registry. This is invoked from DefaultRegistry. Word definitions
// themselves live in the various native_*.go and feature files
// alongside their handlers.
func Register(r *Registry) {
	// String
	RegisterUpper(r)
	RegisterLower(r)
	RegisterConcat(r)
	RegisterSplit(r)
	RegisterTrim(r)
	RegisterContains(r)
	RegisterIndexOf(r)
	RegisterReplace(r)
	RegisterChangeCase(r)
	RegisterNormalize(r)
	RegisterRepeat(r)
	RegisterPad(r)
	RegisterMatch(r)
	RegisterEscape(r)

	// Stack ops
	for _, n := range stackNatives {
		r.RegisterNativeFunc(n)
	}

	// Math: basic arithmetic.
	for _, n := range mathNatives {
		r.RegisterNativeFunc(n)
	}

	// Boolean.
	for _, n := range booleanNatives {
		r.RegisterNativeFunc(n)
	}
	RegisterAny(r)
	RegisterAll(r)

	// Comparison
	RegisterComparison(r)

	// Storage
	RegisterSet(r)
	RegisterGet(r)
	RegisterContext(r)

	// Definition
	RegisterDef(r)
	RegisterUndef(r)
	RegisterVar(r)
	RegisterFn(r)
	RegisterCall(r)
	RegisterDblcall(r)
	RegisterArgs(r)
	RegisterPopArgs(r)

	// Type
	RegisterConvert(r)
	RegisterRecord(r)
	RegisterTable(r)
	RegisterObject(r)
	RegisterResource(r)
	RegisterMake(r)
	RegisterTypeDef(r)
	RegisterTypeof(r)
	RegisterFullTypeof(r)
	RegisterIs(r)
	RegisterGuard(r)
	RegisterInspect(r)
	RegisterBase(r)
	RegisterTor(r)
	RegisterTand(r)
	RegisterTany(r)
	RegisterTall(r)

	// Control flow
	RegisterDo(r)
	RegisterIf(r)
	RegisterFor(r)
	RegisterError(r)

	// Accessors
	RegisterGetr(r)

	// I/O
	RegisterFileIO(r)
	RegisterPrint(r)
	RegisterTrace(r)

	// Unify
	RegisterUnify(r)

	// Module
	RegisterModule(r)

	// Array
	RegisterIota(r)
	RegisterShape(r)
	RegisterRank(r)
	RegisterLength(r)
	RegisterReshape(r)
	RegisterArrFlatten(r)
	RegisterArrTranspose(r)
	RegisterReverse(r)
	RegisterTake(r)
	RegisterShed(r)
	RegisterWhere(r)
	RegisterUnique(r)
	RegisterGrade(r)
	RegisterAt(r)
	RegisterSortby(r)
	RegisterMember(r)
	RegisterArrIndexof(r)
	RegisterGroup(r)
	RegisterReplicate(r)
	RegisterExpand(r)
	RegisterWindow(r)
	RegisterPairs(r)

	// Array higher-order
	RegisterEach(r)
	RegisterFold(r)
	RegisterScan(r)
	RegisterOuter(r)
	RegisterInner(r)

	// Temporal
	RegisterTimeout(r)
	RegisterAwait(r)

	// Help
	RegisterHelp(r)
}
