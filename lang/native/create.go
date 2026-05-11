package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
)

// The "create" word is registered via the consolidated Natives slice in
// natives.go. Handlers below cover the SDK/table/record overloads.
//
// createEntityHandler handles create with an Entity object instance.
func createEntityHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := entityToAPIMap(args[0])
	return createAPIHandler([]engine.Value{engine.NewMap(apiMap)}, ctx, stack, r)
}

// createEntityOptsHandler handles create with an Entity object instance and a data map.
// Sig is opts-first: args[0]=data, args[1]=entity.
func createEntityOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := entityToAPIMapWithOpts(args[1], args[0].AsMap(), "data")
	return createAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// createAPIOptsHandler handles create with {kind:"api",...} and an extra data map.
// The options map is merged into the data field of the API map.
// Sig is opts-first: args[0]=data, args[1]=apiMap (pattern-matched).
func createAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := mergeAPIOptions(args[1].AsMap(), args[0].AsMap(), "data")
	return createAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// createAPIHandler handles create with {kind:"api", spec:String, entity:String, data:{...}}.
func createAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := args[0].AsMap()

	sdkInst, entityName, err := getSDK(apiMap, "create", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Create(extractData(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("create: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "create")
	if err != nil {
		return nil, err
	}

	return []engine.Value{v}, nil
}

// createRecordHandler handles create on a record type (not a table).
// Returns an empty table.
func createRecordHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewList([]engine.Value{})}, nil
}

// createHandler appends a new record to the table.
// The map must contain an "id" field. If a record with the same id already
// exists, an error is returned.
func createHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rec := args[0].AsMap()
	rows := args[1].AsList().Slice()

	idVal, ok := rec.Get("id")
	if !ok {
		return nil, fmt.Errorf("create: record must contain an \"id\" field")
	}
	id, err := idVal.AsString()
	if err != nil {
		return nil, fmt.Errorf("create: id: %w", err)
	}

	// Check for duplicate id.
	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			continue
		}
		m := row.AsMap()
		if existing, ok := m.Get("id"); ok {
			existingStr, _ := existing.AsString()
			if existingStr == id {
				return nil, fmt.Errorf("create: record with id %q already exists", id)
			}
		}
	}

	result := make([]engine.Value, len(rows)+1)
	copy(result, rows)
	result[len(rows)] = args[0]
	return []engine.Value{engine.NewList(result)}, nil
}
