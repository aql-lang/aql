package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// The list-mutation words (push/pop/unshift/shift) are registered via the
// consolidated Natives slice in natives.go.
//
// push appends a single element to the end of a list, returning a new list.
//
//	push 99 [1,2,3] → [1,2,3,99]
//	[1,2,3] 99 push → [1,2,3,99]
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

// pop removes the last element from a list, returning the new list
// and the removed element.
//
//	pop [a,b,c] → [a,b] c
//	[a,b,c] pop → [a,b] c
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

// unshift prepends a single element to the beginning of a list, returning a new list.
//
//	unshift 99 [1,2,3] → [99,1,2,3]
//	[1,2,3] 99 unshift → [99,1,2,3]
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

// shift removes the first element from a list, returning the new list
// and the removed element.
//
//	shift [a,b,c] → [b,c] a
//	[a,b,c] shift → [b,c] a
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
