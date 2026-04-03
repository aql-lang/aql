package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// pushFunc returns the "push" native function definition.
// push appends a single element to the end of a list, returning a new list.
//
//	push 99 [1,2,3] → [1,2,3,99]
//	[1,2,3] 99 push → [1,2,3,99]
func pushFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:              "push",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TList},
				Handler: pushHandler,
			},
		},
	}
}

func pushHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	newElem := args[0]
	list := args[1].AsList().Slice()
	if list == nil {
		return nil, fmt.Errorf("push: expected concrete list")
	}

	result := make([]engine.Value, len(list)+1)
	copy(result, list)
	result[len(list)] = newElem

	return []engine.Value{engine.NewList(result)}, nil
}

// popFunc returns the "pop" native function definition.
// pop removes the last element from a list, returning the new list
// and the removed element.
//
//	pop [a,b,c] → [a,b] c
//	[a,b,c] pop → [a,b] c
func popFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:              "pop",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TList},
				Handler: popHandler,
			},
		},
	}
}

func popHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList().Slice()
	if list == nil || len(list) == 0 {
		return nil, fmt.Errorf("pop: cannot pop from empty list")
	}

	newList := make([]engine.Value, len(list)-1)
	copy(newList, list[:len(list)-1])
	popped := list[len(list)-1]

	return []engine.Value{engine.NewList(newList), popped}, nil
}

// unshiftFunc returns the "unshift" native function definition.
// unshift prepends a single element to the beginning of a list, returning a new list.
//
//	unshift 99 [1,2,3] → [99,1,2,3]
//	[1,2,3] 99 unshift → [99,1,2,3]
func unshiftFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:              "unshift",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TList},
				Handler: unshiftHandler,
			},
		},
	}
}

func unshiftHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	newElem := args[0]
	list := args[1].AsList().Slice()
	if list == nil {
		return nil, fmt.Errorf("unshift: expected concrete list")
	}

	result := make([]engine.Value, len(list)+1)
	result[0] = newElem
	copy(result[1:], list)

	return []engine.Value{engine.NewList(result)}, nil
}

// shiftFunc returns the "shift" native function definition.
// shift removes the first element from a list, returning the new list
// and the removed element.
//
//	shift [a,b,c] → [b,c] a
//	[a,b,c] shift → [b,c] a
func shiftFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:              "shift",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TList},
				Handler: shiftHandler,
			},
		},
	}
}

func shiftHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList().Slice()
	if list == nil || len(list) == 0 {
		return nil, fmt.Errorf("shift: cannot shift from empty list")
	}

	shifted := list[0]
	newList := make([]engine.Value, len(list)-1)
	copy(newList, list[1:])

	return []engine.Value{engine.NewList(newList), shifted}, nil
}
