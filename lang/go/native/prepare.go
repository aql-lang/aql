package native

import (
	"fmt"
)

// The "prepare" word is registered via the consolidated Natives slice in
// natives.go. This file keeps both the prepare handler and the shared
// buildFetchArgs helper used by direct.go.
//
// prepareAPIHandler handles prepare with {kind:"api", spec:String, path:String, method:String, ...}.
// It calls SDK.Prepare() and returns the fetch definition as a map.
func prepareAPIHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	apiMap, _ := AsMap(args[0])

	sdkInst, _, err := getSDK(apiMap, "prepare", r)
	if err != nil {
		return nil, err
	}

	fetchargs := buildFetchArgs(apiMap)

	fetchdef, err := sdkInst.Prepare(fetchargs)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}

	v, err := anyToValue(fetchdef)
	if err != nil {
		return nil, fmt.Errorf("prepare: converting result: %w", err)
	}

	return []Value{v}, nil
}

// buildFetchArgs extracts fetch arguments (path, method, headers, body, params, query)
// from an API options map, excluding the kind/spec/entity control fields.
func buildFetchArgs(apiMap ReadMap) map[string]any {
	out := make(map[string]any)
	for _, key := range apiMap.Keys() {
		switch key {
		case "kind", "spec", "entity":
			continue
		default:
			v, _ := apiMap.Get(key)
			out[key] = valueToAny(v)
		}
	}
	return out
}
