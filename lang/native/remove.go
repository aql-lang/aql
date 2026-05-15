package native

import (
	"fmt"

	"github.com/aql-lang/aql/lang/engine"
)

// The "remove" word is registered via the consolidated Natives slice in
// natives.go.
//
// removeEntityHandler handles remove with an Entity object instance.
func removeEntityHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap := entityToAPIMap(args[0])
	return removeAPIHandler([]engine.Value{engine.NewMap(apiMap)}, ctx, stack, r)
}

// removeEntityOptsHandler handles remove with an Entity object instance and an options map.
// Sig is opts-first: args[0]=opts, args[1]=entity.
func removeEntityOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	_m, _ := engine.AsMap(args[0])
	merged := entityToAPIMapWithOpts(args[1], _m, "query")
	return removeAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// removeAPIOptsHandler handles remove with {kind:"api",...} and an extra options map.
// The options map is merged into the query field of the API map.
// Sig is opts-first: args[0]=opts, args[1]=apiMap (pattern-matched).
func removeAPIOptsHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	_m1, _ := engine.AsMap(args[1])
	_m0, _ := engine.AsMap(args[0])
	merged := mergeAPIOptions(_m1, _m0, "query")
	return removeAPIHandler([]engine.Value{engine.NewMap(merged)}, ctx, stack, r)
}

// removeAPIHandler handles remove with {kind:"api", spec:String, entity:String, query:{...}}.
func removeAPIHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	apiMap, _ := engine.AsMap(args[0])

	sdkInst, entityName, err := getSDK(apiMap, "remove", r)
	if err != nil {
		return nil, err
	}

	ent := sdkInst.Entity(entityName, nil)
	result, err := ent.Remove(extractQuery(apiMap), nil)
	if err != nil {
		return nil, fmt.Errorf("remove: entity %q: %w", entityName, err)
	}

	v, err := convertResultItem(result, "remove")
	if err != nil {
		return nil, err
	}

	return []engine.Value{v}, nil
}

// removeRecordHandler handles remove on a record type (not a table).
// Returns an empty table.
func removeRecordHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	return []engine.Value{engine.NewList([]engine.Value{})}, nil
}

// removeHandler finds a record by its "id" field and removes it from the table.
// Returns the updated table. The map must contain an "id" field.
func removeHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	filter, _ := engine.AsMap(args[0])
	_lst, _ := engine.AsList(args[1])
	rows := _lst.Slice()

	idVal, ok := filter.Get("id")
	if !ok {
		return nil, fmt.Errorf("remove: filter must contain an \"id\" field")
	}
	id, err := engine.AsString(idVal)
	if err != nil {
		return nil, err
	}

	found := false
	var result []engine.Value
	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			result = append(result, row)
			continue
		}
		rec, _ := engine.AsMap(row)
		existing, ok := rec.Get("id")
		if ok {
			existingStr, _ := engine.AsString(existing)
			if existingStr == id {
				found = true
				continue // skip this record
			}
		}
		result = append(result, row)
	}

	if !found {
		return nil, fmt.Errorf("remove: no record found with id %q", id)
	}

	if result == nil {
		result = []engine.Value{}
	}
	return []engine.Value{engine.NewList(result)}, nil
}
