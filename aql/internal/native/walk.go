package native

import (
	"fmt"
	"strings"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// walkFunc returns the "walk" native function definition.
// walk is stack-only and has three signatures:
//   - [any, function, function] — walks the value with before and after callbacks,
//     returning the transformed tree
//   - [any, function] — walks the value with a before callback,
//     returning the transformed tree
//   - [any] — walks the value and collects all leaf nodes
//     as a list of {path, value} maps
func walkFunc() NativeFunc {
	return NativeFunc{
		Name:             "walk",
		ForwardPrecedence: false,
		Signatures: []NativeSig{
			{
				Args:    []engine.Type{engine.TAny, engine.TFunction, engine.TFunction},
				Handler: walkBeforeAfterHandler,
			},
			{
				Args:    []engine.Type{engine.TAny, engine.TFunction},
				Handler: walkBeforeHandler,
			},
			{
				Args:    []engine.Type{engine.TAny},
				Handler: walkHandler,
			},
		},
	}
}

// walkHandler uses voxgigstruct.Walk to traverse the value depth-first,
// collecting each leaf node into a list of maps with "path" and "value" keys.
func walkHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])

	var leaves []engine.Value

	voxgigstruct.Walk(data, func(key *string, val any, parent any, path []string) any {
		// Only collect leaf nodes (non-map, non-list values).
		if !voxgigstruct.IsNode(val) {
			leaf := engine.NewOrderedMap()

			pathStr := strings.Join(path, ".")
			leaf.Set("path", engine.NewString(pathStr))

			v, err := anyToValue(val)
			if err != nil {
				v = engine.NewString(fmt.Sprintf("%v", val))
			}
			leaf.Set("value", v)

			leaves = append(leaves, engine.NewMap(leaf))
		}
		return val
	})

	if leaves == nil {
		leaves = []engine.Value{}
	}
	return []engine.Value{engine.NewList(leaves)}, nil
}

// makeWalkApply creates a voxgigstruct.WalkApply from an AQL function callback.
// The callback receives a {key, value, path} map and its return value replaces
// the node in the tree.
func makeWalkApply(cb engine.Value, r *engine.Registry, callErr *error) func(*string, any, any, []string) any {
	return func(key *string, val any, parent any, path []string) any {
		if *callErr != nil {
			return val
		}

		leaf := engine.NewOrderedMap()

		if key != nil {
			leaf.Set("key", engine.NewString(*key))
		} else {
			leaf.Set("key", engine.NewString(""))
		}

		pathStr := strings.Join(path, ".")
		leaf.Set("path", engine.NewString(pathStr))

		v, err := anyToValue(val)
		if err != nil {
			v = engine.NewString(fmt.Sprintf("%v", val))
		}
		leaf.Set("value", v)

		cbArgs := []engine.Value{engine.NewMap(leaf)}
		cbSig := engine.MatchFnSig(cb, cbArgs)
		if cbSig == nil {
			*callErr = fmt.Errorf("walk: no matching callback signature")
			return val
		}
		cbResult, err := r.CallAQL(cbSig, cbArgs)
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
func walkBeforeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	beforeCb := args[1]

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
	return []engine.Value{resultVal}, nil
}

// walkBeforeAfterHandler walks the value depth-first with before (pre-order)
// and after (post-order) callbacks. Both callbacks receive a {key, value, path}
// map and their return values replace the node. Returns the transformed tree.
func walkBeforeAfterHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	data := valueToAny(args[0])
	beforeCb := args[1]
	afterCb := args[2]

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
	return []engine.Value{resultVal}, nil
}
