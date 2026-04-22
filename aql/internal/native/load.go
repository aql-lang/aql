package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// loadFunc returns the "load" native function definition.
// load has forward precedence and three signatures:
//   - [map(kind:"api")] — loads a single entity via the SDK
//   - [table, map]      — finds a single record by matching the map's key-value pairs (typically {id:"..."})
//   - [map, map]        — record type + filter: returns empty map
func RegisterLoad(r *engine.Registry) {
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	r.RegisterNativeFunc(engine.NativeFunc{
		Name:             "load",
		ForwardPrecedence: true,
		Signatures: []engine.NativeSig{
			// Entity object signatures (highest priority).
			{
				Args:    []engine.Type{engine.TResourceEntity, engine.TMap},
				Handler: loadEntityOptsHandler,
			},
			{
				Args:    []engine.Type{engine.TResourceEntity},
				Handler: loadEntityHandler,
			},
			{
				Args:     []engine.Type{engine.TMap, engine.TMap},
				Handler:  loadAPIOptsHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:     []engine.Type{engine.TMap},
				Handler:  loadAPIHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: loadHandler,
			},
			{
				Args:    []engine.Type{engine.TMap, engine.TMap},
				Handler: loadRecordHandler,
			},
		},
	})
}

// loadEntityHandler handles load with an Entity object instance.
func loadEntityHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := entityToAPIMap(args[0])
	return loadAPIHandler([]engine.Value{engine.NewMap(apiMap)}, ctx, stack, r)
}

// loadEntityOptsHandler handles load with an Entity object instance and an options map.
func loadEntityOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := entityToAPIMapWithOpts(args[0], args[1].AsMap(), "query")
	return loadAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// loadAPIOptsHandler handles load with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
func loadAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := mergeAPIOptions(args[0].AsMap(), args[1].AsMap(), "query")
	return loadAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// loadAPIHandler handles load with {kind:"api", spec:String, entity:String, query:{...}}.
func loadAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := args[0].AsMap()

	sdkInst, entityName, err := getSDK(apiMap, "load", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Load(extractQuery(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("load: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "load")
	if err != nil {
		return nil, err
	}

	return []engine.Value{v}, nil
}

// loadRecordHandler handles load on a record type (not a table).
// Returns an empty map.
func loadRecordHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewMap(engine.NewOrderedMap())}, nil
}

// loadHandler finds and returns a single record matching the filter.
// Returns an error if no matching record is found.
func loadHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := args[0].AsList().Slice()
	filter := args[1].AsMap()

	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			continue
		}
		rec := row.AsMap()
		if recordMatches(rec, filter) {
			return []engine.Value{row}, nil
		}
	}

	return nil, fmt.Errorf("load: no record found matching filter")
}
