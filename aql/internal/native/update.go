package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// updateFunc returns the "update" native function definition.
// update has suffix precedence and three signatures:
//   - [map(kind:"api")] — updates an entity via the SDK
//   - [table, map]      — finds a record by "id" and merges the map's fields into it
//   - [map, map]        — record type + patch: returns empty table
func updateFunc() NativeFunc {
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	return NativeFunc{
		Name:             "update",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:     []engine.Type{engine.TMap, engine.TMap},
				Handler:  updateAPIOptsHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:     []engine.Type{engine.TMap},
				Handler:  updateAPIHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: updateHandler,
			},
			{
				Args:    []engine.Type{engine.TMap, engine.TMap},
				Handler: updateRecordHandler,
			},
		},
	}
}

// updateAPIOptsHandler handles update with {kind:"api",...} and an extra data map.
// The options map is merged into the data field of the API map.
func updateAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	merged := mergeAPIOptions(args[0].AsMap(), args[1].AsMap(), "data")
	return updateAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// updateAPIHandler handles update with {kind:"api", spec:String, entity:String, data:{...}}.
func updateAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := args[0].AsMap()

	sdkInst, entityName, err := getSDK(apiMap, "update", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Update(extractData(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("update: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "update")
	if err != nil {
		return nil, err
	}

	return []engine.Value{v}, nil
}

// updateRecordHandler handles update on a record type (not a table).
// Returns an empty table.
func updateRecordHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewList([]engine.Value{})}, nil
}

// updateHandler finds a record by its "id" field and merges the provided
// fields into it. Returns the updated table. The map must contain an "id" field.
func updateHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	rows := args[0].AsList()
	patch := args[1].AsMap()

	idVal, ok := patch.Get("id")
	if !ok {
		return nil, fmt.Errorf("update: record must contain an \"id\" field")
	}
	id := idVal.AsString()

	found := false
	result := make([]engine.Value, len(rows))
	for i, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			result[i] = row
			continue
		}
		rec := row.AsMap()
		existing, ok := rec.Get("id")
		if !ok || existing.AsString() != id {
			result[i] = row
			continue
		}

		// Merge: copy existing fields, then overlay patch fields.
		merged := engine.NewOrderedMap()
		for _, key := range rec.Keys() {
			v, _ := rec.Get(key)
			merged.Set(key, v)
		}
		for _, key := range patch.Keys() {
			v, _ := patch.Get(key)
			merged.Set(key, v)
		}
		result[i] = engine.NewMap(merged)
		found = true
	}

	if !found {
		return nil, fmt.Errorf("update: no record found with id %q", id)
	}

	return []engine.Value{engine.NewList(result)}, nil
}
