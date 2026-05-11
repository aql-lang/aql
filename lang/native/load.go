package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
)

// The "load" word is registered via the consolidated Natives slice in
// natives.go.
//
// loadEntityHandler handles load with an Entity object instance.
func loadEntityHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := entityToAPIMap(args[0])
	return loadAPIHandler([]engine.Value{engine.NewMap(apiMap)}, ctx, stack, r)
}

// loadEntityOptsHandler handles load with an Entity object instance and an options map.
// Sig is opts-first: args[0]=opts, args[1]=entity.
func loadEntityOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := entityToAPIMapWithOpts(args[1], args[0].AsMap(), "query")
	return loadAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// loadAPIOptsHandler handles load with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
// Sig is opts-first: args[0]=opts, args[1]=apiMap (pattern-matched).
func loadAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := mergeAPIOptions(args[1].AsMap(), args[0].AsMap(), "query")
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
	filter := args[0].AsMap()
	rows := args[1].AsList().Slice()

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
