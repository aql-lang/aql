package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// pushFunc returns the "push" native function definition.
// push appends element(s) to the end of a list, returning a new list.
// If the element is a list, its elements are spread (like JS args spread).
//
//	push [a,b] c     → [a,b,c]
//	push [a,b] [c,d] → [a,b,c,d]
func pushFunc() NativeFunc {
	return NativeFunc{
		Name:             "push",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TAny},
				Handler: pushHandler,
			},
		},
	}
}

func pushHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList()
	if list == nil {
		return nil, fmt.Errorf("push: expected concrete list")
	}
	elem := args[1]

	result := make([]engine.Value, len(list))
	copy(result, list)

	// If element is a list, spread its elements.
	if elem.VType.Equal(engine.TList) && elem.Data != nil {
		if elems := elem.AsList(); elems != nil {
			result = append(result, elems...)
		}
	} else {
		result = append(result, elem)
	}

	return []engine.Value{engine.NewList(result)}, nil
}

// popFunc returns the "pop" native function definition.
// pop removes the last element from a list, returning the new list
// and the removed element.
//
//	pop [a,b,c] → [a,b] c
func popFunc() NativeFunc {
	return NativeFunc{
		Name:             "pop",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList},
				Handler: popHandler,
			},
		},
	}
}

func popHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList()
	if list == nil || len(list) == 0 {
		return nil, fmt.Errorf("pop: cannot pop from empty list")
	}

	newList := make([]engine.Value, len(list)-1)
	copy(newList, list[:len(list)-1])
	popped := list[len(list)-1]

	return []engine.Value{engine.NewList(newList), popped}, nil
}

// unshiftFunc returns the "unshift" native function definition.
// unshift prepends element(s) to the beginning of a list, returning a new list.
// If the element is a list, its elements are spread.
//
//	unshift [a,b] c     → [c,a,b]
//	unshift [a,b] [c,d] → [c,d,a,b]
func unshiftFunc() NativeFunc {
	return NativeFunc{
		Name:             "unshift",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TAny},
				Handler: unshiftHandler,
			},
		},
	}
}

func unshiftHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList()
	if list == nil {
		return nil, fmt.Errorf("unshift: expected concrete list")
	}
	elem := args[1]

	var prefix []engine.Value
	// If element is a list, spread its elements.
	if elem.VType.Equal(engine.TList) && elem.Data != nil {
		if elems := elem.AsList(); elems != nil {
			prefix = elems
		}
	} else {
		prefix = []engine.Value{elem}
	}

	result := make([]engine.Value, 0, len(prefix)+len(list))
	result = append(result, prefix...)
	result = append(result, list...)

	return []engine.Value{engine.NewList(result)}, nil
}

// shiftFunc returns the "shift" native function definition.
// shift removes the first element from a list, returning the new list
// and the removed element.
//
//	shift [a,b,c] → [b,c] a
func shiftFunc() NativeFunc {
	return NativeFunc{
		Name:             "shift",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TList},
				Handler: shiftHandler,
			},
		},
	}
}

func shiftHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList()
	if list == nil || len(list) == 0 {
		return nil, fmt.Errorf("shift: cannot shift from empty list")
	}

	shifted := list[0]
	newList := make([]engine.Value, len(list)-1)
	copy(newList, list[1:])

	return []engine.Value{engine.NewList(newList), shifted}, nil
}
