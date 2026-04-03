package native

import (
	"fmt"
	"strconv"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// mergeFunc returns the "merge" native function definition.
// merge has forward precedence and multiple signatures:
//   - [list, map] — create new list with map's integer keys replacing elements
//   - [map, list] — create new list from list, appending map's in-range integer-keyed values
//   - [any, any]  — deep-merges the second value into the first using voxgig struct Merge
func mergeFunc() engine.NativeFunc {
	return engine.NativeFunc{
		Name:             "merge",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: mergeListMapHandler,
			},
			{
				Args:    []engine.Type{engine.TMap, engine.TList},
				Handler: mergeMapListHandler,
			},
			{
				Args:    []engine.Type{engine.TAny, engine.TAny},
				Handler: mergeHandler,
			},
		},
	}
}

// mergeHandler calls voxgigstruct.Merge on two values, returning the merged result.
func mergeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	a := valueToAny(args[0])
	b := valueToAny(args[1])

	result := voxgigstruct.Merge([]any{a, b})

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}
	return []engine.Value{val}, nil
}

// mergeListMapHandler creates a new list with map's integer keys replacing
// elements at those positions. Non-integer keys and out-of-range indices
// are ignored. The original list is unchanged.
//
//	[a,b,c] merge {1:d} → [a,d,c]
func mergeListMapHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	list := args[0].AsList().Slice()
	m := args[1].AsMap()
	if list == nil || m == nil {
		return nil, fmt.Errorf("merge: expected concrete list and map")
	}

	// Copy the list.
	result := make([]engine.Value, len(list))
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

	return []engine.Value{engine.NewList(result)}, nil
}

// mergeMapListHandler creates a new list from the list argument, with
// map's in-range integer-keyed values appended at their positions.
// Non-integer keys are ignored. Keys beyond the list length extend it.
// Keys within range replace existing elements.
//
//	{3:d,x:X} merge [a,b,c] → [a,b,c,d]
func mergeMapListHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	m := args[0].AsMap()
	list := args[1].AsList().Slice()
	if m == nil || list == nil {
		return nil, fmt.Errorf("merge: expected concrete map and list")
	}

	// Start with a copy of the list.
	result := make([]engine.Value, len(list))
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

	return []engine.Value{engine.NewList(result)}, nil
}
