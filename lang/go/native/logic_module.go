package native

// LogicModuleNatives holds the derived boolean connectives moved out of core
// into the aql:logic-util module (LogicUtil namespace):
//
//	nand     not (a and b)
//	nor      not (a or b)
//	iff      a == b (logical biconditional)
//	xnor     a == b (alias of iff)
//	implies  (not a) or b
//
// The everyday connectives — and, or, not, xor, any, all — stay in core.
// boolBinaryNative + impliesHandler live in native_boolean.go / natives.go.
var LogicModuleNatives = []NativeFunc{
	boolBinaryNative("nand", func(a, b bool) bool { return !(a && b) }),
	boolBinaryNative("nor", func(a, b bool) bool { return !(a || b) }),
	boolBinaryNative("iff", func(a, b bool) bool { return a == b }),
	boolBinaryNative("xnor", func(a, b bool) bool { return a == b }),
	{
		Name: "implies",
		Signatures: []Signature{
			{Args: []*Type{TBoolean, TBoolean}, Impl: Go(impliesHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Impl: Go(impliesHandler), Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
}
