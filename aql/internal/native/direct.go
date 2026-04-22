package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// directFunc returns the "direct" native function definition.
// direct has forward precedence and one signature:
//   - [map(kind:"api")] — makes an HTTP request via the SDK
func RegisterDirect(r *engine.Registry) {
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:             "direct",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			{
				Args:     []engine.Type{engine.TMap},
				Handler:  directAPIHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
		},
	})
}

// directAPIHandler handles direct with {kind:"api", spec:String, path:String, method:String, ...}.
// It calls SDK.Direct() and returns the result as a map with ok, status, headers, data.
func directAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := args[0].AsMap()

	sdkInst, _, err := getSDK(apiMap, "direct", r)
	if err != nil {
		return nil, err
	}

	fetchargs := buildFetchArgs(apiMap)

	result, err := sdkInst.Direct(fetchargs)
	if err != nil {
		return nil, fmt.Errorf("direct: %w", err)
	}

	v, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("direct: converting result: %w", err)
	}

	return []engine.Value{v}, nil
}
