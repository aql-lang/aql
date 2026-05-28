package native

// booleanNatives covers the boolean / logical-connective words.
// The classical trio (`not`, `and`, `or`) follows Lisp/Python
// short-circuit semantics — `0 or 5` returns 5, `1 and 2` returns 2,
// and `not v` always returns a Boolean. The remaining connectives
// (`otherwise`, `xor`, `nand`, `nor`, `iff`, `xnor`) coerce
// non-Boolean inputs via CoerceBoolean. `any` and `all` are the
// list-reduction forms of `or` / `and`; they always return Boolean.
//
// Algorithms live in the eng layer (CoerceBoolean,
// FlattenDisjunctAlts, TandValues etc.); this file owns the word
// names, signatures, and dispatch wiring.
var booleanNatives = []NativeFunc{
	{
		Name: "not",

		Signatures: []NativeSig{
			{Args: []*Type{TBoolean}, Handler: notHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny}, Handler: notHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "and",

		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, Handler: andHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Handler: andHandler, Returns: []*Type{TAny}, BarrierPos: -1},
		},
	},
	{
		Name: "or",

		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, BarrierPos: 1, Handler: orHandler, Returns: []*Type{TBoolean}},
			{Args: []*Type{TAny, TAny}, BarrierPos: 1, Handler: orHandler, Returns: []*Type{TAny}},
		},
	},
	{
		Name: "otherwise",

		Signatures: []NativeSig{
			{Args: []*Type{TAny, TAny}, BarrierPos: 1, Handler: otherwiseHandler, Returns: []*Type{TAny}},
		},
	},
	boolBinaryNative("xor", func(a, b bool) bool { return a != b }),
	boolBinaryNative("nand", func(a, b bool) bool { return !(a && b) }),
	boolBinaryNative("nor", func(a, b bool) bool { return !(a || b) }),
	boolBinaryNative("iff", func(a, b bool) bool { return a == b }),
	boolBinaryNative("xnor", func(a, b bool) bool { return a == b }),
	{
		Name: "any",

		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: anyHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
	{
		Name: "all",

		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: allHandler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	},
}

// anyHandler returns true iff any element of the list is truthy.
// Empty list returns false (the identity for OR).
func anyHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return []Value{NewBoolean(false)}, nil
	}
	list, _ := AsList(args[0])
	n := list.Len()
	for i := 0; i < n; i++ {
		if CoerceBoolean(list.Get(i)) {
			return []Value{NewBoolean(true)}, nil
		}
	}
	return []Value{NewBoolean(false)}, nil
}

// allHandler returns true iff every element of the list is truthy.
// Empty list returns true (the identity for AND).
func allHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	if !IsConcrete(args[0]) {
		return []Value{NewBoolean(true)}, nil
	}
	list, _ := AsList(args[0])
	n := list.Len()
	for i := 0; i < n; i++ {
		if !CoerceBoolean(list.Get(i)) {
			return []Value{NewBoolean(false)}, nil
		}
	}
	return []Value{NewBoolean(true)}, nil
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
		Name: name,

		Signatures: []NativeSig{
			{Args: []*Type{TBoolean, TBoolean}, Handler: handler, Returns: []*Type{TBoolean}, BarrierPos: -1},
			{Args: []*Type{TAny, TAny}, Handler: handler, Returns: []*Type{TBoolean}, BarrierPos: -1},
		},
	}
}

func otherwiseHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	// IsNoneShape matches both the sentinel `none` AND the bare type
	// literal `None` — handlers throughout the codebase return the
	// latter for "no value", so the LHS check has to cover it too.
	if IsNoneShape(args[1]) {
		return []Value{args[0]}, nil
	}
	return []Value{args[1]}, nil
}
