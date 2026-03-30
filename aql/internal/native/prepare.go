package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// prepareFunc returns the "prepare" native function definition.
// prepare has forward precedence and one signature:
//   - [map(kind:"api")] — prepares a fetch definition via the SDK
func prepareFunc() NativeFunc {
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	return NativeFunc{
		Name:             "prepare",
		ForwardPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:     []engine.Type{engine.TMap},
				Handler:  prepareAPIHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
		},
	}
}

// prepareAPIHandler handles prepare with {kind:"api", spec:String, path:String, method:String, ...}.
// It calls SDK.Prepare() and returns the fetch definition as a map.
func prepareAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := args[0].AsMap()

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

	return []engine.Value{v}, nil
}

// buildFetchArgs extracts fetch arguments (path, method, headers, body, params, query)
// from an API options map, excluding the kind/spec/entity control fields.
func buildFetchArgs(apiMap *engine.OrderedMap) map[string]any {
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
