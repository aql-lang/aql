package engine

// booleanNatives covers the boolean / logical-connective words that
// remain in the aql production layer. The canonical core trio —
// `not`, `and`, `or` — has been moved into aqleng (see
// eng/go/core_boolean.go) and is installed here via
// eng.RegisterCoreBoolean from register.go.
//
// What stays here:
//
//	`otherwise` — None-coalescing (distinct from `or` which
//	              short-circuits on falsy, so 0 or 5 = 5 but
//	              0 otherwise 5 = 0).
//	`xor`/`nand`/`nor`/`iff`/`xnor` — classical connectives that
//	                                  coerce non-booleans via
//	                                  CoerceBoolean.
var booleanNatives = []NativeFunc{
	{
		Name:        "otherwise",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TAny, TAny}, BarrierPos: 1, Handler: otherwiseHandler, Returns: []*Type{TAny}},
		},
	},
	boolBinaryNative("xor", func(a, b bool) bool { return a != b }),
	boolBinaryNative("nand", func(a, b bool) bool { return !(a && b) }),
	boolBinaryNative("nor", func(a, b bool) bool { return !(a || b) }),
	boolBinaryNative("iff", func(a, b bool) bool { return a == b }),
	boolBinaryNative("xnor", func(a, b bool) bool { return a == b }),
}

// boolBinaryNative builds a NativeFunc with two signatures —
// [TBoolean, TBoolean] and [TAny, TAny] — both routing through the
// same boolean-coercing handler. Used for the classical connectives
// (xor, nand, nor, iff, xnor) that share an identical shape.
func boolBinaryNative(name string, fn func(a, b bool) bool) NativeFunc {
	handler := func(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
		a := CoerceBoolean(args[0])
		b := CoerceBoolean(args[1])
		return []Value{NewBoolean(fn(a, b))}, nil
	}
	return NativeFunc{
		Name:        name,
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, Handler: handler, Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TAny}, Handler: handler, Returns: []*Type{TBoolean}},
		},
	}
}

func otherwiseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if args[1].VType.Equal(TNone) {
		return []Value{args[0]}, nil
	}
	return []Value{args[1]}, nil
}
