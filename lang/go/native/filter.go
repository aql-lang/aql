package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "filter" word is registered via the consolidated Natives slice in
// natives.go.
//
// filterHandler calls voxgigstruct.Filter with an AQL callback as predicate.
// The callback receives a map with "key" and "value" fields and should return
// a boolean indicating whether to keep the item.
// filterHandler keeps the elements of a list/map for which the callback
// returns true. The callback (args[0]) is a Function VALUE invoked once per
// element with a SINGLE {key, value} pair Map — key is the list index (or
// map key) and value is the element. A predicate therefore reads the
// element via `.value`, e.g. `filter ([p:Any] => [p.value gt 3]) xs`. (The
// afn param must be typed — a bare `[p]` parses as a type name.)
func filterHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	cb := args[0]
	data := valueToAny(args[1])

	var callErr error
	result := voxgigstruct.Filter(data, func(pair [2]any) bool {
		if callErr != nil {
			return false
		}

		item := NewOrderedMap()

		keyVal, err := anyToValue(pair[0])
		if err != nil {
			keyVal = NewString(fmt.Sprintf("%v", pair[0]))
		}
		item.Set("key", keyVal)

		valVal, err := anyToValue(pair[1])
		if err != nil {
			valVal = NewString(fmt.Sprintf("%v", pair[1]))
		}
		item.Set("value", valVal)

		cbArgs := []Value{NewMap(item)}
		cbSig := MatchFnSig(cb, cbArgs)
		if cbSig == nil {
			callErr = fmt.Errorf("filter: no matching callback signature")
			return false
		}
		var cbCaps []CapturedBinding
		if fd, ok := cb.Data.(FnDefInfo); ok {
			cbCaps = fd.Captured
		}
		cbResult, err := r.CallAQL(cbSig, cbArgs, cbCaps)
		if err != nil {
			callErr = err
			return false
		}
		if len(cbResult) > 0 && cbResult[0].Parent.ConformsTo(TBoolean) {
			b, _ := AsBoolean(cbResult[0])
			return b
		}
		return false
	})

	if callErr != nil {
		return nil, fmt.Errorf("filter: callback error: %w", callErr)
	}

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	return []Value{val}, nil
}

// filterReachHandler is the lens form of filter: it keeps the elements of a
// list (or the values of a map) for which the receiverless Reach (args[0])
// applies to Boolean true. Unlike the Function form, the reach reads the
// element directly — `filter $.active xs` keeps each x where x.active is true.
func filterReachHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	info, err := AsReach(args[0])
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	keep := func(elem Value) (bool, error) {
		res, err := ApplyReach(r, info, elem)
		if err != nil {
			return false, err
		}
		if res.Parent.ConformsTo(TBoolean) {
			b, _ := AsBoolean(res)
			return b, nil
		}
		return false, nil
	}

	switch {
	case args[1].Parent.ConformsTo(TList) && IsConcrete(args[1]):
		data, _ := AsList(args[1])
		out := make([]Value, 0, data.Len())
		for i := 0; i < data.Len(); i++ {
			elem := data.Get(i)
			ok, err := keep(elem)
			if err != nil {
				return nil, fmt.Errorf("filter: element %d: %w", i, err)
			}
			if ok {
				out = append(out, elem)
			}
		}
		return []Value{NewList(out)}, nil
	case args[1].Parent.ConformsTo(TMap) && IsConcrete(args[1]):
		data, _ := AsMap(args[1])
		out := NewOrderedMap()
		for _, k := range data.Keys() {
			v, _ := data.Get(k)
			ok, err := keep(v)
			if err != nil {
				return nil, fmt.Errorf("filter: key %q: %w", k, err)
			}
			if ok {
				out.Set(k, v)
			}
		}
		return []Value{NewMap(out)}, nil
	default:
		return nil, r.AqlError("filter_error", "filter: lens form expects a concrete list or map", "filter")
	}
}
