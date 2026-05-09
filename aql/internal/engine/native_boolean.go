package engine

// booleanNatives covers the boolean / logical-connective words.
//
// `or` and `and` short-circuit and return the "winning" operand
// rather than a pure boolean. `otherwise` is None-coalescing
// (distinct from `or` which short-circuits on falsy, so 0 or 5 = 5
// but 0 otherwise 5 = 0). The classical connectives (xor, nand,
// nor, xnor, iff) coerce non-booleans through CoerceBoolean.
var booleanNatives = []NativeFunc{
	{
		Name:              "or",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean, TBoolean}, BarrierPos: 1, Handler: orHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny, TAny}, BarrierPos: 1, Handler: orHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "and",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean, TBoolean}, Handler: andHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny, TAny}, Handler: andHandler, Returns: []Type{TAny}},
		},
	},
	{
		Name:              "not",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean}, Handler: notHandler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny}, Handler: notHandler, Returns: []Type{TBoolean}},
		},
	},
	{
		Name:              "otherwise",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TAny, TAny}, BarrierPos: 1, Handler: otherwiseHandler, Returns: []Type{TAny}},
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
		Name:              name,
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{Args: []Type{TBoolean, TBoolean}, Handler: handler, Returns: []Type{TBoolean}},
			{Args: []Type{TAny, TAny}, Handler: handler, Returns: []Type{TBoolean}},
		},
	}
}

func orHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

func andHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

func notHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!CoerceBoolean(args[0]))}, nil
}

func otherwiseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if args[1].VType.Equal(TNone) {
		return []Value{args[0]}, nil
	}
	return []Value{args[1]}, nil
}
