package native

// booleanNatives covers the boolean / logical-connective words.
// The classical trio (`not`, `and`, `or`) follows Lisp/Python
// short-circuit semantics — `0 or 5` returns 5, `1 and 2` returns 2,
// and `not v` always returns a Boolean. The remaining connectives
// (`otherwise`, `xor`, `nand`, `nor`, `iff`, `xnor`) coerce
// non-Boolean inputs via CoerceBoolean.
//
// Algorithms live in the eng layer (CoerceBoolean,
// FlattenDisjunctAlts, TandValues etc.); this file owns the word
// names, signatures, and dispatch wiring.
var booleanNatives = []NativeFunc{
	{
		Name:        "not",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TBoolean}, Handler: notHandler, Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny}, Handler: notHandler, Returns: []*Type{TBoolean}},
		},
	},
	{
		Name:        "and",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, Handler: andHandler, Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TAny}, Handler: andHandler, Returns: []*Type{TAny}},
		},
	},
	{
		Name:        "or",
		ForwardArgs: true,
		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, BarrierPos: 1, Handler: orHandler, Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TAny}, BarrierPos: 1, Handler: orHandler, Returns: []*Type{TAny}},
		},
	},
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

// notHandler always returns a Boolean.
func notHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{NewBoolean(!CoerceBoolean(args[0]))}, nil
}

// andHandler returns args[1] when falsy, else args[0]. Short-circuit.
func andHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
}

// orHandler returns args[1] when truthy, else args[0]. Short-circuit.
func orHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if CoerceBoolean(args[1]) {
		return []Value{args[1]}, nil
	}
	return []Value{args[0]}, nil
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
	if args[1].Parent.Equal(TNone) {
		return []Value{args[0]}, nil
	}
	return []Value{args[1]}, nil
}
