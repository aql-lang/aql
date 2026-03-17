package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// createFunc returns the "create" native function definition.
// create has suffix precedence and three signatures:
//   - [map(kind:"api")] — creates an entity via the SDK
//   - [table, map]      — appends the map as a new record to the table; the map must contain an "id" field
//   - [map, map]        — record type + record: returns empty table
func createFunc() NativeFunc {
	apiPattern := engine.NewOrderedMap()
	apiPattern.Set("kind", engine.NewString("api"))
	apiPatternVal := engine.NewMap(apiPattern)

	return NativeFunc{
		Name:             "create",
		SuffixPrecedence: true,
		Signatures: []NativeSig{
			{
				Args:     []engine.Type{engine.TMap},
				Handler:  createAPIHandler,
				Patterns: map[int]engine.Value{0: apiPatternVal},
			},
			{
				Args:    []engine.Type{engine.TList, engine.TMap},
				Handler: createHandler,
			},
			{
				Args:    []engine.Type{engine.TMap, engine.TMap},
				Handler: createRecordHandler,
			},
		},
	}
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
	rows := args[0].AsList()
	rec := args[1].AsMap()

	idVal, ok := rec.Get("id")
	if !ok {
		return nil, fmt.Errorf("create: record must contain an \"id\" field")
	}
	id := idVal.AsString()

	// Check for duplicate id.
	for _, row := range rows {
		if !row.VType.Matches(engine.TMap) {
			continue
		}
		m := row.AsMap()
		if existing, ok := m.Get("id"); ok && existing.AsString() == id {
			return nil, fmt.Errorf("create: record with id %q already exists", id)
		}
	}

	result := make([]engine.Value, len(rows)+1)
	copy(result, rows)
	result[len(rows)] = args[1]
	return []engine.Value{engine.NewList(result)}, nil
}
