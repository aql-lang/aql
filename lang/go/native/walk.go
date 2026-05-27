package native

import (
	"fmt"
	"strings"

	voxgigstruct "github.com/voxgig/struct"
)

// The "walk" word is registered via the consolidated Natives slice in
// natives.go. This file keeps the leaf-only/before/before-after handlers
// plus the makeWalkApply helper that bridges AQL callbacks into
// voxgigstruct.Walk.
//
// walkHandler uses voxgigstruct.Walk to traverse the value depth-first,
// collecting each leaf node into a list of maps with "path" and "value" keys.
func walkHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	data := valueToAny(args[0])

	var leaves []Value

	voxgigstruct.Walk(data, func(key *string, val any, parent any, path []string) any {
		// Only collect leaf nodes (non-map, non-list values).
		if !voxgigstruct.IsNode(val) {
			leaf := NewOrderedMap()

			pathStr := strings.Join(path, ".")
			leaf.Set("path", NewString(pathStr))

			v, err := anyToValue(val)
			if err != nil {
				v = NewString(fmt.Sprintf("%v", val))
			}
			leaf.Set("value", v)

			leaves = append(leaves, NewMap(leaf))
		}
		return val
	})

	if leaves == nil {
		leaves = []Value{}
	}
	return []Value{NewList(leaves)}, nil
}

// makeWalkApply creates a voxgigstruct.WalkApply from an AQL function callback.
// The callback receives a {key, value, path} map and its return value replaces
// the node in the tree.
func makeWalkApply(cb Value, r *Registry, callErr *error) func(*string, any, any, []string) any {
	return func(key *string, val any, parent any, path []string) any {
		if *callErr != nil {
			return val
		}

		leaf := NewOrderedMap()

		if key != nil {
			leaf.Set("key", NewString(*key))
		} else {
			leaf.Set("key", NewString(""))
		}

		pathStr := strings.Join(path, ".")
		leaf.Set("path", NewString(pathStr))

		v, err := anyToValue(val)
		if err != nil {
			v = NewString(fmt.Sprintf("%v", val))
		}
		leaf.Set("value", v)

		cbArgs := []Value{NewMap(leaf)}
		cbSig := MatchFnSig(cb, cbArgs)
		if cbSig == nil {
			*callErr = fmt.Errorf("walk: no matching callback signature")
			return val
		}
		var cbCaps []CapturedBinding
		if fd, ok := cb.Data.(FnDefInfo); ok {
			cbCaps = fd.Captured
		}
		cbResult, err := r.CallAQL(cbSig, cbArgs, cbCaps)
		if err != nil {
			*callErr = err
			return val
		}
		if len(cbResult) > 0 {
			return valueToAny(cbResult[0])
		}
		return val
	}
}

// walkBeforeHandler walks the value depth-first with a before callback (pre-order).
// The callback receives a {key, value, path} map for each node and its return
// value replaces the node. Returns the transformed tree.
func walkBeforeHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	beforeCb := args[0]
	data := valueToAny(args[1])

	var callErr error
	beforeApply := makeWalkApply(beforeCb, r, &callErr)

	result := voxgigstruct.Walk(data, beforeApply)

	if callErr != nil {
		return nil, fmt.Errorf("walk: before callback error: %w", callErr)
	}

	resultVal, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("walk: error converting result: %w", err)
	}
	return []Value{resultVal}, nil
}

// walkBeforeAfterHandler walks the value depth-first with before (pre-order)
// and after (post-order) callbacks. Both callbacks receive a {key, value, path}
// map and their return values replace the node. Returns the transformed tree.
func walkBeforeAfterHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	beforeCb := args[0]
	afterCb := args[1]
	data := valueToAny(args[2])

	var callErr error
	beforeApply := makeWalkApply(beforeCb, r, &callErr)
	afterApply := makeWalkApply(afterCb, r, &callErr)

	result := voxgigstruct.Walk(data, beforeApply, afterApply)

	if callErr != nil {
		return nil, fmt.Errorf("walk: callback error: %w", callErr)
	}

	resultVal, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("walk: error converting result: %w", err)
	}
	return []Value{resultVal}, nil
}
