package engine

import "fmt"

// stackNatives covers the stack-manipulation primitives. All are
// stack-only (ForwardArgs=false). Argument convention is
// post-§1.4 unified: args[0] is the top of stack, args[1] is the
// next-deeper element, etc. Splice ordering: the returned []Value
// is laid back onto the stack in source order, so an N-arg word
// that returns the same N values produces the inputs unchanged
// (see swap for a worked example).
var stackNatives = []NativeFunc{
	{
		Name:        "dup",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny},
			Handler:   dupHandler,
			ReturnsFn: ReturnsIdentity(0, 0),
		}},
	},
	{
		Name:        "swap",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny},
			Handler:   swapHandler,
			ReturnsFn: ReturnsIdentity(0, 1),
		}},
	},
	{
		Name:        "drop",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny},
			Handler: dropHandler,
			Returns: []*Type{},
		}},
	},
	{
		Name:        "over",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny},
			Handler:   overHandler,
			ReturnsFn: ReturnsIdentity(1, 0, 1),
		}},
	},
	{
		Name:        "rot",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny, TAny},
			Handler:   rotHandler,
			ReturnsFn: ReturnsIdentity(1, 0, 2),
		}},
	},
	{
		Name:        "nip",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny},
			Handler:   nipHandler,
			ReturnsFn: ReturnsIdentity(0),
		}},
	},
	{
		Name:        "tuck",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny},
			Handler:   tuckHandler,
			ReturnsFn: ReturnsIdentity(0, 1, 0),
		}},
	},
	{
		Name:        "dup2",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny},
			Handler:   dup2Handler,
			ReturnsFn: ReturnsIdentity(1, 0, 1, 0),
		}},
	},
	{
		Name:        "swap2",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny, TAny, TAny},
			Handler:   swap2Handler,
			ReturnsFn: ReturnsIdentity(1, 0, 3, 2),
		}},
	},
	{
		Name:        "drop2",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:    []*Type{TAny, TAny},
			Handler: drop2Handler,
			Returns: []*Type{},
		}},
	},
	{
		Name:        "over2",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:      []*Type{TAny, TAny, TAny, TAny},
			Handler:   over2Handler,
			ReturnsFn: ReturnsIdentity(3, 2, 1, 0, 3, 2),
		}},
	},
	{
		Name:        "depth",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			FullStack:        true,
			Handler:          depthHandler,
			CheckFullStackFn: depthCheckFullStack,
		}},
	},
	{
		Name:        "pick",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:             []*Type{TInteger},
			FullStack:        true,
			Handler:          pickHandler,
			CheckFullStackFn: pickCheckFullStack,
		}},
	},
	{
		Name:        "roll",
		ForwardArgs: false,
		Signatures: []NativeSig{{
			Args:             []*Type{TInteger},
			FullStack:        true,
			Handler:          rollHandler,
			CheckFullStackFn: rollCheckFullStack,
		}},
	},
}

func dupHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0], args[0]}, nil
}

func swapHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0], args[1]}, nil
}

func dropHandler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return nil, nil
}

func overHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[1]}, nil
}

func rotHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[2]}, nil
}

func nipHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0]}, nil
}

func tuckHandler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[0], args[1], args[0]}, nil
}

func dup2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[1], args[0]}, nil
}

func swap2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[1], args[0], args[3], args[2]}, nil
}

func drop2Handler(_ []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return nil, nil
}

func over2Handler(args []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) {
	return []Value{args[3], args[2], args[1], args[0], args[3], args[2]}, nil
}

func depthHandler(_ []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	return append(stack, NewInteger(int64(len(stack)))), nil
}

func depthCheckFullStack(_ []Value, stack []Value, _ *Registry) []Value {
	return append(append([]Value(nil), stack...), NewCarrier(TInteger))
}

func pickHandler(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n >= len(stack) {
		return nil, fmt.Errorf("pick: index %d out of range (stack depth %d)", n, len(stack))
	}
	return append(stack, stack[len(stack)-1-n]), nil
}

func pickCheckFullStack(_ []Value, stack []Value, _ *Registry) []Value {
	if len(stack) == 0 {
		return append(append([]Value(nil), stack...), NewCarrier(TAny))
	}
	t := stack[0].VType
	for i := 1; i < len(stack); i++ {
		t = CommonAncestorType(t, stack[i].VType)
		if t.Equal(TAny) {
			break
		}
	}
	return append(append([]Value(nil), stack...), NewCarrier(t))
}

func rollHandler(args []Value, _ map[string]Value, stack []Value, _ *Registry) ([]Value, error) {
	_as0, _ := args[0].AsConcreteInteger()
	n := int(_as0)
	if n < 0 || n >= len(stack) {
		return nil, fmt.Errorf("roll: index %d out of range (stack depth %d)", n, len(stack))
	}
	idx := len(stack) - 1 - n
	result := make([]Value, 0, len(stack))
	result = append(result, stack[:idx]...)
	result = append(result, stack[idx+1:]...)
	result = append(result, stack[idx])
	return result, nil
}

func rollCheckFullStack(_ []Value, stack []Value, _ *Registry) []Value {
	if len(stack) == 0 {
		return nil
	}
	out := append([]Value(nil), stack...)
	t := stack[0].VType
	for i := 1; i < len(stack); i++ {
		t = CommonAncestorType(t, stack[i].VType)
	}
	out[len(out)-1] = NewCarrier(t)
	return out
}
