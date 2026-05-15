package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
)

// The "direct" word is registered via the consolidated Natives slice in
// natives.go.
//
// directAPIHandler handles direct with {kind:"api", spec:String, path:String, method:String, ...}.
// It calls SDK.Direct() and returns the result as a map with ok, status, headers, data.
func directAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := engine.AsMap(args[0])

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
