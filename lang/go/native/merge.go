package native

import (
	"fmt"
	"strconv"

	voxgigstruct "github.com/voxgig/struct"
)

// The "merge" word is registered via the consolidated Natives slice in
// natives.go.
//
// mergeHandler calls voxgigstruct.Merge on two values, returning the merged result.
func mergeHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	a := valueToAny(args[0])
	b := valueToAny(args[1])

	result := voxgigstruct.Merge([]any{a, b})

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}
	return []Value{val}, nil
}

// mergeListMapHandler creates a new list with map's integer keys replacing
// elements at those positions. Non-integer keys and out-of-range indices
// are ignored. The original list is unchanged.
//
//	[a,b,c] merge {1:d} → [a,d,c]
func mergeListMapHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	_lst, _ := AsList(args[0])
	list := _lst.Slice()
	m, _ := AsMap(args[1])
	if list == nil || m == nil {
		return nil, r.AqlError("merge_error", "merge: expected concrete list and map", "merge")
	}

	// Copy the list.
	result := make([]Value, len(list))
	copy(result, list)

	// Apply map's integer-keyed values.
	for _, key := range m.Keys() {
		idx, err := strconv.Atoi(key)
		if err != nil {
			continue // non-integer key, ignore
		}
		if idx < 0 {
			continue // negative index, ignore
		}
		val, _ := m.Get(key)
		if idx < len(result) {
			result[idx] = val
		} else if idx == len(result) {
			result = append(result, val)
		}
		// idx > len(result): gap, ignore
	}

	return []Value{NewList(result)}, nil
}

// mergeMapListHandler creates a new list from the list argument, with
// map's in-range integer-keyed values appended at their positions.
// Non-integer keys are ignored. Keys beyond the list length extend it.
// Keys within range replace existing elements.
//
//	{3:d,x:X} merge [a,b,c] → [a,b,c,d]
func mergeMapListHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	m, _ := AsMap(args[0])
	_lst, _ := AsList(args[1])
	list := _lst.Slice()
	if m == nil || list == nil {
		return nil, r.AqlError("merge_error", "merge: expected concrete map and list", "merge")
	}

	// Start with a copy of the list.
	result := make([]Value, len(list))
	copy(result, list)

	// Apply map's integer-keyed values.
	for _, key := range m.Keys() {
		idx, err := strconv.Atoi(key)
		if err != nil {
			continue // non-integer key, ignore
		}
		if idx < 0 {
			continue // negative index, ignore
		}
		val, _ := m.Get(key)
		if idx < len(result) {
			result[idx] = val
		} else if idx == len(result) {
			// Extend by one — append at the end.
			result = append(result, val)
		}
		// idx > len(result): gap, ignore
	}

	return []Value{NewList(result)}, nil
}
