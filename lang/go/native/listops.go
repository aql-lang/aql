package native

import (
	"fmt"
)

// The list-mutation words (push/pop/unshift/shift) are registered via the
// consolidated Natives slice in natives.go.
//
// push appends a single element to the end of a list, returning a new list.
//
//	push 99 [1,2,3] → [1,2,3,99]
//	[1,2,3] 99 push → [1,2,3,99]
func pushHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	newElem := args[0]
	_lst, _ := AsList(args[1])
	list := _lst.Slice()
	if list == nil {
		return nil, fmt.Errorf("push: expected concrete list")
	}

	result := make([]Value, len(list)+1)
	copy(result, list)
	result[len(list)] = newElem

	return []Value{NewList(result)}, nil
}

// pop removes the last element from a list, returning the new list
// and the removed element.
//
//	pop [a,b,c] → [a,b] c
//	[a,b,c] pop → [a,b] c
func popHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	list := _lst.Slice()
	if len(list) == 0 {
		return nil, fmt.Errorf("pop: cannot pop from empty list")
	}

	newList := make([]Value, len(list)-1)
	copy(newList, list[:len(list)-1])
	popped := list[len(list)-1]

	return []Value{NewList(newList), popped}, nil
}

// unshift prepends a single element to the beginning of a list, returning a new list.
//
//	unshift 99 [1,2,3] → [99,1,2,3]
//	[1,2,3] 99 unshift → [99,1,2,3]
func unshiftHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	newElem := args[0]
	_lst, _ := AsList(args[1])
	list := _lst.Slice()
	if list == nil {
		return nil, fmt.Errorf("unshift: expected concrete list")
	}

	result := make([]Value, len(list)+1)
	result[0] = newElem
	copy(result[1:], list)

	return []Value{NewList(result)}, nil
}

// shift removes the first element from a list, returning the new list
// and the removed element.
//
//	shift [a,b,c] → [b,c] a
//	[a,b,c] shift → [b,c] a
func shiftHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	list := _lst.Slice()
	if len(list) == 0 {
		return nil, fmt.Errorf("shift: cannot shift from empty list")
	}

	shifted := list[0]
	newList := make([]Value, len(list)-1)
	copy(newList, list[1:])

	return []Value{NewList(newList), shifted}, nil
}
